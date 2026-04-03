import { compareExecutionGroups, compareTemporalEntries } from 'agently-core-ui-sdk';

function chooseRichValue(...values) {
  for (const value of values) {
    if (typeof value === 'string') {
      if (value.trim() !== '') return value;
      continue;
    }
    if (value != null) return value;
  }
  return values[values.length - 1];
}

function executionStepKey(step = {}) {
  const explicitID = String(step?.id || '').trim();
  if (explicitID) return explicitID;
  return [
    String(step?.kind || ''),
    String(step?.toolName || ''),
    String(step?.requestPayloadId || ''),
    String(step?.responsePayloadId || ''),
    String(step?.providerRequestPayloadId || ''),
    String(step?.providerResponsePayloadId || ''),
    String(step?.streamPayloadId || '')
  ].join('::');
}

function mergeStep(existing = {}, incoming = {}) {
  return {
    ...existing,
    ...incoming,
    id: chooseRichValue(incoming?.id, existing?.id, ''),
    kind: chooseRichValue(incoming?.kind, existing?.kind, ''),
    reason: chooseRichValue(incoming?.reason, existing?.reason, ''),
    toolName: chooseRichValue(incoming?.toolName, existing?.toolName, ''),
    status: chooseRichValue(incoming?.status, existing?.status, ''),
    provider: chooseRichValue(incoming?.provider, existing?.provider, ''),
    model: chooseRichValue(incoming?.model, existing?.model, ''),
    linkedConversationId: chooseRichValue(incoming?.linkedConversationId, existing?.linkedConversationId, ''),
    latencyMs: chooseRichValue(incoming?.latencyMs, existing?.latencyMs, null),
    requestPayloadId: chooseRichValue(incoming?.requestPayloadId, existing?.requestPayloadId, ''),
    responsePayloadId: chooseRichValue(incoming?.responsePayloadId, existing?.responsePayloadId, ''),
    providerRequestPayloadId: chooseRichValue(incoming?.providerRequestPayloadId, existing?.providerRequestPayloadId, ''),
    providerResponsePayloadId: chooseRichValue(incoming?.providerResponsePayloadId, existing?.providerResponsePayloadId, ''),
    streamPayloadId: chooseRichValue(incoming?.streamPayloadId, existing?.streamPayloadId, ''),
    requestPayload: incoming?.requestPayload ?? existing?.requestPayload ?? null,
    responsePayload: incoming?.responsePayload ?? existing?.responsePayload ?? null,
    providerRequestPayload: incoming?.providerRequestPayload ?? existing?.providerRequestPayload ?? null,
    providerResponsePayload: incoming?.providerResponsePayload ?? existing?.providerResponsePayload ?? null,
    streamPayload: incoming?.streamPayload ?? existing?.streamPayload ?? null
  };
}

function mergeExecutions(existing = [], incoming = []) {
  const steps = [];
  const seen = new Map();
  const addStep = (step = {}) => {
    const key = executionStepKey(step);
    const found = seen.get(key);
    if (found == null) {
      seen.set(key, steps.length);
      steps.push(step);
    } else {
      steps[found] = mergeStep(steps[found], step);
    }
  };
  for (const list of [existing, incoming]) {
    for (const execution of Array.isArray(list) ? list : []) {
      for (const step of Array.isArray(execution?.steps) ? execution.steps : []) {
        addStep(step);
      }
    }
  }
  return steps.length > 0 ? [{ steps }] : [];
}

function mergeModelCall(existing = {}, incoming = {}) {
  return {
    ...existing,
    ...incoming,
    provider: chooseRichValue(incoming?.provider, existing?.provider, ''),
    model: chooseRichValue(incoming?.model, existing?.model, ''),
    status: chooseRichValue(incoming?.status, existing?.status, ''),
    startedAt: chooseRichValue(incoming?.startedAt, existing?.startedAt, ''),
    completedAt: chooseRichValue(incoming?.completedAt, existing?.completedAt, ''),
    requestPayloadId: chooseRichValue(incoming?.requestPayloadId, existing?.requestPayloadId, ''),
    responsePayloadId: chooseRichValue(incoming?.responsePayloadId, existing?.responsePayloadId, ''),
    providerRequestPayloadId: chooseRichValue(incoming?.providerRequestPayloadId, existing?.providerRequestPayloadId, ''),
    providerResponsePayloadId: chooseRichValue(incoming?.providerResponsePayloadId, existing?.providerResponsePayloadId, ''),
    streamPayloadId: chooseRichValue(incoming?.streamPayloadId, existing?.streamPayloadId, ''),
    requestPayload: incoming?.requestPayload ?? existing?.requestPayload ?? null,
    responsePayload: incoming?.responsePayload ?? existing?.responsePayload ?? null,
    providerRequestPayload: incoming?.providerRequestPayload ?? existing?.providerRequestPayload ?? null,
    providerResponsePayload: incoming?.providerResponsePayload ?? existing?.providerResponsePayload ?? null,
    streamPayload: incoming?.streamPayload ?? existing?.streamPayload ?? null
  };
}

function mergeModelStep(existing = {}, incoming = {}) {
  const merged = mergeModelCall(existing, incoming);
  return {
    ...merged,
    provider: chooseRichValue(merged?.provider, ''),
    model: chooseRichValue(merged?.model, ''),
    status: chooseRichValue(merged?.status, ''),
    startedAt: chooseRichValue(merged?.startedAt, ''),
    completedAt: chooseRichValue(merged?.completedAt, ''),
    requestPayloadId: chooseRichValue(merged?.requestPayloadId, ''),
    responsePayloadId: chooseRichValue(merged?.responsePayloadId, ''),
    providerRequestPayloadId: chooseRichValue(merged?.providerRequestPayloadId, ''),
    providerResponsePayloadId: chooseRichValue(merged?.providerResponsePayloadId, ''),
    streamPayloadId: chooseRichValue(merged?.streamPayloadId, '')
  };
}

function mergeModelSteps(existing = [], incoming = []) {
  const a = Array.isArray(existing) ? existing : [];
  const b = Array.isArray(incoming) ? incoming : [];
  if (a.length === 0) return b.length > 0 ? [...b] : [];
  if (b.length === 0) return [...a];
  const out = a.map((entry, i) => i < b.length ? mergeModelStep(entry || {}, b[i] || {}) : entry);
  for (let i = a.length; i < b.length; i++) out.push(b[i]);
  return out;
}

function mergeUniqueEntries(existing = [], incoming = []) {
  const out = [];
  const seen = new Map();
  for (const list of [existing, incoming]) {
    for (const entry of Array.isArray(list) ? list : []) {
      const key = String(
        entry?.assistantMessageId
        || entry?.modelMessageId
        || entry?.messageId
        || entry?.toolCallId
        || entry?.toolMessageId
        || entry?.id
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
        out[found] = { ...out[found], ...entry };
      }
    }
  }
  return out;
}

function mergeExecutionGroupEntry(existing = {}, incoming = {}) {
  return {
    ...existing,
    ...incoming,
    assistantMessageId: chooseRichValue(incoming?.assistantMessageId, existing?.assistantMessageId, ''),
    parentMessageId: chooseRichValue(incoming?.parentMessageId, existing?.parentMessageId, ''),
    modelMessageId: chooseRichValue(incoming?.modelMessageId, existing?.modelMessageId, ''),
    sequence: chooseRichValue(incoming?.sequence, existing?.sequence, null),
    iteration: chooseRichValue(incoming?.iteration, existing?.iteration, null),
    preamble: chooseRichValue(incoming?.preamble, existing?.preamble, ''),
    content: chooseRichValue(incoming?.content, existing?.content, ''),
    finalResponse: Boolean(existing?.finalResponse || existing?.FinalResponse || incoming?.finalResponse || incoming?.FinalResponse),
    status: chooseRichValue(incoming?.status, existing?.status, ''),
    modelSteps: mergeModelSteps(
      existing?.modelSteps || [],
      incoming?.modelSteps || []
    ),
    toolSteps: mergeUniqueEntries(
      existing?.toolSteps,
      incoming?.toolSteps
    ),
    toolCallsPlanned: mergeUniqueEntries(existing?.toolCallsPlanned, incoming?.toolCallsPlanned),
    pageId: chooseRichValue(incoming?.pageId, existing?.pageId, incoming?.assistantMessageId, existing?.assistantMessageId, '')
  };
}

function mergeExecutionGroups(existing = [], incoming = []) {
  const out = [];
  const seen = new Map();
  for (const list of [existing, incoming]) {
    for (const group of Array.isArray(list) ? list : []) {
      const key = String(
        group?.assistantMessageId
        || group?.parentMessageId
        || group?.modelMessageId
        || ''
      ).trim();
      if (!key) {
        out.push(group);
        continue;
      }
      const found = seen.get(key);
      if (found == null) {
        seen.set(key, out.length);
        out.push(mergeExecutionGroupEntry({}, group));
      } else {
        out[found] = mergeExecutionGroupEntry(out[found], group);
      }
    }
  }
  return out.sort((left, right) => compareExecutionGroups(left, right));
}

function mergeRow(existing = {}, incoming = {}) {
  const mergedLinkedConversations = mergeUniqueEntries(
    existing?.linkedConversations,
    incoming?.linkedConversations
  );
  const mergedExecutionGroups = mergeExecutionGroups(existing?.executionGroups, incoming?.executionGroups);
  const mergedExecutionGroup = mergeExecutionGroups(
    existing?.executionGroup ? [existing.executionGroup] : [],
    incoming?.executionGroup ? [incoming.executionGroup] : []
  )[0] || null;

  // Preserve live-stream elicitation data that transcript rows may not carry.
  const elicitation = existing?.elicitation?.requestedSchema
    ? existing.elicitation
    : (incoming?.elicitation || existing?.elicitation || null);
  const elicitationId = chooseRichValue(existing?.elicitationId, incoming?.elicitationId, '');
  const callbackURL = chooseRichValue(existing?.callbackURL, incoming?.callbackURL, '');
  const sameAssistantTurn = String(existing?.role || '').toLowerCase() === 'assistant'
    && String(incoming?.role || '').toLowerCase() === 'assistant'
    && String(existing?.turnId || '').trim() !== ''
    && String(existing?.turnId || '').trim() === String(incoming?.turnId || '').trim();
  const existingInterim = Number(existing?.interim ?? 0) || 0;
  const incomingInterim = Number(incoming?.interim ?? 0) || 0;
  const replaceOpenAssistantBubble = sameAssistantTurn && existingInterim !== 0 && incomingInterim === 0;
  const preferIncomingAssistantContent = sameAssistantTurn
    && (
      (incomingInterim !== 0 && existingInterim !== 0)
      || replaceOpenAssistantBubble
    );
  const mergedContent = preferIncomingAssistantContent
    ? chooseRichValue(incoming?.content, existing?.content, '')
    : chooseRichValue(existing?.content, incoming?.content, '');
  const mergedPreamble = replaceOpenAssistantBubble
    ? chooseRichValue(existing?.preamble, incoming?.preamble, '')
    : preferIncomingAssistantContent
    ? chooseRichValue(incoming?.preamble, existing?.preamble, '')
    : chooseRichValue(existing?.preamble, incoming?.preamble, '');

  return {
    ...existing,
    ...incoming,
    id: replaceOpenAssistantBubble
      ? chooseRichValue(incoming?.id, existing?.id, '')
      : chooseRichValue(existing?.id, incoming?.id, ''),
    role: chooseRichValue(existing?.role, incoming?.role, ''),
    mode: chooseRichValue(incoming?.mode, existing?.mode, ''),
    turnId: chooseRichValue(existing?.turnId, incoming?.turnId, ''),
    turnStatus: chooseRichValue(incoming?.turnStatus, existing?.turnStatus, ''),
    status: chooseRichValue(incoming?.status, existing?.status, ''),
    type: chooseRichValue(existing?.type, incoming?.type, ''),
    createdAt: chooseRichValue(existing?.createdAt, incoming?.createdAt, ''),
    interim: chooseRichValue(existing?.interim, incoming?.interim, 0),
    content: mergedContent,
    rawContent: chooseRichValue(existing?.rawContent, incoming?.rawContent, ''),
    preamble: mergedPreamble,
    toolName: chooseRichValue(existing?.toolName, incoming?.toolName, ''),
    linkedConversationId: chooseRichValue(existing?.linkedConversationId, incoming?.linkedConversationId, ''),
    modelSteps: mergeUniqueEntries(existing?.modelSteps, incoming?.modelSteps),
    toolSteps: mergeUniqueEntries(existing?.toolSteps, incoming?.toolSteps),
    linkedConversations: mergedLinkedConversations,
    executionGroup: mergedExecutionGroup,
    executionGroups: mergedExecutionGroups,
    executions: mergeExecutions(existing?.executions, incoming?.executions),
    toolMessage: Array.isArray(existing?.toolMessage) && existing.toolMessage.length > 0 ? existing.toolMessage : incoming?.toolMessage || [],
    elicitation,
    elicitationId,
    callbackURL
  };
}

export function mergeRowSnapshots(previousRows = [], nextRows = []) {
  const out = [];
  const indexByID = new Map();
  const append = (row = {}) => {
    const key = String(row?.id || '').trim();
    if (!key) {
      out.push(row);
      return;
    }
    const found = indexByID.get(key);
    if (found == null) {
      indexByID.set(key, out.length);
      out.push(row);
    } else {
      out[found] = mergeRow(out[found], row);
    }
  };
  for (const row of Array.isArray(previousRows) ? previousRows : []) append(row);
  for (const row of Array.isArray(nextRows) ? nextRows : []) append(row);
  out.sort(compareTemporalEntries);
  return out;
}

function collapseAssistantRowsByTurn(rows = [], ownedTurnIds = new Set()) {
  const out = [];
  const openAssistantIndexByTurnId = new Map();
  for (const row of Array.isArray(rows) ? rows : []) {
    const role = String(row?.role || '').toLowerCase();
    const turnId = String(row?.turnId || '').trim();
    const hasExecution = Array.isArray(row?.executionGroups) && row.executionGroups.length > 0;
    const shouldCollapse = (role === 'assistant' || hasExecution)
      && turnId
      && (ownedTurnIds.size === 0 || ownedTurnIds.has(turnId));
    if (!shouldCollapse) {
      out.push(row);
      continue;
    }
    const found = openAssistantIndexByTurnId.get(turnId);
    if (found == null) {
      out.push(row);
      if (role === 'assistant' && Number(row?.interim || 0) !== 0) {
        openAssistantIndexByTurnId.set(turnId, out.length - 1);
      }
      continue;
    }
    out[found] = mergeRow(out[found], row);
    if (role === 'assistant' && Number(row?.interim || 0) === 0) {
      openAssistantIndexByTurnId.delete(turnId);
    }
  }
  out.sort(compareTemporalEntries);
  return out;
}

export function mergeRenderedRows({
  transcriptRows = [],
  liveRows = [],
  runningTurnId = '',
  hasRunning = false,
  findLatestRunningTurnId,
  currentConversationID = '',
  liveOwnedConversationID = '',
  liveOwnedTurnIds = []
} = {}) {
  const transcriptList = Array.isArray(transcriptRows) ? [...transcriptRows] : [];
  const normalizedCurrentConversationID = String(currentConversationID || '').trim();
  const normalizedLiveConversationID = String(liveOwnedConversationID || '').trim();
  const ownedTurnIds = new Set((Array.isArray(liveOwnedTurnIds) ? liveOwnedTurnIds : []).map((item) => String(item || '').trim()).filter(Boolean));
  const hasLiveSession = normalizedCurrentConversationID
    && normalizedLiveConversationID === normalizedCurrentConversationID
    && ((Array.isArray(liveRows) && liveRows.length > 0) || ownedTurnIds.size > 0);
  if (!hasRunning && !hasLiveSession) {
    return transcriptList.map((row) => ({ ...row, _rowSource: 'transcript' }));
  }

  const streamRows = (Array.isArray(liveRows) ? liveRows : []).filter((row) => String(row?._type || '').toLowerCase() === 'stream');
  const canonicalLiveRows = (Array.isArray(liveRows) ? liveRows : [])
    .filter((row) => String(row?._type || '').toLowerCase() !== 'stream')
    .map((row) => ({
      ...row,
      _rowSource: 'live',
      _bubbleSource: ownedTurnIds.has(String(row?.turnId || '').trim()) ? 'stream' : ''
    }));
  const transcriptBase = transcriptList
    .map((row) => {
      const turnId = String(row?.turnId || '').trim();
      // During an active live session, exclude transcript rows with no turnId.
      // They may belong to the current unfinished turn (e.g. user message
      // fetched before the server assigned a turnId) and will arrive via SSE.
      // Mixing them in causes the user message to appear after execution rows
      // because its transcript createdAt is independent of live row timestamps.
      if (hasLiveSession && !turnId) return null;
      if (!hasLiveSession || !ownedTurnIds.has(turnId)) {
        return { ...row, _rowSource: 'transcript' };
      }
      return null;
    })
    .filter(Boolean);
  const merged = hasLiveSession
    ? mergeRowSnapshots(canonicalLiveRows, transcriptBase)
    : mergeRowSnapshots(transcriptBase, canonicalLiveRows);
  if (streamRows.length === 0) {
    return hasLiveSession ? collapseAssistantRowsByTurn(merged, ownedTurnIds) : merged;
  }

  const activeTurnId = String(runningTurnId || findLatestRunningTurnId?.(merged) || '').trim();
  const rowHasFinalAssistantContent = (row = {}) => {
    const directContent = String(row?.content || '').trim();
    if (directContent) return true;
    const groups = Array.isArray(row?.executionGroups) ? row.executionGroups : [];
    return groups.some((group) => (
      Boolean(group?.finalResponse) && String(group?.content || '').trim() !== ''
    ));
  };
  const hasFinalAssistantForTurn = (turnId) => merged.some((row) => {
    const role = String(row?.role || '').toLowerCase();
    const sameTurn = String(row?.turnId || '').trim() === turnId;
    const nonInterim = Number(row?.interim || 0) === 0;
    const hasContent = rowHasFinalAssistantContent(row);
    return role === 'assistant' && sameTurn && nonInterim && hasContent;
  });
  const hasAssistantForStreamMessage = (streamMessageId) => {
    const id = String(streamMessageId || '').trim();
    if (!id) return false;
    return merged.some((row) => {
      const role = String(row?.role || '').toLowerCase();
      const rowId = String(row?.id || '').trim();
      const nonInterim = Number(row?.interim || 0) === 0;
      const hasContent = rowHasFinalAssistantContent(row);
      return role === 'assistant' && rowId === id && nonInterim && hasContent;
    });
  };
  const mergedIds = new Set(merged.map((row) => String(row?.id || '')).filter(Boolean));
  for (const streamRow of streamRows) {
    const turnId = String(streamRow?.turnId || activeTurnId).trim();
    const streamMessageId = String(streamRow?._streamMessageId || '').trim();
    // Skip stream rows that are already represented by a transcript assistant message.
    if (hasAssistantForStreamMessage(streamMessageId)) continue;
    if (!streamMessageId && turnId && hasFinalAssistantForTurn(turnId) && !hasRunning) continue;
    const streamId = String(streamRow?.id || '').trim();
    if (streamId && mergedIds.has(streamId)) continue;
    if (streamId) mergedIds.add(streamId);
    merged.push({
      ...streamRow,
      _rowSource: 'stream',
      turnId: turnId || streamRow?.turnId || '',
      turnStatus: String(streamRow?.turnStatus || (turnId ? 'running' : '')),
      status: streamRow?.status || (streamRow?.isStreaming === false ? 'completed' : 'streaming'),
      interim: Number(streamRow?.interim || 1) || 1,
      isStreaming: streamRow?.isStreaming !== false
    });
  }

  merged.sort(compareTemporalEntries);
  return hasLiveSession ? collapseAssistantRowsByTurn(merged, ownedTurnIds) : merged;
}
