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
    linkedConversationId: chooseRichValue(incoming?.linkedConversationId, existing?.linkedConversationId, incoming?.LinkedConversationId, existing?.LinkedConversationId, ''),
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
    Provider: chooseRichValue(incoming?.Provider, incoming?.provider, existing?.Provider, existing?.provider, ''),
    Model: chooseRichValue(incoming?.Model, incoming?.model, existing?.Model, existing?.model, ''),
    Status: chooseRichValue(incoming?.Status, incoming?.status, existing?.Status, existing?.status, ''),
    StartedAt: chooseRichValue(incoming?.StartedAt, incoming?.startedAt, existing?.StartedAt, existing?.startedAt, ''),
    CompletedAt: chooseRichValue(incoming?.CompletedAt, incoming?.completedAt, existing?.CompletedAt, existing?.completedAt, ''),
    RequestPayloadId: chooseRichValue(incoming?.RequestPayloadId, incoming?.requestPayloadId, existing?.RequestPayloadId, existing?.requestPayloadId, ''),
    ResponsePayloadId: chooseRichValue(incoming?.ResponsePayloadId, incoming?.responsePayloadId, existing?.ResponsePayloadId, existing?.responsePayloadId, ''),
    ProviderRequestPayloadId: chooseRichValue(incoming?.ProviderRequestPayloadId, incoming?.providerRequestPayloadId, existing?.ProviderRequestPayloadId, existing?.providerRequestPayloadId, ''),
    ProviderResponsePayloadId: chooseRichValue(incoming?.ProviderResponsePayloadId, incoming?.providerResponsePayloadId, existing?.ProviderResponsePayloadId, existing?.providerResponsePayloadId, ''),
    StreamPayloadId: chooseRichValue(incoming?.StreamPayloadId, incoming?.streamPayloadId, existing?.StreamPayloadId, existing?.streamPayloadId, ''),
    ModelCallRequestPayload: incoming?.ModelCallRequestPayload ?? existing?.ModelCallRequestPayload ?? null,
    ModelCallResponsePayload: incoming?.ModelCallResponsePayload ?? existing?.ModelCallResponsePayload ?? null,
    ModelCallProviderRequestPayload: incoming?.ModelCallProviderRequestPayload ?? existing?.ModelCallProviderRequestPayload ?? null,
    ModelCallProviderResponsePayload: incoming?.ModelCallProviderResponsePayload ?? existing?.ModelCallProviderResponsePayload ?? null,
    ModelCallStreamPayload: incoming?.ModelCallStreamPayload ?? existing?.ModelCallStreamPayload ?? null
  };
}

function mergeModelStep(existing = {}, incoming = {}) {
  const merged = mergeModelCall(existing, incoming);
  return {
    ...merged,
    provider: chooseRichValue(merged?.Provider, merged?.provider, ''),
    model: chooseRichValue(merged?.Model, merged?.model, ''),
    status: chooseRichValue(merged?.Status, merged?.status, ''),
    startedAt: chooseRichValue(merged?.StartedAt, merged?.startedAt, ''),
    completedAt: chooseRichValue(merged?.CompletedAt, merged?.completedAt, ''),
    requestPayloadId: chooseRichValue(merged?.RequestPayloadId, merged?.requestPayloadId, ''),
    responsePayloadId: chooseRichValue(merged?.ResponsePayloadId, merged?.responsePayloadId, ''),
    providerRequestPayloadId: chooseRichValue(merged?.ProviderRequestPayloadId, merged?.providerRequestPayloadId, ''),
    providerResponsePayloadId: chooseRichValue(merged?.ProviderResponsePayloadId, merged?.providerResponsePayloadId, ''),
    streamPayloadId: chooseRichValue(merged?.StreamPayloadId, merged?.streamPayloadId, '')
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
        || entry?.AssistantMessageId
        || entry?.modelMessageId
        || entry?.ModelMessageId
        || entry?.messageId
        || entry?.MessageId
        || entry?.toolCallId
        || entry?.ToolCallId
        || entry?.toolMessageId
        || entry?.ToolMessageId
        || entry?.id
        || entry?.Id
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
    assistantMessageId: chooseRichValue(incoming?.assistantMessageId, existing?.assistantMessageId, incoming?.AssistantMessageId, existing?.AssistantMessageId, ''),
    parentMessageId: chooseRichValue(incoming?.parentMessageId, existing?.parentMessageId, incoming?.ParentMessageID, existing?.ParentMessageID, ''),
    modelMessageId: chooseRichValue(incoming?.modelMessageId, existing?.modelMessageId, incoming?.ModelMessageID, existing?.ModelMessageID, ''),
    sequence: chooseRichValue(incoming?.sequence, existing?.sequence, null),
    iteration: chooseRichValue(incoming?.iteration, existing?.iteration, null),
    preamble: chooseRichValue(incoming?.preamble, existing?.preamble, incoming?.Preamble, existing?.Preamble, ''),
    content: chooseRichValue(incoming?.content, existing?.content, incoming?.Content, existing?.Content, ''),
    finalResponse: Boolean(existing?.finalResponse || existing?.FinalResponse || incoming?.finalResponse || incoming?.FinalResponse),
    status: chooseRichValue(incoming?.status, existing?.status, incoming?.Status, existing?.Status, ''),
    modelSteps: mergeModelSteps(
      existing?.modelSteps || (existing?.modelCall ? [existing.modelCall] : []),
      incoming?.modelSteps || (incoming?.modelCall ? [incoming.modelCall] : [])
    ),
    toolSteps: mergeUniqueEntries(
      existing?.toolSteps || existing?.toolCalls || existing?.ToolCalls,
      incoming?.toolSteps || incoming?.toolCalls || incoming?.ToolCalls
    ),
    toolCallsPlanned: mergeUniqueEntries(existing?.toolCallsPlanned || existing?.ToolCallsPlanned, incoming?.toolCallsPlanned || incoming?.ToolCallsPlanned),
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
        || group?.AssistantMessageId
        || group?.parentMessageId
        || group?.ParentMessageID
        || group?.modelMessageId
        || group?.ModelMessageID
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
  return out;
}

function mergeRow(existing = {}, incoming = {}) {
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

  return {
    ...existing,
    ...incoming,
    id: chooseRichValue(existing?.id, incoming?.id, ''),
    role: chooseRichValue(existing?.role, incoming?.role, ''),
    mode: chooseRichValue(incoming?.mode, existing?.mode, ''),
    turnId: chooseRichValue(existing?.turnId, incoming?.turnId, ''),
    turnStatus: chooseRichValue(incoming?.turnStatus, existing?.turnStatus, ''),
    status: chooseRichValue(incoming?.status, existing?.status, ''),
    type: chooseRichValue(existing?.type, incoming?.type, ''),
    createdAt: chooseRichValue(existing?.createdAt, incoming?.createdAt, new Date().toISOString()),
    interim: chooseRichValue(existing?.interim, incoming?.interim, 0),
    content: chooseRichValue(existing?.content, incoming?.content, ''),
    rawContent: chooseRichValue(existing?.rawContent, incoming?.rawContent, ''),
    preamble: chooseRichValue(existing?.preamble, incoming?.preamble, ''),
    toolName: chooseRichValue(existing?.toolName, incoming?.toolName, ''),
    linkedConversationId: chooseRichValue(existing?.linkedConversationId, incoming?.linkedConversationId, ''),
    modelCall: existing?.modelCall || incoming?.modelCall || null,
    modelSteps: mergeUniqueEntries(existing?.modelSteps, incoming?.modelSteps),
    toolSteps: mergeUniqueEntries(existing?.toolSteps, incoming?.toolSteps),
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
  out.sort((a, b) => Date.parse(a.createdAt || 0) - Date.parse(b.createdAt || 0));
  return out;
}

function collapseAssistantRowsByTurn(rows = [], ownedTurnIds = new Set()) {
  const out = [];
  const indexByTurnId = new Map();
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
    const found = indexByTurnId.get(turnId);
    if (found == null) {
      indexByTurnId.set(turnId, out.length);
      out.push(row);
      continue;
    }
    out[found] = mergeRow(out[found], row);
  }
  out.sort((a, b) => Date.parse(a.createdAt || 0) - Date.parse(b.createdAt || 0));
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
  const hasFinalAssistantForTurn = (turnId) => merged.some((row) => {
    const role = String(row?.role || '').toLowerCase();
    const sameTurn = String(row?.turnId || '').trim() === turnId;
    const nonInterim = Number(row?.interim || 0) === 0;
    const hasContent = String(row?.content || '').trim() !== '';
    return role === 'assistant' && sameTurn && nonInterim && hasContent;
  });
  const hasAssistantForStreamMessage = (streamMessageId) => {
    const id = String(streamMessageId || '').trim();
    if (!id) return false;
    return merged.some((row) => {
      const role = String(row?.role || '').toLowerCase();
      const rowId = String(row?.id || '').trim();
      const nonInterim = Number(row?.interim || 0) === 0;
      const hasContent = String(row?.content || '').trim() !== '';
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

  merged.sort((a, b) => Date.parse(a.createdAt || 0) - Date.parse(b.createdAt || 0));
  return hasLiveSession ? collapseAssistantRowsByTurn(merged, ownedTurnIds) : merged;
}
