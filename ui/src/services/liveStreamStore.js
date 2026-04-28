import { compareExecutionGroups, compareTemporalEntries } from 'agently-core-ui-sdk';
import { mergeRowSnapshots } from './rowMerge';
import { isStreamDebugEnabled } from './debugFlags';

function isLiveStoreDebugEnabled() {
  return isStreamDebugEnabled();
}

function logLiveStoreDebug(event, detail = {}) {
  if (!isLiveStoreDebugEnabled()) return;
  // eslint-disable-next-line no-console
  console.log('[agently-live-store]', { event, ...detail });
}

function normalizedStatusValue(value = '') {
  return String(value || '').trim().toLowerCase();
}

function isTerminalStatusValue(value = '') {
  const status = normalizedStatusValue(value);
  return status === 'completed'
    || status === 'succeeded'
    || status === 'success'
    || status === 'done'
    || status === 'failed'
    || status === 'error'
    || status === 'canceled'
    || status === 'cancelled'
    || status === 'terminated';
}

function normalizeExecutionRowStatus(value = '') {
  const status = normalizedStatusValue(value);
  if (!status) return 'running';
  if (status === 'thinking' || status === 'streaming' || status === 'processing' || status === 'in_progress' || status === 'queued' || status === 'pending' || status === 'open' || status === 'tool_calls') {
    return status;
  }
  if (status === 'waiting_for_user' || status === 'blocked') return status;
  if (isTerminalStatusValue(status)) return status;
  return 'running';
}

function normalizedExecutionPayloadStatus(payload = {}) {
  const explicit = normalizedStatusValue(payload?.status);
  if (explicit) return normalizeExecutionRowStatus(explicit);
  const eventType = String(payload?.type || '').trim().toLowerCase();
  if (eventType === 'model_completed') return 'completed';
  if (eventType === 'model_started') return 'running';
  return 'running';
}

function normalizedMessageIds(payload = {}) {
  const patch = payload?.patch && typeof payload.patch === 'object' ? payload.patch : {};
  return [
    payload?.messageId,
    payload?.id,
    payload?.assistantMessageId,
    payload?.toolMessageId,
    payload?.modelCallId,
    patch?.messageId,
    patch?.id,
    patch?.assistantMessageId,
    patch?.toolMessageId,
    patch?.modelCallId,
  ].map((value) => String(value || '').trim()).filter(Boolean);
}

function canonicalPayloadMessageId(payload = {}) {
  const patch = payload?.patch && typeof payload.patch === 'object' ? payload.patch : {};
  const explicit = [
    payload?.messageId,
    payload?.assistantMessageId,
    payload?.toolMessageId,
    payload?.modelCallId,
    patch?.messageId,
    patch?.assistantMessageId,
    patch?.toolMessageId,
    patch?.modelCallId,
  ].map((value) => String(value || '').trim()).find(Boolean);
  if (explicit) return explicit;
  const type = String(payload?.type || '').trim().toLowerCase();
  if (type === 'text_delta' || type === 'narration' || type === 'assistant') {
    return String(payload?.id || '').trim();
  }
  if (String(payload?.op || '').trim().toLowerCase() === 'message_patch') {
    return String(payload?.id || patch?.id || '').trim();
  }
  return '';
}

function normalizedMode(payload = {}) {
  const patch = payload?.patch && typeof payload.patch === 'object' ? payload.patch : {};
  return String(payload?.mode || patch?.mode || '').trim().toLowerCase();
}

function syntheticExecutionRoleFromPreamblePayload(payload = {}) {
  const explicit = String(payload?.executionRole || '').trim().toLowerCase();
  if (explicit === 'narrator' || explicit === 'intake' || explicit === 'summary' || explicit === 'router') {
    return explicit;
  }
  const mode = normalizedMode(payload);
  if (mode === 'narrator' || mode === 'summary' || mode === 'router') {
    return mode;
  }
  const phase = String(payload?.phase || '').trim().toLowerCase();
  if (phase === 'intake' || phase === 'summary') {
    return phase;
  }
  return '';
}

function syntheticModelStepForPreamble(payload = {}, narration = '', existing = null) {
  const executionRole = syntheticExecutionRoleFromPreamblePayload(payload);
  if (!executionRole) return existing;
  const assistantMessageId = canonicalPayloadMessageId(payload);
  const phase = String(payload?.phase || '').trim();
  return {
    ...(existing || {}),
    modelCallId: String(existing?.modelCallId || assistantMessageId || `narration:${String(payload?.turnId || '').trim()}`).trim(),
    assistantMessageId: String(existing?.assistantMessageId || assistantMessageId).trim(),
    executionRole,
    phase,
    provider: String(existing?.provider || payload?.provider || payload?.model?.provider || '').trim(),
    model: String(existing?.model || payload?.modelName || payload?.model?.model || '').trim(),
    status: String(payload?.status || existing?.status || 'running').trim() || 'running',
    responsePayload: {
      content: String(narration || '').trim(),
      messageKind: executionRole,
    }
  };
}

function summarySuppressionSet(chatState = {}) {
  if (!chatState._suppressedSummaryMessageIds) {
    chatState._suppressedSummaryMessageIds = new Set();
  }
  return chatState._suppressedSummaryMessageIds;
}

function rememberSuppressedSummary(chatState = {}, payload = {}) {
  if (normalizedMode(payload) !== 'summary') return;
  const patch = payload?.patch && typeof payload.patch === 'object' ? payload.patch : {};
  const role = String(payload?.role || patch?.role || '').trim().toLowerCase();
  if (role && role !== 'assistant') return;
  const suppressed = summarySuppressionSet(chatState);
  for (const id of normalizedMessageIds(payload)) {
    suppressed.add(id);
  }
}

function isSuppressedSummaryEvent(chatState = {}, payload = {}) {
  if (normalizedMode(payload) === 'summary') return true;
  const suppressed = summarySuppressionSet(chatState);
  return normalizedMessageIds(payload).some((id) => suppressed.has(id));
}

function mergeCanonicalToolCalls(existing = [], incoming = []) {
  const out = [];
  const seen = new Map();
  for (const list of [existing, incoming]) {
    for (const entry of Array.isArray(list) ? list : []) {
      const key = String(
        entry?.toolCallId
        || entry?.ToolCallId
        || entry?.toolMessageId
        || entry?.ToolMessageId
        || entry?.modelCallId
        || entry?.ModelCallId
        || entry?.messageId
        || entry?.MessageId
        || entry?.id
        || entry?.Id
        || entry?.toolName
        || entry?.ToolName
        || ''
      ).trim();
      if (!key) {
        out.push(entry);
        continue;
      }
      const found = seen.get(key);
      if (found == null) {
        seen.set(key, out.length);
        out.push({ ...entry });
      } else {
        // Merge preserving existing non-empty values — incoming empty strings
        // must not overwrite existing data (e.g., model_completed without
        // provider/model must not erase model_started's provider/model).
        const prev = out[found];
        const merged = { ...prev };
        for (const [k, v] of Object.entries(entry)) {
          if (v !== undefined && v !== null && v !== '') {
            merged[k] = v;
          }
        }
        out[found] = merged;
      }
    }
  }
  return out;
}

function mergeCanonicalExecutionGroups(existing = [], incoming = []) {
  const out = [];
  const seen = new Map();
  for (const list of [existing, incoming]) {
    for (const group of Array.isArray(list) ? list : []) {
      const key = String(
        group?.assistantMessageId
        || group?.AssistantMessageId
        || group?.modelMessageId
        || group?.ModelMessageId
        || group?.parentMessageId
        || group?.ParentMessageId
        || ''
      ).trim();
      if (!key) {
        out.push(group);
        continue;
      }
      const found = seen.get(key);
      if (found == null) {
        seen.set(key, out.length);
        out.push({
          ...group,
          toolSteps: Array.isArray(group?.toolSteps) ? [...group.toolSteps] : [],
          toolCallsPlanned: Array.isArray(group?.toolCallsPlanned) ? [...group.toolCallsPlanned] : []
        });
      } else {
        out[found] = {
          ...out[found],
          ...group,
          modelSteps: mergeCanonicalToolCalls(out[found]?.modelSteps, group?.modelSteps),
          toolSteps: mergeCanonicalToolCalls(out[found]?.toolSteps, group?.toolSteps),
          toolCallsPlanned: mergeCanonicalToolCalls(out[found]?.toolCallsPlanned, group?.toolCallsPlanned)
        };
      }
    }
  }
  if (out.length >= 2) {
    const first = out[0] || {};
    const second = out[1] || {};
    const firstAssistantId = String(first?.assistantMessageId || '').trim();
    const secondAssistantId = String(second?.assistantMessageId || '').trim();
    const firstPageId = String(first?.pageId || '').trim();
    const firstToolSteps = Array.isArray(first?.toolSteps) ? first.toolSteps : [];
    const firstIsTurnLifecycleOnly = firstToolSteps.length > 0
      && firstToolSteps.every((step) => String(step?.kind || '').trim().toLowerCase() === 'turn');
    const firstIsDedicatedLifecycleGroup = firstPageId.endsWith(':lifecycle');
    const firstIsPlaceholder = !firstAssistantId
      && String(first?.status || '').trim() !== ''
      && (
        (Array.isArray(first?.modelSteps) && first.modelSteps.length > 0)
        || firstIsTurnLifecycleOnly
      );
    if (firstIsPlaceholder && !firstIsDedicatedLifecycleGroup && secondAssistantId) {
      const merged = {
        ...first,
        ...second,
        assistantMessageId: secondAssistantId,
        modelSteps: mergeCanonicalToolCalls(first?.modelSteps, second?.modelSteps),
        toolSteps: mergeCanonicalToolCalls(first?.toolSteps, second?.toolSteps),
        toolCallsPlanned: mergeCanonicalToolCalls(first?.toolCallsPlanned, second?.toolCallsPlanned)
      };
      return [merged, ...out.slice(2)];
    }
  }
  return out.sort((left, right) => compareExecutionGroups(left, right));
}

// Single helper for all handlers to find the assistant row for a given turn.
// Returns the index in the rows array, or -1 if not found.
function findAssistantRowIndex(rows, turnId, assistantMessageId) {
  if (!Array.isArray(rows)) return -1;
  const tid = String(turnId || '').trim();
  const amid = String(assistantMessageId || '').trim();
  return rows.findIndex((row) => {
    const role = String(row?.role || '').toLowerCase();
    if (role !== 'assistant') return false;
    const rowID = String(row?.id || '').trim();
    if (amid && rowID === amid) return true;
    if (!tid) return false;
    const rowTurnId = String(row?.turnId || '').trim();
    return rowTurnId === tid;
  });
}

function findAssistantRowIndexExact(rows, assistantMessageId) {
  if (!Array.isArray(rows)) return -1;
  const amid = String(assistantMessageId || '').trim();
  if (!amid) return -1;
  return rows.findIndex((row) => (
    String(row?.role || '').toLowerCase() === 'assistant'
    && String(row?.id || '').trim() === amid
  ));
}

function findAssistantExecutionRowIndex(rows, turnId, assistantMessageId) {
  const exact = findAssistantRowIndexExact(rows, assistantMessageId);
  if (exact !== -1) return exact;
  const tid = String(turnId || '').trim();
  if (!Array.isArray(rows) || !tid) return -1;
  for (let index = rows.length - 1; index >= 0; index -= 1) {
    const row = rows[index];
    if (String(row?.role || '').toLowerCase() !== 'assistant') continue;
    if (String(row?.turnId || '').trim() !== tid) continue;
    const groups = Array.isArray(row?.executionGroups) ? row.executionGroups : [];
    const isStandaloneAssistantBubble = groups.length === 0 && Number(row?.interim ?? 0) === 0;
    if (isStandaloneAssistantBubble) {
      return -1;
    }
    if (groups.length > 0) {
      return index;
    }
  }
  return -1;
}

function isAssistantPlaceholderId(id = '', turnId = '') {
  const rowId = String(id || '').trim();
  const tid = String(turnId || '').trim();
  if (!rowId || !tid) return false;
  return rowId === `assistant:${tid}:live`
    || rowId === `assistant:${tid}:1`
    || rowId === `turn:${tid}`;
}

function maybePromoteAssistantRowIdentity(row = {}, turnId = '', assistantMessageId = '') {
  const promotedId = String(assistantMessageId || '').trim();
  if (!promotedId) return row;
  const next = { ...row };
  if (isAssistantPlaceholderId(next?.id, turnId)) {
    next.id = promotedId;
  }
  if (Array.isArray(next.executionGroups) && next.executionGroups.length > 0) {
    next.executionGroups = next.executionGroups.map((group, index) => {
      const pageId = String(group?.pageId || '').trim();
      if (pageId.endsWith(':lifecycle')) return group;
      if (index !== next.executionGroups.length - 1) return group;
      if (String(group?.assistantMessageId || '').trim()) return group;
      return {
        ...group,
        assistantMessageId: promotedId,
        pageId: String(group?.pageId || '').trim() || promotedId,
        startedAt: String(group?.startedAt || '').trim() || String(next?.startedAt || '').trim() || undefined,
        modelSteps: Array.isArray(group?.modelSteps)
          ? group.modelSteps.map((step) => ({
              ...step,
              modelCallId: String(step?.modelCallId || '').trim() || promotedId,
              assistantMessageId: String(step?.assistantMessageId || '').trim() || promotedId,
              startedAt: String(step?.startedAt || '').trim() || String(next?.startedAt || '').trim() || undefined,
            }))
          : group?.modelSteps,
      };
    });
  }
  return next;
}

function preserveExecutionGroupStartedAt(groups = [], fallbackStartedAt = '') {
  const preserved = String(fallbackStartedAt || '').trim();
  if (!Array.isArray(groups) || groups.length === 0 || !preserved) return groups;
  return groups.map((group) => ({
    ...group,
    startedAt: String(group?.startedAt || '').trim() || preserved,
    modelSteps: Array.isArray(group?.modelSteps)
      ? group.modelSteps.map((step) => ({
          ...step,
          startedAt: String(step?.startedAt || '').trim() || preserved,
        }))
      : group?.modelSteps,
  }));
}

function preserveModelStepStartedAt(previousGroups = [], mergedGroups = [], fallbackStartedAt = '') {
  const previousList = Array.isArray(previousGroups) ? previousGroups : [];
  const previousByAssistantId = new Map(
    previousList.map((group) => [
      String(group?.assistantMessageId || '').trim(),
      group,
    ])
  );
  const fallback = String(fallbackStartedAt || '').trim();
  return (Array.isArray(mergedGroups) ? mergedGroups : []).map((group, index) => {
    const assistantMessageId = String(group?.assistantMessageId || '').trim();
    const previous = previousByAssistantId.get(assistantMessageId) || previousList[index] || previousList[previousList.length - 1];
    const previousStartedAt = String(previous?.startedAt || '').trim() || fallback;
    if (!previousStartedAt) return group;
    return {
      ...group,
      startedAt: String(group?.startedAt || '').trim() || previousStartedAt,
      modelSteps: Array.isArray(group?.modelSteps)
        ? group.modelSteps.map((step, index) => {
            const previousStep = Array.isArray(previous?.modelSteps) ? previous.modelSteps[index] : null;
            return {
              ...step,
              startedAt: String(step?.startedAt || '').trim() || String(previousStep?.startedAt || '').trim() || previousStartedAt,
            };
          })
        : group?.modelSteps,
    };
  });
}

function buildCanonicalExecutionRow(payload = {}, fallbackConversationID = '') {
  const conversationID = String(payload?.conversationId || fallbackConversationID || '').trim();
  const turnId = String(payload?.turnId || '').trim();
  const assistantMessageId = canonicalPayloadMessageId(payload);
  const rowID = assistantMessageId || `assistant:${turnId || conversationID}:${Number(payload?.iteration || payload?.pageIndex || 1) || 1}`;
  if (!rowID) return null;
  const createdAt = String(payload?.createdAt || '').trim();
  const finalResponse = !!payload?.finalResponse;
  const normalizedPayloadContent = normalizeStreamingMarkdown(String(payload?.content || '').trim()).content;
  const normalizedVisibleContent = normalizeStreamingMarkdown(String(payload?.narration || payload?.content || '').trim()).content;
  const errorMessage = String(payload?.error || payload?.errorMessage || '').trim();
  const startedAt = String(payload?.startedAt || payload?.createdAt || '').trim() || undefined;
  const normalizedStatus = normalizedExecutionPayloadStatus(payload);
  const completedAt = String(
    payload?.completedAt
    || (String(payload?.type || '').trim().toLowerCase() === 'model_completed' ? (payload?.createdAt || '') : '')
    || (payload?.finalResponse ? (payload?.createdAt || '') : '')
    || (String(normalizedStatus || '').toLowerCase() === 'completed' ? (payload?.createdAt || '') : '')
    || ''
  ).trim() || undefined;
  const group = {
    pageId: assistantMessageId || rowID,
    assistantMessageId,
    parentMessageId: String(payload?.parentMessageId || '').trim(),
    phase: String(payload?.phase || '').trim() || undefined,
    sequence: Number(payload?.pageIndex || payload?.iteration || payload?.eventSeq || 0) || undefined,
    iteration: Number(payload?.iteration || 0) || undefined,
    narration: String(payload?.narration || '').trim(),
    content: finalResponse ? normalizedPayloadContent : '',
    finalResponse,
    status: normalizedStatus,
    errorMessage,
    modelSteps: [{
      modelCallId: String(payload?.modelCallId || assistantMessageId || '').trim() || undefined,
      assistantMessageId,
      phase: String(payload?.phase || '').trim() || undefined,
      provider: String(payload?.model?.provider || '').trim(),
      model: String(payload?.model?.model || '').trim(),
      status: normalizedStatus,
      startedAt,
      completedAt,
      requestPayloadId: String(payload?.requestPayloadId || '').trim() || undefined,
      responsePayloadId: String(payload?.responsePayloadId || '').trim() || undefined,
      providerRequestPayloadId: String(payload?.providerRequestPayloadId || '').trim() || undefined,
      providerResponsePayloadId: String(payload?.providerResponsePayloadId || '').trim() || undefined,
      streamPayloadId: String(payload?.streamPayloadId || '').trim() || undefined
    }],
    toolSteps: Array.isArray(payload?.toolCallsPlanned)
      ? payload.toolCallsPlanned.map((tc) => ({
          toolCallId: String(tc?.toolCallId || tc?.ToolCallId || '').trim(),
          toolName: String(tc?.toolName || tc?.ToolName || '').trim(),
          status: 'planned'
        })).filter((s) => s.toolCallId || s.toolName)
      : [],
    toolCallsPlanned: Array.isArray(payload?.toolCallsPlanned) ? payload.toolCallsPlanned : []
  };
  return {
    id: rowID,
    conversationId: conversationID,
    turnId,
    agentIdUsed: String(payload?.agentIdUsed || '').trim(),
    agentName: String(payload?.agentName || '').trim(),
    role: 'assistant',
    mode: String(payload?.mode || '').trim().toLowerCase(),
    type: 'text',
    createdAt,
    startedAt,
    completedAt,
    errorMessage,
    sequence: Number(payload?.eventSeq || payload?.pageIndex || payload?.iteration || 0) || null,
    status: normalizedStatus,
    turnStatus: normalizedStatus,
    interim: finalResponse ? 0 : 1,
    content: finalResponse ? normalizedPayloadContent : normalizedVisibleContent,
    narration: String(payload?.narration || '').trim(),
    executionGroups: [group]
  };
}

function buildLifecycleStep(kind = '', payload = {}) {
  const lifecycleKind = String(kind || '').trim().toLowerCase();
  if (!lifecycleKind) return null;
  const turnId = String(payload?.turnId || '').trim();
  const createdAt = String(payload?.createdAt || payload?.completedAt || '').trim() || undefined;
  const status = lifecycleKind === 'turn_completed'
    ? 'completed'
    : lifecycleKind === 'turn_failed'
      ? 'failed'
      : lifecycleKind === 'turn_canceled'
        ? 'canceled'
        : 'running';
  return {
    id: `${lifecycleKind}:${turnId || 'turn'}`,
    kind: 'turn',
    reason: lifecycleKind,
    toolName: lifecycleKind,
    status,
    startedAt: createdAt,
    completedAt: ['turn_completed', 'turn_failed', 'turn_canceled'].includes(lifecycleKind) ? createdAt : undefined,
    errorMessage: String(payload?.error || payload?.errorMessage || '').trim() || undefined,
  };
}

function appendLifecycleStep(row = {}, kind = '', payload = {}) {
  const lifecycleStep = buildLifecycleStep(kind, payload);
  if (!lifecycleStep) return row;
  const groups = Array.isArray(row?.executionGroups) ? [...row.executionGroups] : [];
  const turnId = String(payload?.turnId || row?.turnId || '').trim() || 'turn';
  const lifecycleGroupId = `turn:${turnId}:lifecycle`;
  const existingIndex = groups.findIndex((group) => String(group?.pageId || group?.parentMessageId || '').trim() === lifecycleGroupId);
  const group = existingIndex >= 0 ? { ...groups[existingIndex] } : {
    pageId: lifecycleGroupId,
    assistantMessageId: '',
    parentMessageId: lifecycleGroupId,
    phase: 'lifecycle',
    sequence: -1,
    iteration: 0,
    narration: '',
    content: '',
    finalResponse: false,
    status: String(payload?.status || row?.status || 'running').trim(),
    errorMessage: '',
    modelSteps: [],
    toolSteps: [],
    toolCallsPlanned: []
  };
  group.toolSteps = mergeCanonicalToolCalls(group.toolSteps, [lifecycleStep]);
  group.status = String(payload?.status || group?.status || row?.status || lifecycleStep.status).trim() || lifecycleStep.status;
  if (existingIndex >= 0) {
    groups[existingIndex] = group;
  } else {
    groups.unshift(group);
  }
  return {
    ...row,
    executionGroups: groups
  };
}

function payloadSequence(payload = {}) {
  const value = Number(payload?.eventSeq ?? payload?.pageIndex ?? payload?.iteration ?? 0);
  return Number.isFinite(value) ? value : 0;
}

function ensureLiveTurnRows(chatState = {}, payload = {}, fallbackConversationID = '') {
  const conversationID = String(payload?.conversationId || fallbackConversationID || '').trim();
  const turnId = String(payload?.turnId || '').trim();
  if (!turnId) return Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];

  const rows = Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];
  const turnStartedAt = String(payload?.createdAt || '').trim();
  const turnSequence = payloadSequence(payload);
  const userSequence = turnSequence > 0 ? (turnSequence * 2) - 1 : 0;
  const assistantSequence = turnSequence > 0 ? turnSequence * 2 : 1;
  const prompt = String(chatState?.activeStreamPrompt || '').trim();
  const userMessageID = String(payload?.userMessageId || payload?.startedByMessageId || '').trim();
  const userRowID = userMessageID || (prompt ? `user:${turnId}` : '');
  const existingUserIndex = rows.findIndex((row) => {
    if (String(row?.role || '').trim().toLowerCase() !== 'user') return false;
    const rowTurnId = String(row?.turnId || '').trim();
    if (rowTurnId && rowTurnId === turnId) return true;
    return !!userRowID && String(row?.id || '').trim() === userRowID;
  });
  if (existingUserIndex === -1 && userRowID) {
    rows.push({
      id: userRowID,
      role: 'user',
      type: 'text',
      turnId,
      conversationId: conversationID,
      createdAt: turnStartedAt,
      sequence: userSequence,
      interim: 0,
      status: 'completed',
      turnStatus: 'running',
      content: prompt,
      rawContent: prompt
    });
  } else if (existingUserIndex >= 0 && prompt) {
    rows[existingUserIndex] = {
      ...rows[existingUserIndex],
      id: String(rows[existingUserIndex]?.id || '').trim() || userRowID,
      role: 'user',
      type: rows[existingUserIndex]?.type || 'text',
      turnId,
      conversationId: conversationID || rows[existingUserIndex]?.conversationId || '',
      content: rows[existingUserIndex]?.content || prompt,
      rawContent: rows[existingUserIndex]?.rawContent || prompt
    };
  }

  const assistantIndex = findAssistantRowIndex(rows, turnId, '');
  if (assistantIndex !== -1) {
    const row = { ...rows[assistantIndex] };
    row.agentIdUsed = String(payload?.agentIdUsed || row?.agentIdUsed || '').trim();
    row.agentName = String(payload?.agentName || row?.agentName || '').trim();
    row.turnStatus = String(payload?.status || row?.turnStatus || 'running').trim();
    row.status = String(payload?.status || row?.status || 'running').trim();
    row.startedAt = row.startedAt || turnStartedAt;
    rows[assistantIndex] = appendLifecycleStep(row, 'turn_started', payload);
    rows.sort(compareTemporalEntries);
    chatState.liveRows = rows;
    return rows;
  }

  const lifecycleRow = appendLifecycleStep({
    id: `turn:${turnId}`,
    conversationId: conversationID,
    turnId,
    agentIdUsed: String(payload?.agentIdUsed || '').trim(),
    agentName: String(payload?.agentName || '').trim(),
    role: 'assistant',
    mode: String(payload?.mode || '').trim().toLowerCase(),
    type: 'text',
    createdAt: turnStartedAt,
    startedAt: turnStartedAt || undefined,
    completedAt: undefined,
    errorMessage: '',
    sequence: assistantSequence,
    status: 'running',
    turnStatus: 'running',
    interim: 1,
    content: '',
    narration: '',
    executionGroups: []
  }, 'turn_started', payload);
  rows.push(lifecycleRow);
  rows.sort(compareTemporalEntries);
  chatState.liveRows = rows;
  return rows;
}

function applyExecutionStreamEventToRows(rows = [], payload = {}, fallbackConversationID = '') {
  const nextRow = buildCanonicalExecutionRow(payload, fallbackConversationID);
  if (!nextRow) return Array.isArray(rows) ? rows : [];
  const existing = Array.isArray(rows) ? [...rows] : [];
  const nextTurnId = String(nextRow.turnId || '').trim();
  const index = findAssistantExecutionRowIndex(existing, nextTurnId, nextRow.id);
  if (index === -1) {
    existing.push(nextRow);
  } else {
    let prev = existing[index];
    prev = maybePromoteAssistantRowIdentity(prev, nextTurnId, nextRow.id);
    const preservedStartedAt = String(prev?.startedAt || '').trim();
    if (preservedStartedAt) {
      nextRow.startedAt = preservedStartedAt;
      nextRow.executionGroups = preserveExecutionGroupStartedAt(nextRow.executionGroups, preservedStartedAt);
    }
    const eventType = String(payload?.type || '').trim().toLowerCase();
    const shouldAdvanceRowStatus = eventType === 'model_started' || eventType === 'text_delta' || eventType === 'tool_calls_planned';
    // Update content/status when finalResponse arrives; otherwise keep existing
    const updatedContent = nextRow.interim === 0 && String(nextRow.content || '').trim()
      ? nextRow.content
      : prev.content;
    const updatedInterim = nextRow.interim === 0 ? 0 : prev.interim;
    existing[index] = {
      ...prev,
      agentIdUsed: String(nextRow.agentIdUsed || prev?.agentIdUsed || '').trim(),
      agentName: String(nextRow.agentName || prev?.agentName || '').trim(),
      id: String(nextRow.id || prev?.id || '').trim() || prev?.id,
      status: shouldAdvanceRowStatus ? (nextRow.status || prev.status) : (prev.status || nextRow.status),
      turnStatus: shouldAdvanceRowStatus ? (nextRow.turnStatus || prev.turnStatus) : (prev.turnStatus || nextRow.turnStatus),
      startedAt: prev.startedAt || nextRow.startedAt,
      createdAt: prev.createdAt || nextRow.createdAt,
      sequence: nextRow.sequence ?? null,
      interim: updatedInterim,
      content: updatedContent,
      narration: nextRow.narration || prev.narration,
      executionGroups: preserveModelStepStartedAt(
        prev.executionGroups,
        preserveExecutionGroupStartedAt(
          mergeCanonicalExecutionGroups(prev.executionGroups, nextRow.executionGroups),
          preservedStartedAt || nextRow.startedAt || prev.startedAt
        ),
        preservedStartedAt || nextRow.startedAt || prev.startedAt
      )
    };
  }
  existing.sort(compareTemporalEntries);
  return existing;
}

function applyToolStreamEventToRows(rows = [], payload = {}, fallbackConversationID = '') {
  const assistantMessageId = canonicalPayloadMessageId(payload);
  if (!assistantMessageId) return Array.isArray(rows) ? rows : [];
  const turnId = String(payload?.turnId || '').trim();
  const existing = Array.isArray(rows) ? [...rows] : [];
  const index = findAssistantExecutionRowIndex(existing, turnId, assistantMessageId);
  if (index === -1) return existing;
  const row = { ...existing[index] };
  const groups = Array.isArray(row.executionGroups) ? row.executionGroups : [];
  // Find the execution group matching the assistantMessageId, or fall back to
  // the last group (most recent iteration owns the tool call).
  let groupIdx = groups.findIndex((g) =>
    String(g?.assistantMessageId || '').trim() === assistantMessageId
  );
  if (groupIdx === -1) groupIdx = groups.length - 1;
  if (groupIdx < 0 || !groups[groupIdx]) return existing;
  const group = { ...groups[groupIdx] };
    const toolStep = {
    toolMessageId: String(payload?.toolMessageId || payload?.messageId || payload?.id || '').trim(),
    toolCallId: String(payload?.toolCallId || '').trim(),
    toolName: String(payload?.toolName || '').trim(),
    phase: String(payload?.phase || '').trim() || undefined,
    status: String(payload?.status || '').trim(),
    requestPayload: payload?.arguments && typeof payload.arguments === 'object'
      ? payload.arguments
      : undefined,
    requestPayloadId: String(payload?.requestPayloadId || '').trim() || undefined,
    responsePayload: payload?.responsePayload && typeof payload.responsePayload === 'object'
      ? payload.responsePayload
      : undefined,
    responsePayloadId: String(payload?.responsePayloadId || '').trim() || undefined,
    linkedConversationId: String(payload?.linkedConversationId || '').trim() || undefined,
    linkedConversationAgentId: String(payload?.linkedConversationAgentId || '').trim() || undefined,
    linkedConversationTitle: String(payload?.linkedConversationTitle || '').trim() || undefined,
    startedAt: payload?.createdAt || undefined,
    completedAt: ['tool_call_completed', 'tool_call_failed', 'tool_call_canceled'].includes(payload?.type)
      ? (payload?.createdAt || undefined)
      : undefined
  };
  group.toolSteps = mergeCanonicalToolCalls(group.toolSteps, [toolStep]);
  const updatedGroups = [...groups];
  updatedGroups[groupIdx] = group;
  row.executionGroups = updatedGroups;
  // Propagate turnId to the row so later terminal turn events
  // can find this row by turnId even when model_started never fired.
  if (turnId && !row.turnId) {
    row.turnId = turnId;
  }
  existing[index] = row;
  return existing;
}

function applyMessagePatchToRows(rows = [], payload = {}) {
  const patch = payload?.patch && typeof payload.patch === 'object' ? payload.patch : {};
  const messageId = String(payload?.messageId || payload?.id || '').trim();
  const turnId = String(patch?.turnId || '').trim();
  if (!messageId && !turnId) return Array.isArray(rows) ? rows : [];

  const patchCreatedAt = String(patch?.createdAt || '').trim();
  const role = String(patch?.role || '').trim().toLowerCase();
  const messageType = String(patch?.messageType || '').trim();
  // For user messages, prefer rawContent (original query) over content
  // (which may be the expanded/internal prompt).
  const rawContent = String(patch?.rawContent || '').trim();
  const patchContent = normalizeStreamingMarkdown(rawContent || String(patch?.content || '').trim()).content;
  const fallbackPatchId = [
    'patch',
    turnId || 'no-turn',
    role || 'message',
    messageType || 'text'
  ].join(':');
  const baseRow = {
    id: messageId || fallbackPatchId,
    role,
    phase: String(patch?.phase || '').trim().toLowerCase(),
    mode: String(patch?.mode || '').trim().toLowerCase(),
    type: messageType,
    turnId,
    createdAt: patchCreatedAt,
    status: String(patch?.status || '').trim(),
    turnStatus: String(patch?.status || '').trim(),
    interim: Number(patch?.interim ?? 0) || 0,
    content: patchContent,
    rawContent: rawContent,
    narration: String(patch?.narration || '').trim(),
    toolName: String(patch?.toolName || '').trim(),
    linkedConversationId: String(patch?.linkedConversationId || '').trim(),
    parentMessageId: String(patch?.parentMessageId || '').trim(),
    sequence: Number.isFinite(Number(patch?.sequence)) ? Number(patch.sequence) : null,
    iteration: Number.isFinite(Number(patch?.iteration)) ? Number(patch.iteration) : null
  };
  logLiveStoreDebug('message_patch:incoming', {
    messageId,
    turnId,
    role,
    mode: baseRow.mode,
    type: baseRow.type,
    contentHead: String(baseRow.content || '').slice(0, 120),
    rawHead: String(baseRow.rawContent || '').slice(0, 120)
  });

  const existing = Array.isArray(rows) ? [...rows] : [];
  const filtered = existing.filter((row) => {
    if (String(row?._type || '').toLowerCase() !== 'stream') return true;
    const sameTurn = turnId && String(row?.turnId || '').trim() === turnId;
    if (!sameTurn) return true;
    const isExecutionEvidence = role === 'tool'
      || messageType === 'tool'
      || messageType === 'tool_op'
      || (role === 'assistant' && Number(baseRow.interim || 0) === 1 && (baseRow.content || baseRow.narration));
    return !isExecutionEvidence;
  });
  if (!role && !turnId) {
    logLiveStoreDebug('message_patch:ignored-empty-identity', {
      messageId,
      hasContent: patchContent !== '',
      hasRawContent: rawContent !== '',
      mode: baseRow.mode,
      type: baseRow.type
    });
    return filtered;
  }
  if (role === 'user' && turnId) {
    const existingUserIdx = filtered.findIndex((row) => (
      String(row?.role || '').trim().toLowerCase() === 'user'
      && String(row?.turnId || '').trim() === turnId
    ));
    if (existingUserIdx !== -1) {
      const prev = filtered[existingUserIdx];
      filtered[existingUserIdx] = {
        ...prev,
        role: 'user',
        phase: baseRow.phase || prev.phase,
        mode: baseRow.mode || prev.mode,
        type: baseRow.type || prev.type,
        turnId,
        status: baseRow.status || prev.status,
        turnStatus: baseRow.turnStatus || prev.turnStatus,
        interim: baseRow.interim,
        content: patchContent || prev.content,
        rawContent: rawContent || String(prev.rawContent || ''),
        narration: baseRow.narration || prev.narration,
        toolName: baseRow.toolName || prev.toolName,
        linkedConversationId: baseRow.linkedConversationId || prev.linkedConversationId,
        parentMessageId: baseRow.parentMessageId || prev.parentMessageId,
        sequence: baseRow.sequence ?? prev.sequence ?? null,
        iteration: baseRow.iteration ?? prev.iteration ?? null,
      };
      logLiveStoreDebug('message_patch:user-merged', {
        turnId,
        existingId: String(prev?.id || '').trim(),
        resultId: String(filtered[existingUserIdx]?.id || '').trim(),
        contentHead: String(filtered[existingUserIdx]?.content || '').slice(0, 120)
      });
      return filtered;
    }
  }
  // For assistant messages, merge into an existing execution row for the same
  // turn rather than creating a duplicate row. mergeRowSnapshots matches by id
  // only, so a message_patch with id "msg-123" would not merge into an
  // execution row with id "mc-1" — even though they represent the same turn.
  if (role === 'assistant' && turnId) {
    const existingIdx = findAssistantRowIndex(filtered, turnId, messageId);
    if (existingIdx !== -1) {
      const prev = filtered[existingIdx];
      const groups = Array.isArray(prev.executionGroups) ? [...prev.executionGroups] : [];
      const patchInterim = Number(patch?.interim ?? 0) || 0;
      const patchStatus = String(patch?.status || '').trim();
      const patchPreamble = String(patch?.narration || '').trim();
      let groupPatched = false;
      const updatedGroups = groups.map((group, index) => {
        const assistantMessageId = String(group?.assistantMessageId || '').trim();
        const shouldPatch = (messageId && assistantMessageId === messageId)
          || (!messageId && index === groups.length - 1);
        if (!shouldPatch) return group;
        groupPatched = true;
        return {
          ...group,
          narration: patchPreamble || group?.narration || '',
          content: patchContent || group?.content || '',
          finalResponse: patchInterim === 0 && patchContent !== '' ? true : !!group?.finalResponse,
          status: patchStatus || (patchInterim === 0 ? 'completed' : 'streaming') || group?.status || ''
        };
      });
      // Never set interim=0 from message_patch — the backend may
      // prematurely clear interim for tool-call responses. Only
      // Assistant message appends and turn_completed should mark a row as final.
      filtered[existingIdx] = {
        ...prev,
        phase: baseRow.phase || prev.phase,
        mode: baseRow.mode || prev.mode,
        content: patchContent || prev.content,
        rawContent: rawContent || String(prev.rawContent || ''),
        narration: String(patch?.narration || '').trim() || prev.narration,
        status: baseRow.status || prev.status,
        turnStatus: baseRow.turnStatus || prev.turnStatus,
        executionGroups: groupPatched ? updatedGroups : prev.executionGroups
      };
      logLiveStoreDebug('message_patch:assistant-merged-into-execution', {
        turnId,
        existingId: String(prev?.id || '').trim(),
        patchId: messageId,
        groupPatched,
        contentHead: String(filtered[existingIdx]?.content || '').slice(0, 120)
      });
      return filtered;
    }
  }
  // Tool/tool_op message patches should not create standalone rows when an
  // execution row already exists for the turn — the execution group's toolSteps
  // are the authoritative source for tool visibility.
  if ((role === 'tool' || messageType === 'tool' || messageType === 'tool_op') && turnId) {
    const execIdx = findAssistantRowIndex(filtered, turnId, '');
    if (execIdx !== -1 && Array.isArray(filtered[execIdx]?.executionGroups) && filtered[execIdx].executionGroups.length > 0) {
      return filtered;
    }
  }
  const merged = mergeRowSnapshots(filtered, [baseRow]);
  logLiveStoreDebug('message_patch:appended-row', {
    turnId,
    role,
    messageId: baseRow.id,
    mergedCount: merged.length
  });
  return merged;
}

export function resetLiveStreamState(chatState = {}, options = {}) {
  chatState.liveRows = [];
  chatState.lastStreamEventAt = 0;
  chatState.activeStreamTurnId = '';
  chatState.activeStreamStartedAt = 0;
  chatState.liveOwnedConversationID = '';
  chatState.liveOwnedTurnIds = [];
  if (!options?.preservePrompt) {
    chatState.activeStreamPrompt = '';
  }
  chatState._suppressedSummaryMessageIds = new Set();
}

function stripLeadingFence(text = '') {
  const value = String(text || '');
  const match = value.match(/^```([a-zA-Z0-9_-]+)?\r?\n?/);
  if (!match) return { text: value, hasLeadingFence: false, language: '' };
  return {
    text: value.slice(match[0].length),
    hasLeadingFence: true,
    language: String(match[1] || '').trim().toLowerCase(),
  };
}

function stripTrailingFence(text = '') {
  const value = String(text || '');
  const match = value.match(/\r?\n?```$/);
  if (!match) return { text: value, hasTrailingFence: false };
  return {
    text: value.slice(0, value.length - match[0].length),
    hasTrailingFence: true,
  };
}

export function normalizeStreamingMarkdown(text = '') {
  const raw = String(text || '');
  const leading = stripLeadingFence(raw);
  const trailing = stripTrailingFence(leading.text);
  return {
    content: trailing.text,
    hadLeadingFence: leading.hasLeadingFence,
    hadTrailingFence: trailing.hasTrailingFence,
    language: leading.language,
  };
}

export function applyStreamChunk(chatState = {}, payload = {}, conversationID = '') {
  if (isSuppressedSummaryEvent(chatState, payload)) {
    rememberSuppressedSummary(chatState, payload);
    return chatState.liveRows || [];
  }
  const turnId = String(payload?.turnId || chatState.activeStreamTurnId || chatState.runningTurnId || '').trim();
  const streamMessageID = canonicalPayloadMessageId(payload);
  const delta = String(payload?.content || '');
  if (!delta) return chatState.liveRows || [];

  const liveRows = Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];
  // Find the existing assistant execution row for this turn.
  const index = findAssistantExecutionRowIndex(liveRows, turnId, streamMessageID);
  if (index >= 0) {
    // Append delta to the execution row's streaming content.
    const row = { ...liveRows[index] };
    row._streamContent = `${String(row._streamContent || '')}${delta}`;
    const normalized = normalizeStreamingMarkdown(row._streamContent);
    row._streamFence = {
      hasLeadingFence: normalized.hadLeadingFence,
      hasTrailingFence: normalized.hadTrailingFence,
      language: normalized.language,
    };
    row.content = normalized.content;
    row.isStreaming = true;
    // Clear narration once real content starts streaming — prevents
    // concatenated narration+response in the bubble.
    if (row.narration && row._streamContent.length > 0) {
      row.narration = '';
    }
    const groups = Array.isArray(row.executionGroups) ? [...row.executionGroups] : [];
    if (groups.length > 0) {
      const groupIndex = groups.length - 1;
      const group = { ...groups[groupIndex] };
      group.content = row.content;
      group.finalResponse = false;
      group.status = 'streaming';
      if (Array.isArray(group.modelSteps) && group.modelSteps.length > 0) {
        const modelSteps = [...group.modelSteps];
        const lastModelStepIndex = modelSteps.length - 1;
        modelSteps[lastModelStepIndex] = {
          ...modelSteps[lastModelStepIndex],
          status: 'streaming'
        };
        group.modelSteps = modelSteps;
      }
      groups[groupIndex] = group;
      row.executionGroups = groups;
    }
    liveRows[index] = row;
  } else {
    // No execution row yet (text_delta arrived before model_started).
    // Create a minimal assistant row that model_started will merge into.
    const streamID = String(payload?.streamId || conversationID);
    const normalized = normalizeStreamingMarkdown(delta);
    const visibleContent = normalized.content;
    liveRows.push({
      id: streamMessageID || `assistant:${turnId || streamID}:1`,
      role: 'assistant',
      mode: String(payload?.mode || '').trim().toLowerCase(),
      turnId,
      status: 'running',
      turnStatus: 'running',
      interim: 1,
      isStreaming: true,
      _streamContent: delta,
      _streamFence: {
        hasLeadingFence: normalized.hadLeadingFence,
        hasTrailingFence: normalized.hadTrailingFence,
        language: normalized.language,
      },
      content: visibleContent,
      executionGroups: [{
        assistantMessageId: streamMessageID || '',
        content: visibleContent,
        finalResponse: false,
        status: 'streaming',
        modelSteps: [{
          modelCallId: streamMessageID || '',
          assistantMessageId: streamMessageID || '',
          status: 'streaming'
        }],
        toolSteps: [],
        toolCallsPlanned: []
      }],
      createdAt: String(payload?.createdAt || '').trim(),
      sequence: payloadSequence(payload)
    });
    liveRows.sort(compareTemporalEntries);
  }
  chatState.liveRows = liveRows;
  return liveRows;
}

function applyAssistantFinalToRows(rows = [], payload = {}) {
  const patch = payload?.patch && typeof payload.patch === 'object' ? payload.patch : {};
  const turnId = String(payload?.turnId || patch?.turnId || '').trim();
  const content = normalizeStreamingMarkdown(String(
    patch?.rawContent
    || payload?.content
    || patch?.content
    || ''
  ).trim()).content;
  if (!turnId || !content) return Array.isArray(rows) ? rows : [];
  const existing = Array.isArray(rows) ? [...rows] : [];
  const index = findAssistantRowIndex(existing, turnId, canonicalPayloadMessageId(payload));
  if (index === -1) return existing;
  const prev = existing[index];
  const groups = Array.isArray(prev.executionGroups) ? [...prev.executionGroups] : [];
  // Update the last execution group with final content — don't create new groups.
    const assistantMessageId = String(
      payload?.messageId
      || payload?.id
      || patch?.messageId
      || patch?.id
      || canonicalPayloadMessageId(payload)
      || ''
    ).trim();
    if (groups.length > 0) {
      const last = groups[groups.length - 1];
      groups[groups.length - 1] = {
        ...last,
        content,
        finalResponse: true,
        status: String(payload?.status || last.status || '').trim()
      };
    }
  existing[index] = maybePromoteAssistantRowIdentity({
    ...prev,
    id: assistantMessageId || prev.id,
    agentIdUsed: String(payload?.agentIdUsed || prev?.agentIdUsed || '').trim(),
    agentName: String(payload?.agentName || prev?.agentName || '').trim(),
    mode: String(payload?.mode || prev?.mode || '').trim().toLowerCase(),
    content: normalizeStreamingMarkdown(content).content,
    completedAt: String(payload?.completedAt || payload?.createdAt || prev?.completedAt || '').trim(),
    interim: 0,
    isStreaming: false,
    _streamContent: '',
    _streamFence: null,
    status: normalizeExecutionRowStatus(payload?.status || prev.status),
    turnStatus: normalizeExecutionRowStatus(payload?.status || prev.turnStatus),
    executionGroups: preserveExecutionGroupStartedAt(groups, prev.startedAt)
  }, turnId, assistantMessageId);
  return existing;
}

export function applyElicitationRequestedEvent(chatState = {}, payload = {}) {
  const turnId = String(payload?.turnId || '').trim();
  const assistantMessageId = canonicalPayloadMessageId(payload);
  const elicitationId = String(payload?.elicitationId || '').trim();
  const elicitationData = payload?.elicitationData && typeof payload.elicitationData === 'object'
    ? payload.elicitationData
    : null;
  const message = String(payload?.content || '').trim();
  const callbackURL = String(payload?.callbackUrl || '').trim();
  if (!elicitationId) return chatState.liveRows || [];

  const requestedSchema = elicitationData?.requestedSchema
    || elicitationData?.schema
    || elicitationData
    || null;
  const elicitation = {
    elicitationId,
    message,
    requestedSchema,
    callbackURL
  };

  // Build the callbackURL for the resolve endpoint if not provided by the event.
  const conversationId = String(payload?.conversationId || payload?.streamId || '').trim();
  const resolvedCallbackURL = callbackURL
    || (conversationId && elicitationId
      ? `/v1/elicitations/${encodeURIComponent(conversationId)}/${encodeURIComponent(elicitationId)}/resolve`
      : '');

  const rows = Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];
  const index = findAssistantExecutionRowIndex(rows, turnId, assistantMessageId);
  if (index >= 0) {
    const prev = rows[index];
    rows[index] = {
      ...prev,
      // Replace raw streamed content (e.g. LLM-generated JSON) with the
      // elicitation message so the UI no longer displays raw JSON blocks.
    content: message || prev.content,
    _streamContent: '',
    _streamFence: null,
      elicitation,
      elicitationId,
      callbackURL: resolvedCallbackURL,
      conversationId,
      status: 'pending'
    };
  } else {
    // Create a minimal row for the elicitation.
    // Use role 'elicition' so classifyMessage routes to the modal renderer.
    rows.push({
      id: assistantMessageId || `elicitation:${turnId}:${elicitationId}`,
      role: 'elicition',
      turnId,
      turnStatus: 'waiting_for_user',
      status: 'pending',
      interim: 1,
      content: message,
      elicitation,
      elicitationId,
      callbackURL: resolvedCallbackURL,
      conversationId,
      createdAt: String(payload?.createdAt || '').trim(),
      sequence: payloadSequence(payload)
    });
    rows.sort(compareTemporalEntries);
  }
  chatState.liveRows = rows;
  return rows;
}

export function applyPreambleEvent(chatState = {}, payload = {}, fallbackConversationID = '') {
  if (isSuppressedSummaryEvent(chatState, payload)) {
    rememberSuppressedSummary(chatState, payload);
    return chatState.liveRows || [];
  }
  const turnId = String(payload?.turnId || '').trim();
  const assistantMessageId = canonicalPayloadMessageId(payload);
  const narration = String(payload?.content || payload?.narration || '').trim();
  if (!narration) return chatState.liveRows || [];

  const rows = Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];
  const index = findAssistantExecutionRowIndex(rows, turnId, assistantMessageId);
  if (index >= 0) {
    const prev = rows[index];
    const groups = Array.isArray(prev.executionGroups) ? [...prev.executionGroups] : [];
    if (groups.length > 0) {
      const lastIndex = groups.length - 1;
      const last = { ...groups[lastIndex] };
      const lastAssistantMessageId = String(last?.assistantMessageId || '').trim();
      const shouldAppendGroup = assistantMessageId && lastAssistantMessageId && lastAssistantMessageId !== assistantMessageId;
      if (shouldAppendGroup) {
        const syntheticStep = syntheticModelStepForPreamble(payload, narration, null);
        groups.push({
          assistantMessageId,
          pageId: assistantMessageId,
          narration,
          iteration: Number(payload?.iteration || 0) || undefined,
          status: String(payload?.status || 'running').trim().toLowerCase() || 'running',
          modelSteps: syntheticStep ? [syntheticStep] : [],
          toolSteps: [],
          toolCallsPlanned: [],
        });
      } else {
        last.narration = narration;
        const modelSteps = Array.isArray(last.modelSteps) ? [...last.modelSteps] : [];
        const syntheticStep = syntheticModelStepForPreamble(payload, narration, modelSteps[0] || null);
        if (syntheticStep) {
          if (modelSteps.length > 0) {
            modelSteps[0] = syntheticStep;
          } else {
            modelSteps.push(syntheticStep);
          }
          last.modelSteps = modelSteps;
        }
        if (!String(last?.assistantMessageId || '').trim() && assistantMessageId) {
          last.assistantMessageId = assistantMessageId;
          last.pageId = String(last?.pageId || '').trim() || assistantMessageId;
        }
        groups[lastIndex] = last;
      }
    }
    const hasStreamContent = String(prev?._streamContent || '').trim() !== '';
    rows[index] = {
      ...prev,
      agentIdUsed: String(payload?.agentIdUsed || prev?.agentIdUsed || '').trim(),
      agentName: String(payload?.agentName || prev?.agentName || '').trim(),
      // For active interim pages, the latest narration should own the bubble
      // until actual streamed/final content replaces it.
      content: hasStreamContent ? prev.content : narration,
      narration,
      executionGroups: groups
    };
    rows[index] = maybePromoteAssistantRowIdentity(rows[index], turnId, assistantMessageId);
  } else {
    // No row yet — create a minimal one that model_started will merge into
    const conversationID = String(payload?.conversationId || fallbackConversationID || '').trim();
    rows.push({
      id: assistantMessageId || `assistant:${turnId || conversationID}:1`,
      role: 'assistant',
      mode: String(payload?.mode || '').trim().toLowerCase(),
      turnId,
      conversationId: conversationID,
      agentIdUsed: String(payload?.agentIdUsed || '').trim(),
      agentName: String(payload?.agentName || '').trim(),
      turnStatus: 'running',
      interim: 1,
      content: narration,
      narration,
      executionGroups: [{
        assistantMessageId,
        narration,
        iteration: Number(payload?.iteration || 0) || undefined,
        modelSteps: (() => {
          const step = syntheticModelStepForPreamble(payload, narration, null);
          return step ? [step] : [];
        })()
      }],
      createdAt: String(payload?.createdAt || '').trim(),
      sequence: payloadSequence(payload)
    });
    rows.sort(compareTemporalEntries);
  }
  chatState.liveRows = rows;
  return rows;
}

export function applyAssistantTerminalEvent(chatState = {}, payload = {}) {
  if (isSuppressedSummaryEvent(chatState, payload)) {
    rememberSuppressedSummary(chatState, payload);
    return chatState.liveRows || [];
  }
  const nextRows = applyAssistantFinalToRows(chatState.liveRows, payload);
  chatState.liveRows = nextRows;
  return nextRows;
}

// Backward-compatible alias while older callers/tests move to the more
// accurate terminal-event naming.

export function applyAssistantMessageAddEvent(chatState = {}, payload = {}) {
  rememberSuppressedSummary(chatState, payload);
  if (isSuppressedSummaryEvent(chatState, payload)) {
    return chatState.liveRows || [];
  }
  const patch = payload?.patch && typeof payload.patch === 'object' ? payload.patch : {};
  const messageId = String(payload?.messageId || payload?.id || patch?.id || '').trim();
  const turnId = String(patch?.turnId || payload?.turnId || '').trim();
  const role = String(patch?.role || '').trim().toLowerCase();
  const content = normalizeStreamingMarkdown(String(patch?.rawContent || payload?.content || patch?.content || '').trim()).content;
  if (!messageId || !turnId || role !== 'assistant' || !content) {
    return Array.isArray(chatState.liveRows) ? chatState.liveRows : [];
  }
  const rows = Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];
  const existingIndex = rows.findIndex((row) => String(row?.id || '').trim() === messageId);
  const nextRow = {
    id: messageId,
    _bubbleSource: 'message_add',
    role: 'assistant',
    mode: String(patch?.mode || '').trim().toLowerCase(),
    type: String(patch?.messageType || patch?.type || 'text').trim().toLowerCase(),
    turnId,
    createdAt: String(patch?.createdAt || payload?.createdAt || '').trim(),
    status: String(payload?.status || patch?.status || 'completed').trim(),
    turnStatus: String(patch?.status || 'running').trim(),
    interim: Number(patch?.interim ?? 0) || 0,
    content,
    rawContent: String(patch?.rawContent || '').trim(),
    narration: String(patch?.narration || '').trim(),
    parentMessageId: String(patch?.parentMessageId || '').trim(),
    sequence: Number.isFinite(Number(patch?.sequence)) ? Number(patch.sequence) : null,
    iteration: Number.isFinite(Number(patch?.iteration)) ? Number(patch.iteration) : null,
    executionGroups: []
  };
  if (existingIndex >= 0) {
    rows[existingIndex] = { ...rows[existingIndex], ...nextRow };
  } else {
    rows.push(nextRow);
    rows.sort(compareTemporalEntries);
  }
  chatState.liveRows = rows;
  return rows;
}

export function applyExecutionStreamEvent(chatState = {}, payload = {}, fallbackConversationID = '') {
  if (isSuppressedSummaryEvent(chatState, payload)) {
    rememberSuppressedSummary(chatState, payload);
    return chatState.liveRows || [];
  }
  const nextRows = applyExecutionStreamEventToRows(chatState.liveRows, payload, fallbackConversationID);
  const targetTurnId = String(payload?.turnId || '').trim();
  const row = (Array.isArray(nextRows) ? nextRows : []).find((entry) => String(entry?.turnId || '').trim() === targetTurnId)
    || (Array.isArray(nextRows) ? nextRows[nextRows.length - 1] : null);
  const groups = Array.isArray(row?.executionGroups) ? row.executionGroups : [];
  logLiveStoreDebug('execution_stream_applied', {
    type: String(payload?.type || '').trim().toLowerCase(),
    turnId: targetTurnId,
    rowId: String(row?.id || '').trim(),
    rowStatus: String(row?.status || '').trim(),
    rowTurnStatus: String(row?.turnStatus || '').trim(),
    groups: groups.map((group) => ({
      pageId: String(group?.pageId || '').trim(),
      phase: String(group?.phase || '').trim(),
      status: String(group?.status || '').trim(),
      modelSteps: Array.isArray(group?.modelSteps) ? group.modelSteps.length : 0,
      toolSteps: Array.isArray(group?.toolSteps) ? group.toolSteps.length : 0,
      plannedTools: Array.isArray(group?.toolCallsPlanned) ? group.toolCallsPlanned.length : 0,
    })),
  });
  chatState.liveRows = nextRows;
  return nextRows;
}

export function applyTurnStartedEvent(chatState = {}, payload = {}, fallbackConversationID = '') {
  return ensureLiveTurnRows(chatState, payload, fallbackConversationID);
}

export function applyToolStreamEvent(chatState = {}, payload = {}, fallbackConversationID = '') {
  const nextRows = applyToolStreamEventToRows(chatState.liveRows, payload, fallbackConversationID);
  chatState.liveRows = nextRows;
  return nextRows;
}

export function applyMessagePatchEvent(chatState = {}, payload = {}) {
  rememberSuppressedSummary(chatState, payload);
  if (isSuppressedSummaryEvent(chatState, payload)) {
    return chatState.liveRows || [];
  }
  const nextRows = applyMessagePatchToRows(chatState.liveRows, payload);
  chatState.liveRows = nextRows;
  return nextRows;
}

export function markLiveOwnedTurn(chatState = {}, conversationID = '', turnID = '') {
  const nextConversationID = String(conversationID || '').trim();
  const nextTurnID = String(turnID || '').trim();
  if (nextConversationID) {
    chatState.liveOwnedConversationID = nextConversationID;
  }
  if (!nextTurnID) {
    const existing = Array.isArray(chatState.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [];
    chatState.liveOwnedTurnIds = existing;
    return existing;
  }
  const next = [nextTurnID];
  chatState.liveOwnedTurnIds = next;
  if (Array.isArray(chatState.liveRows) && chatState.liveRows.length > 0) {
    chatState.liveRows = chatState.liveRows.filter((row) => {
      const rowTurnID = String(row?.turnId || '').trim();
      if (!rowTurnID) return true;
      return rowTurnID === nextTurnID;
    });
  }
  return next;
}

export function finalizeStreamTurn(chatState = {}, payload = {}, fallbackConversationID = '') {
  const turnID = String(payload?.turnId || chatState.activeStreamTurnId || chatState.runningTurnId || '').trim();
  const content = String(payload?.content || '').trim();
  const status = String(payload?.status || 'completed').trim() || 'completed';
  const errorMessage = String(payload?.error || payload?.errorMessage || '').trim();
  const rows = Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];
  let finalized = false;
  let assignedTerminalContent = false;

  // Terminal turn ownership is exact: once the turn ends, every assistant row
  // for that turn must stop presenting as live/running. The latest matching
  // row receives the terminal content payload; earlier assistant rows keep
  // their own content but still transition to the terminal lifecycle.
  for (let index = rows.length - 1; index >= 0; index -= 1) {
    const row = rows[index];
    const rowTurnID = String(row?.turnId || '').trim();
    if (turnID && rowTurnID && rowTurnID !== turnID) continue;
    const role = String(row?.role || '').toLowerCase();
    if (role !== 'assistant') continue;

    const streamContent = String(row?._streamContent || '').trim();
    const baseContent = String(row?.content || '').trim();
    const shouldAssignTerminalContent = !assignedTerminalContent;
    const finalizedContent = shouldAssignTerminalContent
      ? normalizeStreamingMarkdown(content || streamContent || baseContent).content
      : normalizeStreamingMarkdown(baseContent).content;
    const groups = Array.isArray(row?.executionGroups) ? row.executionGroups : [];
    rows[index] = appendLifecycleStep({
      ...row,
      mode: String(payload?.mode || row?.mode || '').trim().toLowerCase(),
      status,
      turnStatus: status,
      completedAt: String(payload?.completedAt || payload?.createdAt || row?.completedAt || '').trim(),
      errorMessage: errorMessage || row?.errorMessage || '',
      interim: 0,
      isStreaming: false,
      content: finalizedContent,
      _streamContent: '',
      _streamFence: null,
      executionGroups: groups.map((group) => {
        const finalizedAt = String(payload?.completedAt || payload?.createdAt || '').trim();
        const modelSteps = Array.isArray(group?.modelSteps)
          ? group.modelSteps.map((ms) => {
              if (ms?.startedAt && !ms?.completedAt && finalizedAt) {
                return { ...ms, completedAt: finalizedAt, status };
              }
              return ms?.status === status ? ms : { ...ms, status };
            })
          : group?.modelSteps;
        return {
          ...group,
          status,
          errorMessage: errorMessage || group?.errorMessage || '',
          finalResponse: shouldAssignTerminalContent
            ? (finalizedContent ? true : Boolean(group?.finalResponse || group?.FinalResponse))
            : Boolean(group?.finalResponse || group?.FinalResponse),
          content: finalizedContent || String(group?.content || ''),
          modelSteps
        };
      })
    }, payload?.type || 'turn_completed', payload);
    finalized = true;
    if (shouldAssignTerminalContent) {
      assignedTerminalContent = true;
    }
  }

  if (!finalized && turnID) {
    const fallbackRow = buildCanonicalExecutionRow({
      ...payload,
      turnId: turnID,
      status,
      finalResponse: false,
      narration: '',
      content: ''
    }, fallbackConversationID);
    if (fallbackRow) {
      rows.push({
        ...fallbackRow,
        status,
        turnStatus: status,
        interim: 0,
        errorMessage,
        content: '',
        executionGroups: (Array.isArray(fallbackRow.executionGroups) ? fallbackRow.executionGroups : []).map((group) => ({
          ...group,
          status,
          errorMessage,
          content: '',
          finalResponse: false
        }))
      });
      rows.sort(compareTemporalEntries);
    }
  }

  chatState.liveRows = rows;
  chatState.activeStreamTurnId = '';
  chatState.activeStreamStartedAt = 0;
  chatState.activeStreamPrompt = '';
  if (turnID) {
    const nextOwnedTurnIds = (Array.isArray(chatState.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [])
      .map((value) => String(value || '').trim())
      .filter((value) => value && value !== turnID);
    chatState.liveOwnedTurnIds = nextOwnedTurnIds;
    if (nextOwnedTurnIds.length === 0) {
      chatState.liveOwnedConversationID = '';
    }
  }
  return rows;
}
