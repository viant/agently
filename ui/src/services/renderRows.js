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
  const phase = String(page?.phase || '').trim().toLowerCase();
  const hasVisibleContent = String(page?.narration || '').trim() !== '' || String(page?.content || '').trim() !== '';
  const hasError = String(page?.errorMessage || page?.ErrorMessage || '').trim() !== '';
  const hasTools = (Array.isArray(page?.toolSteps) && page.toolSteps.length > 0)
    || (Array.isArray(page?.toolCallsPlanned) && page.toolCallsPlanned.length > 0);
  const hasModelCall = !!page?.modelCall || !!page?.modelSteps?.length;
  const isActive = ['running', 'thinking', 'streaming', 'processing', 'in_progress', 'waiting_for_user', 'tool_calls'].includes(status);
  const isError = status === 'failed' || status === 'error' || status === 'terminated';
  const isExplicitPhase = phase === 'intake' || phase === 'sidecar' || phase === 'summary';
  return hasVisibleContent || hasError || hasTools || isActive || isError || (isExplicitPhase && hasModelCall);
}

function executionPageIteration(page = {}) {
  const raw = Number(page?.iteration);
  if (Number.isFinite(raw)) return raw;
  const status = String(page?.status || '').trim().toLowerCase();
  const hasAssistantIdentity = String(page?.assistantMessageId || page?.pageId || '').trim() !== '';
  const hasVisibleContent = String(page?.narration || '').trim() !== '' || String(page?.content || '').trim() !== '';
  const isActive = ['running', 'thinking', 'streaming', 'processing', 'waiting_for_user', 'in_progress', 'tool_calls'].includes(status);
  if (hasAssistantIdentity || hasVisibleContent || isActive) return 1;
  return 0;
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
    const summaryPages = executionPages.filter((page) => String(page?.phase || '').trim().toLowerCase() === 'summary');
    const visiblePages = executionPages.filter((page) => String(page?.phase || '').trim().toLowerCase() !== 'summary' && isVisibleExecutionPage(page));
    const normalizedExecutionPages = [
      ...visiblePages,
      ...summaryPages.filter((page) => isVisibleExecutionPage(page) || String(page?.content || '').trim() !== '')
    ];
    const linkedConversations = Array.isArray(turn?.linkedConversations) ? turn.linkedConversations : [];
    const latestVisiblePage = visiblePages.length > 0 ? visiblePages[visiblePages.length - 1] : null;
    const latestFinalVisiblePage = visiblePages.length > 0
      ? ([...visiblePages].reverse().find((page) => page?.finalResponse) || null)
      : null;
    const latestVisibleStatus = String(latestVisiblePage?.status || turnStatus || '').trim().toLowerCase();
    const renderPageIsActive = !!latestVisiblePage && ['running', 'thinking', 'streaming', 'processing', 'in_progress', 'waiting_for_user', 'tool_calls'].includes(latestVisibleStatus);
    const renderPage = (() => {
      if (!latestVisiblePage) return null;
      if (renderPageIsActive) return latestVisiblePage;
      return latestFinalVisiblePage || latestVisiblePage;
    })();
    let hasExplicitAssistantRow = false;
    let assistantRowSuppressesElicitationContent = false;

    if (turn?.user) {
      rows.push(normalizeOne({
        id: turn.user?.messageId || `${turnId}:user`,
        messageId: turn.user?.messageId || '',
        role: 'user',
        content: turn.user?.content || '',
        turnId,
        turnStatus,
        createdAt: turn?.createdAt || '',
        sequence: Number.isFinite(Number(turn?.user?.sequence)) ? Number(turn.user.sequence) : null,
        errorMessage: turn?.errorMessage || '',
        executionGroup: null,
        executionGroups: []
      }));
    }

    const extraMessages = Array.isArray(turn?.messages) ? turn.messages : [];
    for (const message of extraMessages) {
      rows.push(normalizeOne({
        id: message?.messageId || `${turnId}:message`,
        messageId: message?.messageId || '',
        _bubbleSource: 'turn_message',
        role: String(message?.role || '').trim().toLowerCase(),
        content: message?.content || '',
        turnId,
        turnStatus,
        status: message?.status || '',
        mode: message?.mode || '',
        interim: Number(message?.interim || 0) || 0,
        createdAt: message?.createdAt || turn?.createdAt || '',
        sequence: Number.isFinite(Number(message?.sequence)) ? Number(message.sequence) : null,
        executionGroup: null,
        executionGroups: []
      }));
    }

    const assistantFinal = turn?.assistant?.final || null;
    const assistantPreamble = turn?.assistant?.narration || null;
    if (renderPage) {
      const transcriptElicitation = turn?.elicitation && typeof turn.elicitation === 'object'
        ? turn.elicitation
        : null;
      const embeddedElicitation = extractEmbeddedElicitation(renderPage?.content || '');
      const elicitationStatus = String(transcriptElicitation?.status || '').trim().toLowerCase();
      const suppressAssistantContent = !!embeddedElicitation
        && (elicitationStatus === '' || elicitationStatus === 'pending' || elicitationStatus === 'open');
      const renderedContent = renderPageIsActive
        ? (renderPage?.content || '')
        : (renderPage?.content || assistantFinal?.content || '');
      rows.push(normalizeOne({
        id: renderPage?.assistantMessageId || assistantFinal?.messageId || renderPage?.pageId || turnId,
        messageId: renderPage?.assistantMessageId || assistantFinal?.messageId || '',
        role: 'assistant',
        interim: renderPageIsActive
          ? 1
          : (renderPage?.finalResponse || String(assistantFinal?.content || '').trim() !== '' ? 0 : 1),
        content: suppressAssistantContent ? '' : renderedContent,
        narration: renderPage?.narration || assistantPreamble?.content || '',
        turnId,
        turnStatus,
        status: renderPage?.status || turnStatus,
        createdAt: turn?.createdAt || '',
        sequence: Number.isFinite(Number(renderPage?.sequence)) ? Number(renderPage.sequence) : null,
        errorMessage: renderPage?.errorMessage || turn?.errorMessage || '',
        linkedConversations,
        executionGroup: normalizedExecutionPages[0] || null,
        executionGroups: normalizedExecutionPages
      }));
      hasExplicitAssistantRow = true;
      assistantRowSuppressesElicitationContent = suppressAssistantContent;
    }

    if (!hasExplicitAssistantRow && ['running', 'thinking', 'processing', 'waiting_for_user', 'in_progress', 'tool_calls'].includes(turnStatus)) {
      rows.push(normalizeOne({
        id: `turn:${turnId}:execution`,
        role: 'assistant',
        interim: 1,
        content: '',
        narration: assistantPreamble?.content || '',
        turnId,
        turnStatus,
        status: turnStatus,
        createdAt: turn?.createdAt || '',
        errorMessage: turn?.errorMessage || '',
        linkedConversations,
        executionGroup: {
          assistantMessageId: '',
          parentMessageId: turn?.startedByMessageId || turn?.user?.messageId || '',
          iteration: 1,
          narration: assistantPreamble?.content || '',
          content: '',
          finalResponse: false,
          status: turnStatus,
          startedAt: turn?.createdAt || '',
          modelSteps: [],
          toolSteps: [],
          toolCallsPlanned: []
        },
        executionGroups: [{
          assistantMessageId: '',
          parentMessageId: turn?.startedByMessageId || turn?.user?.messageId || '',
          iteration: 1,
          narration: assistantPreamble?.content || '',
          content: '',
          finalResponse: false,
          status: turnStatus,
          startedAt: turn?.createdAt || '',
          modelSteps: [],
          toolSteps: [],
          toolCallsPlanned: []
        }]
      }));
      hasExplicitAssistantRow = true;
    }

    if (!hasExplicitAssistantRow && assistantFinal && String(assistantFinal?.content || '').trim() !== '') {
      rows.push(normalizeOne({
        id: assistantFinal?.messageId || turnId,
        role: 'assistant',
        interim: 0,
        content: assistantFinal?.content || '',
        narration: assistantPreamble?.content || '',
        turnId,
        turnStatus,
        status: turnStatus,
        createdAt: turn?.createdAt || '',
        errorMessage: turn?.errorMessage || '',
        linkedConversations,
        executionGroup: null,
        executionGroups: normalizedExecutionPages
      }));
      hasExplicitAssistantRow = true;
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

    if (turn?.elicitation && (!hasExplicitAssistantRow || assistantRowSuppressesElicitationContent)) {
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
  const trackerActiveTurnId = String(streamState?.activeTurnId || '').trim();
  const trackerTurns = projectTrackerToTurns(streamState, currentConversationID);
  const explicitRows = Array.isArray(liveRows) ? liveRows : [];
  const trackerRowsBase = buildCanonicalTranscriptRows(trackerTurns).rows;
  const trackerRows = trackerRowsBase.map((row) => {
    if (String(row?.role || '').trim().toLowerCase() !== 'assistant') return row;
    const rowId = String(row?.id || '').trim();
    if (!rowId) return row;
    const matchingExplicit = explicitRows.find((entry) => (
      String(entry?.role || '').trim().toLowerCase() === 'assistant'
      && String(entry?.id || '').trim() === rowId
    ));
    if (!matchingExplicit) return row;
    return {
      ...row,
      content: String(row?.content || '').trim() !== '' ? row.content : matchingExplicit?.content || row?.content || '',
      narration: String(row?.narration || '').trim() !== '' ? row.narration : matchingExplicit?.narration || row?.narration || '',
      isStreaming: matchingExplicit?.isStreaming,
      _streamContent: matchingExplicit?._streamContent,
      _streamFence: matchingExplicit?._streamFence,
      rawContent: matchingExplicit?.rawContent ?? row?.rawContent,
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
        const hasExecutionGroups = Array.isArray(row?.executionGroups) && row.executionGroups.length > 0;
        if (!turnId || !trackerAssistantTurnIds.has(turnId)) return true;
        return !hasExecutionGroups && Number(row?.interim ?? 0) === 0;
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
      trackerActiveTurnId
        ? Array.from(new Set([...(Array.isArray(liveOwnedTurnIds) ? liveOwnedTurnIds : []), trackerActiveTurnId]))
        : liveOwnedTurnIds
    ),
    liveRows: effectiveLiveRows,
    runningTurnId: String(runningTurnId || '').trim(),
    hasRunning: !!hasRunning || effectiveLiveRows.length > 0 || !!String(runningTurnId || '').trim(),
    findLatestRunningTurnId,
    currentConversationID,
    liveOwnedConversationID,
    liveOwnedTurnIds: trackerActiveTurnId
      ? Array.from(new Set([...(Array.isArray(liveOwnedTurnIds) ? liveOwnedTurnIds : []), trackerActiveTurnId]))
      : liveOwnedTurnIds
  });
  return {
    effectiveLiveRows,
    mergedRows
  };
}
