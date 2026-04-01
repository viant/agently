import { mergeRowSnapshots } from './rowMerge';

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
  if (status === 'waiting_for_user' || status === 'blocked') return status;
  if (isTerminalStatusValue(status)) return 'running';
  return 'running';
}

function normalizedMessageIds(payload = {}) {
  const patch = payload?.patch && typeof payload.patch === 'object' ? payload.patch : {};
  return [
    payload?.id,
    payload?.assistantMessageId,
    payload?.modelCallId,
    patch?.id,
    patch?.assistantMessageId,
    patch?.modelCallId,
  ].map((value) => String(value || '').trim()).filter(Boolean);
}

function normalizedMode(payload = {}) {
  const patch = payload?.patch && typeof payload.patch === 'object' ? payload.patch : {};
  return String(payload?.mode || patch?.mode || '').trim().toLowerCase();
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
    const firstIsPlaceholder = !firstAssistantId
      && Array.isArray(first?.modelSteps)
      && first.modelSteps.length > 0
      && String(first?.status || '').trim() !== '';
    if (firstIsPlaceholder && secondAssistantId) {
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
  return out;
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

function buildCanonicalExecutionRow(payload = {}, fallbackConversationID = '') {
  const conversationID = String(payload?.conversationId || fallbackConversationID || '').trim();
  const turnId = String(payload?.turnId || '').trim();
  const assistantMessageId = String(payload?.assistantMessageId || '').trim();
  const rowID = assistantMessageId || `assistant:${turnId || conversationID}:${Number(payload?.iteration || payload?.pageIndex || 1) || 1}`;
  if (!rowID) return null;
  const createdAt = String(payload?.createdAt || new Date().toISOString()).trim();
  const finalResponse = !!payload?.finalResponse;
  const normalizedPayloadContent = normalizeStreamingMarkdown(String(payload?.content || '').trim()).content;
  const normalizedVisibleContent = normalizeStreamingMarkdown(String(payload?.preamble || payload?.content || '').trim()).content;
  const group = {
    pageId: assistantMessageId || rowID,
    assistantMessageId,
    parentMessageId: String(payload?.parentMessageId || '').trim(),
    iteration: Number(payload?.iteration || 0) || undefined,
    preamble: String(payload?.preamble || '').trim(),
    content: finalResponse ? normalizedPayloadContent : '',
    finalResponse,
    status: String(payload?.status || '').trim(),
    modelSteps: [{
      modelCallId: assistantMessageId,
      provider: String(payload?.model?.provider || '').trim(),
      model: String(payload?.model?.model || '').trim(),
      status: String(payload?.status || '').trim(),
      startedAt: payload?.startedAt || payload?.createdAt || new Date().toISOString(),
      completedAt: payload?.completedAt
        || (payload?.finalResponse ? (payload?.createdAt || new Date().toISOString()) : undefined)
        || (String(payload?.status || '').toLowerCase() === 'completed' ? (payload?.createdAt || new Date().toISOString()) : undefined),
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
    status: normalizeExecutionRowStatus(payload?.status),
    turnStatus: normalizeExecutionRowStatus(payload?.status),
    interim: finalResponse ? 0 : 1,
    content: finalResponse ? normalizedPayloadContent : normalizedVisibleContent,
    preamble: String(payload?.preamble || '').trim(),
    executionGroups: [group],
    executionGroupsTotal: Number(payload?.pageCount || 1) || 1,
    executionGroupsOffset: Math.max(0, (Number(payload?.pageCount || 1) || 1) - 1),
    executionGroupsLimit: 1
  };
}

function turnEventISO(payload = {}, fallback = Date.now()) {
  const parsed = Date.parse(String(payload?.createdAt || '').trim());
  if (Number.isFinite(parsed)) return new Date(parsed).toISOString();
  return new Date(fallback).toISOString();
}

function ensureLiveTurnRows(chatState = {}, payload = {}, fallbackConversationID = '') {
  const conversationID = String(payload?.conversationId || fallbackConversationID || '').trim();
  const turnId = String(payload?.turnId || '').trim();
  if (!turnId) return Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];

  const rows = Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];
  const turnStartedAt = turnEventISO(payload, Number(chatState?.activeStreamStartedAt || Date.now()));
  const prompt = String(chatState?.activeStreamPrompt || '').trim();
  const existingUserIndex = rows.findIndex((row) => String(row?.id || '').trim() === `user:${turnId}`);
  if (existingUserIndex === -1 && prompt) {
    rows.push({
      id: `user:${turnId}`,
      role: 'user',
      type: 'text',
      turnId,
      conversationId: conversationID,
      createdAt: new Date(Math.max(0, Date.parse(turnStartedAt) - 1)).toISOString(),
      interim: 0,
      status: 'completed',
      turnStatus: 'running',
      content: prompt,
      rawContent: prompt
    });
  }

  const assistantIndex = findAssistantRowIndex(rows, turnId, '');
  if (assistantIndex === -1) {
    rows.push({
      id: `assistant:${turnId}:live`,
      role: 'assistant',
      mode: String(payload?.mode || '').trim().toLowerCase(),
      turnId,
      conversationId: conversationID,
      agentIdUsed: String(payload?.agentIdUsed || '').trim(),
      agentName: String(payload?.agentName || '').trim(),
      createdAt: new Date(Date.parse(turnStartedAt) + 1).toISOString(),
      startedAt: turnStartedAt,
      interim: 1,
      status: String(payload?.status || 'running').trim(),
      turnStatus: String(payload?.status || 'running').trim(),
      content: '',
      executionGroups: [{
        assistantMessageId: '',
        parentMessageId: '',
        iteration: 1,
        preamble: '',
        content: '',
        finalResponse: false,
        status: String(payload?.status || 'running').trim(),
        startedAt: turnStartedAt,
        modelSteps: [{
          modelCallId: '',
          provider: '',
          model: '',
          status: String(payload?.status || 'running').trim(),
          startedAt: turnStartedAt
        }],
        toolSteps: [],
        toolCallsPlanned: []
      }],
      executionGroupsTotal: 1,
      executionGroupsOffset: 0,
      executionGroupsLimit: 1
    });
  } else {
    const row = { ...rows[assistantIndex] };
    row.agentIdUsed = String(payload?.agentIdUsed || row?.agentIdUsed || '').trim();
    row.agentName = String(payload?.agentName || row?.agentName || '').trim();
    row.turnStatus = String(payload?.status || row?.turnStatus || 'running').trim();
    row.status = String(payload?.status || row?.status || 'running').trim();
    row.startedAt = row.startedAt || turnStartedAt;
    rows[assistantIndex] = row;
  }

  rows.sort((a, b) => Date.parse(a.createdAt || 0) - Date.parse(b.createdAt || 0));
  chatState.liveRows = rows;
  return rows;
}

function applyExecutionStreamEventToRows(rows = [], payload = {}, fallbackConversationID = '') {
  const nextRow = buildCanonicalExecutionRow(payload, fallbackConversationID);
  if (!nextRow) return Array.isArray(rows) ? rows : [];
  const existing = Array.isArray(rows) ? [...rows] : [];
  const nextTurnId = String(nextRow.turnId || '').trim();
  const index = findAssistantRowIndex(existing, nextTurnId, nextRow.id);
  if (index === -1) {
    existing.push(nextRow);
  } else {
    const prev = existing[index];
    // Update content/status when finalResponse arrives; otherwise keep existing
    const updatedContent = nextRow.interim === 0 && String(nextRow.content || '').trim()
      ? nextRow.content
      : prev.content;
    const updatedInterim = nextRow.interim === 0 ? 0 : prev.interim;
    existing[index] = {
      ...prev,
      agentIdUsed: String(nextRow.agentIdUsed || prev?.agentIdUsed || '').trim(),
      agentName: String(nextRow.agentName || prev?.agentName || '').trim(),
      status: prev.status || nextRow.status,
      turnStatus: prev.turnStatus || nextRow.turnStatus,
      interim: updatedInterim,
      content: updatedContent,
      preamble: nextRow.preamble || prev.preamble,
      executionGroups: mergeCanonicalExecutionGroups(prev.executionGroups, nextRow.executionGroups)
    };
  }
  existing.sort((a, b) => Date.parse(a.createdAt || 0) - Date.parse(b.createdAt || 0));
  return existing;
}

function applyToolStreamEventToRows(rows = [], payload = {}, fallbackConversationID = '') {
  const assistantMessageId = String(payload?.assistantMessageId || '').trim();
  if (!assistantMessageId) return Array.isArray(rows) ? rows : [];
  const turnId = String(payload?.turnId || '').trim();
  const existing = Array.isArray(rows) ? [...rows] : [];
  const index = findAssistantRowIndex(existing, turnId, assistantMessageId);
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
    toolMessageId: String(payload?.toolMessageId || payload?.id || '').trim(),
    toolCallId: String(payload?.toolCallId || '').trim(),
    toolName: String(payload?.toolName || '').trim(),
    status: String(payload?.status || '').trim(),
    requestPayloadId: String(payload?.requestPayloadId || '').trim() || undefined,
    responsePayloadId: String(payload?.responsePayloadId || '').trim() || undefined,
    linkedConversationId: String(payload?.linkedConversationId || '').trim() || undefined,
    startedAt: payload?.createdAt || undefined,
    completedAt: payload?.type === 'tool_call_completed' ? (payload?.createdAt || undefined) : undefined
  };
  group.toolSteps = mergeCanonicalToolCalls(group.toolSteps, [toolStep]);
  const updatedGroups = [...groups];
  updatedGroups[groupIdx] = group;
  row.executionGroups = updatedGroups;
  // Propagate turnId to the row so later events (assistant_final, turn_completed)
  // can find this row by turnId even when model_started never fired.
  if (turnId && !row.turnId) {
    row.turnId = turnId;
  }
  existing[index] = row;
  return existing;
}

function applyMessagePatchToRows(rows = [], payload = {}) {
  const patch = payload?.patch && typeof payload.patch === 'object' ? payload.patch : {};
  const messageId = String(payload?.id || '').trim();
  const turnId = String(patch?.turnId || '').trim();
  if (!messageId && !turnId) return Array.isArray(rows) ? rows : [];

  const createdAt = String(patch?.createdAt || new Date().toISOString()).trim();
  const role = String(patch?.role || '').trim().toLowerCase();
  const messageType = String(patch?.messageType || '').trim();
  // For user messages, prefer rawContent (original query) over content
  // (which may be the expanded/internal prompt).
  const rawContent = String(patch?.rawContent || '').trim();
  const patchContent = normalizeStreamingMarkdown(rawContent || String(patch?.content || '').trim()).content;
  const baseRow = {
    id: messageId || `patch:${turnId}:${createdAt}`,
    role,
    mode: String(patch?.mode || '').trim().toLowerCase(),
    type: messageType,
    turnId,
    createdAt,
    status: String(patch?.status || '').trim(),
    turnStatus: String(patch?.status || '').trim(),
    interim: Number(patch?.interim ?? 0) || 0,
    content: patchContent,
    rawContent: rawContent,
    preamble: String(patch?.preamble || '').trim(),
    toolName: String(patch?.toolName || '').trim(),
    linkedConversationId: String(patch?.linkedConversationId || '').trim(),
    parentMessageId: String(patch?.parentMessageId || '').trim(),
    sequence: Number.isFinite(Number(patch?.sequence)) ? Number(patch.sequence) : null,
    iteration: Number.isFinite(Number(patch?.iteration)) ? Number(patch.iteration) : null
  };

  const existing = Array.isArray(rows) ? [...rows] : [];
  const filtered = existing.filter((row) => {
    if (String(row?._type || '').toLowerCase() !== 'stream') return true;
    const sameTurn = turnId && String(row?.turnId || '').trim() === turnId;
    if (!sameTurn) return true;
    const isExecutionEvidence = role === 'tool'
      || messageType === 'tool'
      || messageType === 'tool_op'
      || (role === 'assistant' && Number(baseRow.interim || 0) === 1 && (baseRow.content || baseRow.preamble));
    return !isExecutionEvidence;
  });
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
        mode: baseRow.mode || prev.mode,
        type: baseRow.type || prev.type,
        turnId,
        status: baseRow.status || prev.status,
        turnStatus: baseRow.turnStatus || prev.turnStatus,
        interim: baseRow.interim,
        content: patchContent || prev.content,
        rawContent: rawContent || String(prev.rawContent || ''),
        preamble: baseRow.preamble || prev.preamble,
        toolName: baseRow.toolName || prev.toolName,
        linkedConversationId: baseRow.linkedConversationId || prev.linkedConversationId,
        parentMessageId: baseRow.parentMessageId || prev.parentMessageId,
        sequence: baseRow.sequence ?? prev.sequence ?? null,
        iteration: baseRow.iteration ?? prev.iteration ?? null,
      };
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
      const patchPreamble = String(patch?.preamble || '').trim();
      let groupPatched = false;
      const updatedGroups = groups.map((group, index) => {
        const assistantMessageId = String(group?.assistantMessageId || '').trim();
        const shouldPatch = (messageId && assistantMessageId === messageId)
          || (!messageId && index === groups.length - 1);
        if (!shouldPatch) return group;
        groupPatched = true;
        return {
          ...group,
          preamble: patchPreamble || group?.preamble || '',
          content: patchContent || group?.content || '',
          finalResponse: patchInterim === 0 && patchContent !== '' ? true : !!group?.finalResponse,
          status: patchStatus || (patchInterim === 0 ? 'completed' : 'streaming') || group?.status || ''
        };
      });
      // Never set interim=0 from message_patch — the backend may
      // prematurely clear interim for tool-call responses. Only
      // assistant_final and turn_completed should mark a row as final.
      filtered[existingIdx] = {
        ...prev,
        mode: baseRow.mode || prev.mode,
        content: patchContent || prev.content,
        rawContent: rawContent || String(prev.rawContent || ''),
        preamble: String(patch?.preamble || '').trim() || prev.preamble,
        status: baseRow.status || prev.status,
        turnStatus: baseRow.turnStatus || prev.turnStatus,
        executionGroups: groupPatched ? updatedGroups : prev.executionGroups
      };
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
  return mergeRowSnapshots(filtered, [baseRow]);
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

// Detect whether accumulated streaming content looks like an LLM-generated
// elicitation JSON block.  When it does, suppress the raw JSON from the
// visible bubble so the user doesn't see the JSON flash before the
// elicitation overlay appears.
function looksLikeElicitationJSON(text) {
  if (!text || text.length < 20) return false;
  return /[`{]\s*"type"\s*:\s*"elicitation"/.test(text);
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
  const turnId = String(chatState.activeStreamTurnId || chatState.runningTurnId || '').trim();
  const streamMessageID = String(payload?.id || '').trim();
  const delta = String(payload?.content || '');
  if (!delta) return chatState.liveRows || [];

  const liveRows = Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];
  // Find the existing assistant execution row for this turn.
  const index = findAssistantRowIndex(liveRows, turnId, streamMessageID);
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
    // Suppress raw JSON when it looks like an LLM-generated elicitation block.
    row.content = looksLikeElicitationJSON(row._streamContent) ? '' : normalized.content;
    row.isStreaming = true;
    // Clear preamble once real content starts streaming — prevents
    // concatenated preamble+response in the bubble.
    if (row.preamble && row._streamContent.length > 0) {
      row.preamble = '';
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
    const visibleContent = looksLikeElicitationJSON(delta) ? '' : normalized.content;
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
      createdAt: payload?.createdAt || new Date().toISOString()
    });
    liveRows.sort((a, b) => Date.parse(a.createdAt || 0) - Date.parse(b.createdAt || 0));
  }
  chatState.liveRows = liveRows;
  return liveRows;
}

function applyAssistantFinalToRows(rows = [], payload = {}) {
  const turnId = String(payload?.turnId || '').trim();
  const content = String(payload?.content || '').trim();
  if (!turnId || !content) return Array.isArray(rows) ? rows : [];
  const existing = Array.isArray(rows) ? [...rows] : [];
  const index = findAssistantRowIndex(existing, turnId, String(payload?.assistantMessageId || '').trim());
  if (index === -1) return existing;
  const prev = existing[index];
  const groups = Array.isArray(prev.executionGroups) ? [...prev.executionGroups] : [];
  // Update the last execution group with final content — don't create new groups.
  if (groups.length > 0) {
    const last = groups[groups.length - 1];
    groups[groups.length - 1] = {
      ...last,
      content,
      finalResponse: true,
      status: String(payload?.status || last.status || '').trim()
    };
  }
  existing[index] = {
    ...prev,
    agentIdUsed: String(payload?.agentIdUsed || prev?.agentIdUsed || '').trim(),
    agentName: String(payload?.agentName || prev?.agentName || '').trim(),
    mode: String(payload?.mode || prev?.mode || '').trim().toLowerCase(),
    content: normalizeStreamingMarkdown(content).content,
    interim: 0,
    isStreaming: false,
    _streamContent: '',
    _streamFence: null,
    status: prev.status || normalizeExecutionRowStatus(payload?.status),
    turnStatus: prev.turnStatus || normalizeExecutionRowStatus(payload?.status),
    executionGroups: groups
  };
  return existing;
}

export function applyLinkedConversationEvent(chatState = {}, payload = {}) {
  const toolCallId = String(payload?.toolCallId || '').trim();
  const linkedConversationId = String(payload?.linkedConversationId || '').trim();
  const turnId = String(payload?.turnId || '').trim();
  if (!linkedConversationId || (!toolCallId && !turnId)) return chatState.liveRows || [];
  const rows = Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];
  for (let i = rows.length - 1; i >= 0; i--) {
    const row = rows[i];
    if (String(row?.role || '').toLowerCase() !== 'assistant') continue;
    if (turnId && String(row?.turnId || '').trim() !== turnId) continue;
    const groups = Array.isArray(row?.executionGroups) ? row.executionGroups : [];
    let matched = false;
    const updatedGroups = groups.map((group) => {
      const steps = Array.isArray(group?.toolSteps) ? group.toolSteps : [];
      const updatedSteps = steps.map((step) => {
        const stepToolCallId = String(step?.toolCallId || '').trim();
        const stepToolMessageId = String(step?.toolMessageId || '').trim();
        // Match by toolCallId (OpID) or toolMessageId — the linked_conversation_attached
        // event may carry either depending on what's in context.
        if (toolCallId && (stepToolCallId === toolCallId || stepToolMessageId === toolCallId)) {
          matched = true;
          return { ...step, linkedConversationId };
        }
        return step;
      });
      return matched ? { ...group, toolSteps: updatedSteps } : group;
    });
    if (matched) {
      rows[i] = { ...row, executionGroups: updatedGroups };
      break;
    }
  }
  chatState.liveRows = rows;
  return rows;
}

export function applyElicitationRequestedEvent(chatState = {}, payload = {}) {
  const turnId = String(payload?.turnId || '').trim();
  const assistantMessageId = String(payload?.assistantMessageId || '').trim();
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
  const index = findAssistantRowIndex(rows, turnId, assistantMessageId);
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
      createdAt: payload?.createdAt || new Date().toISOString()
    });
    rows.sort((a, b) => Date.parse(a.createdAt || 0) - Date.parse(b.createdAt || 0));
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
  const assistantMessageId = String(payload?.assistantMessageId || '').trim();
  const preamble = String(payload?.content || payload?.preamble || '').trim();
  if (!preamble) return chatState.liveRows || [];

  const rows = Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];
  const index = findAssistantRowIndex(rows, turnId, assistantMessageId);
  if (index >= 0) {
    const prev = rows[index];
    const groups = Array.isArray(prev.executionGroups) ? [...prev.executionGroups] : [];
    if (groups.length > 0) {
      const last = { ...groups[groups.length - 1] };
      last.preamble = preamble;
      groups[groups.length - 1] = last;
    }
    rows[index] = {
      ...prev,
      agentIdUsed: String(payload?.agentIdUsed || prev?.agentIdUsed || '').trim(),
      agentName: String(payload?.agentName || prev?.agentName || '').trim(),
      preamble,
      executionGroups: groups
    };
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
      content: preamble,
      preamble,
      executionGroups: [{
        assistantMessageId,
        preamble,
        iteration: Number(payload?.iteration || 0) || undefined
      }],
      createdAt: payload?.createdAt || new Date().toISOString()
    });
    rows.sort((a, b) => Date.parse(a.createdAt || 0) - Date.parse(b.createdAt || 0));
  }
  chatState.liveRows = rows;
  return rows;
}

export function applyAssistantFinalEvent(chatState = {}, payload = {}) {
  if (isSuppressedSummaryEvent(chatState, payload)) {
    rememberSuppressedSummary(chatState, payload);
    return chatState.liveRows || [];
  }
  const nextRows = applyAssistantFinalToRows(chatState.liveRows, payload);
  chatState.liveRows = nextRows;
  return nextRows;
}

export function applyExecutionStreamEvent(chatState = {}, payload = {}, fallbackConversationID = '') {
  if (isSuppressedSummaryEvent(chatState, payload)) {
    rememberSuppressedSummary(chatState, payload);
    return chatState.liveRows || [];
  }
  const nextRows = applyExecutionStreamEventToRows(chatState.liveRows, payload, fallbackConversationID);
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
  const existing = Array.isArray(chatState.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [];
  if (!nextTurnID) {
    chatState.liveOwnedTurnIds = existing;
    return existing;
  }
  if (existing.includes(nextTurnID)) {
    chatState.liveOwnedTurnIds = existing;
    return existing;
  }
  const next = [...existing, nextTurnID];
  chatState.liveOwnedTurnIds = next;
  return next;
}

export function finalizeStreamTurn(chatState = {}, payload = {}, fallbackConversationID = '') {
  const turnID = String(payload?.turnId || chatState.activeStreamTurnId || chatState.runningTurnId || '').trim();
  const content = String(payload?.content || '').trim();
  const status = String(payload?.status || 'completed').trim() || 'completed';
  const rows = Array.isArray(chatState.liveRows) ? [...chatState.liveRows] : [];

  // Find the assistant execution row for this turn.
  for (let index = rows.length - 1; index >= 0; index -= 1) {
    const row = rows[index];
    const rowTurnID = String(row?.turnId || '').trim();
    if (turnID && rowTurnID && rowTurnID !== turnID) continue;
    const role = String(row?.role || '').toLowerCase();
    if (role !== 'assistant') continue;

    // Use explicit content > stream content > existing content.
    const streamContent = String(row?._streamContent || '').trim();
    const finalizedContent = normalizeStreamingMarkdown(content || streamContent || String(row?.content || '').trim()).content;

    const groups = Array.isArray(row?.executionGroups) ? row.executionGroups : [];
    rows[index] = {
      ...row,
      mode: String(payload?.mode || row?.mode || '').trim().toLowerCase(),
      status,
      turnStatus: 'completed',
      interim: 0,
      isStreaming: false,
      content: finalizedContent,
      _streamContent: '',
      _streamFence: null,
      executionGroups: groups.map((group) => {
        // Stamp completedAt on model steps that have startedAt but no completedAt
        // so the elapsed time displays correctly after finalization.
        const nowISO = new Date().toISOString();
        const modelSteps = Array.isArray(group?.modelSteps)
          ? group.modelSteps.map((ms) => {
              if (ms?.startedAt && !ms?.completedAt) {
                return { ...ms, completedAt: nowISO, status };
              }
              return ms?.status ? ms : { ...ms, status };
            })
          : group?.modelSteps;
        return {
          ...group,
          status,
          finalResponse: finalizedContent ? true : Boolean(group?.finalResponse || group?.FinalResponse),
          content: finalizedContent || String(group?.content || ''),
          modelSteps
        };
      })
    };
    break;
  }

  chatState.liveRows = rows;
  chatState.activeStreamTurnId = '';
  chatState.activeStreamStartedAt = 0;
  chatState.activeStreamPrompt = '';
  return rows;
}
