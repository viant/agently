import { compareTemporalEntries } from 'agently-core-ui-sdk';
import { projectTrackerToTurns } from 'agently-core-ui-sdk/internal';
import { normalizeOne } from './messageNormalizer';
import { mergeRenderedRows } from './rowMerge';

function filterLiveOwnedTranscriptRows(rows = [], currentConversationID = '', ownedConversationID = '', ownedTurnIds = []) {
  const currentID = String(currentConversationID || '').trim();
  const liveID = String(ownedConversationID || '').trim();
  if (!currentID || !liveID || currentID !== liveID) {
    return Array.isArray(rows) ? rows : [];
  }
  const owned = new Set((Array.isArray(ownedTurnIds) ? ownedTurnIds : []).map((item) => String(item || '').trim()).filter(Boolean));
  if (owned.size === 0) {
    return Array.isArray(rows) ? rows : [];
  }
  return (Array.isArray(rows) ? rows : []).filter((row) => !owned.has(String(row?.turnId || '').trim()));
}

function isVisibleExecutionPage(page = {}) {
  if (!page || typeof page !== 'object') return false;
  const status = String(page?.status || '').trim().toLowerCase();
  const hasVisibleContent = String(page?.preamble || '').trim() !== '' || String(page?.content || '').trim() !== '';
  const hasError = String(page?.errorMessage || page?.ErrorMessage || '').trim() !== '';
  const hasTools = (Array.isArray(page?.toolSteps) && page.toolSteps.length > 0)
    || (Array.isArray(page?.toolCallsPlanned) && page.toolCallsPlanned.length > 0);
  const isActive = ['running', 'thinking', 'streaming', 'processing', 'in_progress', 'waiting_for_user', 'tool_calls'].includes(status);
  const isError = status === 'failed' || status === 'error' || status === 'terminated';
  return hasVisibleContent || hasError || hasTools || isActive || isError;
}

function extractEmbeddedElicitation(text = '') {
  const raw = String(text || '').trim();
  if (!raw) return null;
  let candidate = raw;
  try {
    const fence = raw.match(/```(?:json)?\s*([\s\S]*?)\s*```/i);
    if (fence && fence[1]) candidate = String(fence[1]).trim();
  } catch (_) {}
  const start = candidate.indexOf('{');
  if (start === -1) return null;

  let depth = 0;
  let inString = false;
  let escaped = false;
  for (let i = start; i < candidate.length; i += 1) {
    const ch = candidate[i];
    if (inString) {
      if (escaped) {
        escaped = false;
      } else if (ch === '\\') {
        escaped = true;
      } else if (ch === '"') {
        inString = false;
      }
      continue;
    }
    if (ch === '"') {
      inString = true;
      continue;
    }
    if (ch === '{') {
      depth += 1;
      continue;
    }
    if (ch !== '}') continue;
    depth -= 1;
    if (depth !== 0) continue;
    try {
      const parsed = JSON.parse(candidate.slice(start, i + 1));
      if (!parsed || typeof parsed !== 'object') return null;
      if (String(parsed.type || '').toLowerCase() !== 'elicitation') return null;
      return {
        message: String(parsed.message || parsed.prompt || '').trim(),
        requestedSchema: parsed.requestedSchema || null
      };
    } catch (_) {
      return null;
    }
  }
  return null;
}

function displayableElicitationMessage(rawMessage = '', embedded = null) {
  const explicit = String(rawMessage || '').trim();
  if (embedded?.message) return embedded.message;
  if (!explicit) return '';
  if (/^[{\[]/.test(explicit)) return '';
  return explicit;
}

export function isCanonicalTranscriptTurn(turn = {}) {
  return !!turn && typeof turn === 'object' && (
    Object.prototype.hasOwnProperty.call(turn, 'turnId')
    || Object.prototype.hasOwnProperty.call(turn, 'execution')
    || Object.prototype.hasOwnProperty.call(turn, 'assistant')
    || Object.prototype.hasOwnProperty.call(turn, 'user')
    || Object.prototype.hasOwnProperty.call(turn, 'elicitation')
  );
}

export function buildCanonicalTranscriptRows(turns = [], options = {}) {
  const rows = [];
  const queuedTurns = [];
  let runningTurnId = '';
  const list = Array.isArray(turns) ? turns : [];
  const holdAfterTurnId = String(options?.holdAfterTurnId || '').trim();
  const holdAfterTurnIndex = holdAfterTurnId
    ? list.findIndex((turn) => String(turn?.turnId || '').trim() === holdAfterTurnId)
    : -1;
  const runningTurnIndex = list.findIndex((turn) => {
    const status = String(turn?.status || '').trim().toLowerCase();
    return ['running', 'thinking', 'processing', 'waiting_for_user', 'in_progress'].includes(status);
  });

  const turnPreview = (turn = {}) => {
    const user = turn?.user && typeof turn.user === 'object' ? turn.user : {};
    const content = String(user?.content || '').trim();
    return {
      id: turn?.turnId || '',
      conversationId: turn?.conversationId || '',
      status: String(turn?.status || '').toLowerCase(),
      queueSeq: turn?.queueSeq || null,
      content,
      preview: content.slice(0, 220),
      createdAt: turn?.createdAt || '',
      overrides: {
        agent: String(turn?.agentIdUsed || '').trim(),
        model: String(turn?.modelOverride || '').trim(),
        tools: []
      }
    };
  };

  const hasPersistedAssistant = (turn = {}) => {
    const assistantFinal = turn?.assistant?.final || null;
    if (assistantFinal && String(assistantFinal?.content || '').trim() !== '') return true;
    const pages = Array.isArray(turn?.execution?.pages) ? turn.execution.pages : [];
    return pages.some((page) => Number(page?.iteration || 0) !== 0 && String(page?.content || '').trim() !== '');
  };

  for (let turnIndex = 0; turnIndex < list.length; turnIndex += 1) {
    const turn = list[turnIndex] || {};
    const turnId = String(turn?.turnId || '').trim();
    const turnStatus = String(turn?.status || '').trim().toLowerCase();
    const shouldHoldBehindRunningTurn = runningTurnIndex >= 0
      && turnIndex > runningTurnIndex
      && !hasPersistedAssistant(turn);
    const shouldHoldBehindLiveStream = holdAfterTurnIndex >= 0
      && turnIndex > holdAfterTurnIndex;

    if (!runningTurnId && ['running', 'thinking', 'processing', 'waiting_for_user', 'in_progress'].includes(turnStatus)) {
      runningTurnId = turnId;
    }
    if (turnStatus === 'queued' || turnStatus === 'pending' || turnStatus === 'open' || shouldHoldBehindRunningTurn || shouldHoldBehindLiveStream) {
      queuedTurns.push(turnPreview(turn));
      continue;
    }

    const executionPages = Array.isArray(turn?.execution?.pages) ? turn.execution.pages : [];
    const summaryPages = executionPages.filter((page) => Number(page?.iteration || 0) === 0);
    const visiblePages = executionPages.filter((page) => Number(page?.iteration || 0) !== 0 && isVisibleExecutionPage(page));
    const normalizedExecutionPages = [
      ...visiblePages,
      ...summaryPages.filter((page) => isVisibleExecutionPage(page) || String(page?.content || '').trim() !== '')
    ];
    const linkedConversations = Array.isArray(turn?.linkedConversations) ? turn.linkedConversations : [];

    if (turn?.user) {
      rows.push(normalizeOne({
        id: turn.user?.messageId || `${turnId}:user`,
        role: 'user',
        content: turn.user?.content || '',
        turnId,
        turnStatus,
        createdAt: turn?.createdAt || '',
        errorMessage: turn?.errorMessage || '',
        executionGroup: null,
        executionGroups: [],
        executionGroupsTotal: 0,
        executionGroupsOffset: 0,
        executionGroupsLimit: 0
      }));
    }

    const assistantFinal = turn?.assistant?.final || null;
    const assistantPreamble = turn?.assistant?.preamble || null;
    if (visiblePages.length > 0) {
      const lastPage = visiblePages[visiblePages.length - 1];
      const finalPage = [...visiblePages].reverse().find((page) => page?.finalResponse) || lastPage;
      const transcriptElicitation = turn?.elicitation && typeof turn.elicitation === 'object'
        ? turn.elicitation
        : null;
      const embeddedElicitation = extractEmbeddedElicitation(finalPage?.content || '');
      const elicitationStatus = String(transcriptElicitation?.status || '').trim().toLowerCase();
      const suppressAssistantContent = !!embeddedElicitation
        && (elicitationStatus === '' || elicitationStatus === 'pending' || elicitationStatus === 'open');
      rows.push(normalizeOne({
        id: finalPage?.assistantMessageId || assistantFinal?.messageId || finalPage?.pageId || turnId,
        role: 'assistant',
        interim: finalPage?.finalResponse || String(assistantFinal?.content || '').trim() !== '' ? 0 : 1,
        content: suppressAssistantContent ? '' : (finalPage?.content || assistantFinal?.content || ''),
        preamble: visiblePages[0]?.preamble || assistantPreamble?.content || '',
        turnId,
        turnStatus,
        status: finalPage?.status || turnStatus,
        createdAt: turn?.createdAt || '',
        errorMessage: finalPage?.errorMessage || turn?.errorMessage || '',
        linkedConversations,
        executionGroup: normalizedExecutionPages[0] || null,
        executionGroups: normalizedExecutionPages,
        executionGroupsTotal: normalizedExecutionPages.length,
        executionGroupsOffset: 0,
        executionGroupsLimit: normalizedExecutionPages.length
      }));
    } else if (assistantFinal && String(assistantFinal?.content || '').trim() !== '') {
      rows.push(normalizeOne({
        id: assistantFinal?.messageId || turnId,
        role: 'assistant',
        interim: 0,
        content: assistantFinal?.content || '',
        preamble: assistantPreamble?.content || '',
        turnId,
        turnStatus,
        status: turnStatus,
        createdAt: turn?.createdAt || '',
        errorMessage: turn?.errorMessage || '',
        linkedConversations,
        executionGroup: null,
        executionGroups: normalizedExecutionPages,
        executionGroupsTotal: normalizedExecutionPages.length,
        executionGroupsOffset: 0,
        executionGroupsLimit: normalizedExecutionPages.length
      }));
    }

    if (summaryPages.length > 0) {
      const summaryPage = summaryPages[summaryPages.length - 1];
      rows.push(normalizeOne({
        id: summaryPage?.assistantMessageId || summaryPage?.pageId || `summary:${turnId}`,
        role: 'assistant',
        mode: 'summary',
        interim: 0,
        content: summaryPage?.content || '',
        turnId,
        turnStatus,
        status: summaryPage?.status || turnStatus,
        createdAt: turn?.createdAt || ''
      }));
    }

    if (turn?.elicitation) {
      const elic = turn.elicitation;
      const embeddedElicitation = extractEmbeddedElicitation(turn?.assistant?.final?.content || '');
      const elicitationMessage = displayableElicitationMessage(elic.message || '', embeddedElicitation);
      rows.push(normalizeOne({
        id: `elicitation:${elic.elicitationId || turnId}`,
        role: 'assistant',
        interim: 0,
        content: elicitationMessage,
        turnId,
        turnStatus,
        status: elic.status || 'pending',
        elicitationId: elic.elicitationId || '',
        elicitation: {
          elicitationId: elic.elicitationId || '',
          message: elicitationMessage,
          requestedSchema: elic.requestedSchema || embeddedElicitation?.requestedSchema || null,
          callbackURL: elic.callbackUrl || elic.callbackURL || ''
        }
      }));
    }

    for (const linked of linkedConversations) {
      if (!linked?.conversationId) continue;
      rows.push(normalizeOne({
        id: `linked:${linked.conversationId}`,
        role: 'tool',
        type: 'tool',
        kind: 'tool',
        reason: 'link',
        toolName: 'llm/agents/run',
        turnId,
        turnStatus,
        linkedConversationId: linked.conversationId,
        linkedConversationAgentId: linked.agentId || '',
        linkedConversationTitle: linked.title || '',
        status: linked.status || '',
        response: linked.response || '',
        updatedAt: linked.updatedAt || '',
        createdAt: linked.createdAt || ''
      }));
    }
  }

  queuedTurns.sort((a, b) => {
    const aSeq = Number(a?.queueSeq || 0);
    const bSeq = Number(b?.queueSeq || 0);
    if (aSeq !== bSeq) return aSeq - bSeq;
    return String(a?.id || '').localeCompare(String(b?.id || ''));
  });
  rows.sort(compareTemporalEntries);
  return { rows, queuedTurns, runningTurnId };
}

export function buildConversationRenderRows({
  transcriptRows = [],
  streamState = null,
  liveRows = [],
  currentConversationID = '',
  runningTurnId = '',
  hasRunning = false,
  findLatestRunningTurnId = () => '',
  liveOwnedConversationID = '',
  liveOwnedTurnIds = []
} = {}) {
  const trackerTurns = projectTrackerToTurns(streamState, currentConversationID);
  const explicitRows = Array.isArray(liveRows) ? liveRows : [];
  const trackerRowsBase = buildCanonicalTranscriptRows(trackerTurns).rows;
  const trackerRows = trackerRowsBase.map((row) => {
    if (String(row?.role || '').trim().toLowerCase() !== 'assistant') return row;
    const rowId = String(row?.id || '').trim();
    const rowTurnId = String(row?.turnId || '').trim();
    const trackerRowsForTurn = rowTurnId
      ? trackerRowsBase.filter((entry) => (
        String(entry?.role || '').trim().toLowerCase() === 'assistant'
        && String(entry?.turnId || '').trim() === rowTurnId
      ))
      : [];
    const explicitRowsForTurn = rowTurnId
      ? explicitRows.filter((entry) => (
        String(entry?.role || '').trim().toLowerCase() === 'assistant'
        && String(entry?.turnId || '').trim() === rowTurnId
      ))
      : [];
    const matchingExplicit = explicitRows.find((entry) => {
      if (String(entry?.role || '').trim().toLowerCase() !== 'assistant') return false;
      const explicitId = String(entry?.id || '').trim();
      if (rowId && explicitId && rowId === explicitId) return true;
      return false;
    });
    const fallbackExplicit = !matchingExplicit && trackerRowsForTurn.length === 1 && explicitRowsForTurn.length === 1
      ? explicitRowsForTurn[0]
      : null;
    const resolvedExplicit = matchingExplicit || fallbackExplicit;
    if (!resolvedExplicit) return row;
    return {
      ...row,
      isStreaming: resolvedExplicit?.isStreaming,
      _streamContent: resolvedExplicit?._streamContent,
      _streamFence: resolvedExplicit?._streamFence,
      rawContent: resolvedExplicit?.rawContent ?? row?.rawContent,
    };
  });
  const trackerAssistantTurnIds = new Set(
    trackerRows
      .filter((row) => String(row?.role || '').trim().toLowerCase() === 'assistant')
      .map((row) => String(row?.turnId || '').trim())
      .filter(Boolean)
  );
  const effectiveLiveRows = [
    ...trackerRows,
    ...explicitRows.filter((row) => {
      const type = String(row?._type || '').trim().toLowerCase();
      if (type === 'stream') return true;
      const role = String(row?.role || '').trim().toLowerCase();
      if (role === 'user') return true;
      if (role === 'assistant') {
        const turnId = String(row?.turnId || '').trim();
        return !turnId || !trackerAssistantTurnIds.has(turnId);
      }
      // Non-user/non-assistant explicit live rows are intentionally excluded here.
      // The tracker projection owns canonical live rendering; explicit rows are
      // only for transient overlays and optimistic user bubbles.
      return false;
    })
  ].sort(compareTemporalEntries);
  const mergedRows = mergeRenderedRows({
    transcriptRows: filterLiveOwnedTranscriptRows(
      transcriptRows,
      currentConversationID,
      liveOwnedConversationID,
      liveOwnedTurnIds
    ),
    liveRows: effectiveLiveRows,
    runningTurnId: String(runningTurnId || '').trim(),
    hasRunning: !!hasRunning || effectiveLiveRows.length > 0 || !!String(runningTurnId || '').trim(),
    findLatestRunningTurnId,
    currentConversationID,
    liveOwnedConversationID,
    liveOwnedTurnIds
  });
  return {
    effectiveLiveRows,
    mergedRows
  };
}
