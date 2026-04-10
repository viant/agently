import { compareExecutionGroups, compareTemporalEntries } from 'agently-core-ui-sdk';

export function classifyMessage(message) {
  if (!message) return 'bubble';
  if (message._type === 'starter') return 'starter';
  if (message._type === 'queue') return 'queue';
  if (message._type === 'iteration') return 'iteration';
  if (String(message.mode || '').trim().toLowerCase() === 'summary') return undefined;
  if (message.status === 'summarized') return undefined;
  if (message.status === 'summary') return undefined;

  // Elicitation handling — match original agently logic.
  // Consider forms with captured user payload or terminal status as resolved.
  const hasUED = !!(message?.userElicitationData || message?.userData);
  const hasPayloadId = !!(message?.elicitationPayloadId);
  const stLower = String(message.status || '').toLowerCase();
  const isResolved = hasUED || hasPayloadId
    || ['accepted', 'done', 'succeeded', 'success', 'failed', 'error', 'canceled', 'declined'].includes(stLower);

  // Role-based elicitation (role === 'elicition' from backend)
  if (message.role === 'elicition' && (stLower === 'open' || stLower === 'pending') && !isResolved) {
    return 'elicition';
  }

  // Schema-based elicitation with callbackURL (modal form)
  const schema = message.elicitation?.requestedSchema;
  if (schema && typeof message.callbackURL === 'string' && message.callbackURL) {
    if (!isResolved && (stLower === 'open' || stLower === 'pending')) {
      return 'elicition';
    }
  }

  // Inline form for schema elicitations without callbackURL
  if (schema && !isResolved) return 'form';

  // Resolved elicitations fall through to bubble
  return 'bubble';
}

const SYNTHETIC_RENDER_TYPES = new Set(['iteration', 'queue']);

function mergeGeneratedFiles(...lists) {
  const out = [];
  const seen = new Set();
  for (const list of lists) {
    if (!Array.isArray(list) || list.length === 0) continue;
    for (const item of list) {
      const id = String(item?.id || item?.ID || '').trim();
      if (!id || seen.has(id)) continue;
      seen.add(id);
      out.push(item);
    }
  }
  return out;
}

function debugIterationsEnabled() {
  if (typeof window === 'undefined') return false;
  try {
    return String(window.localStorage?.getItem('agently.debugIterations') || '').trim() === '1';
  } catch (_) {
    return false;
  }
}

function debugIterationTimeline(stage, payload = {}) {
  if (!debugIterationsEnabled()) return;
  const stamp = new Date().toISOString();
  // eslint-disable-next-line no-console
  console.log(`[iteration:${stage}]`, { time: stamp, ...payload });
}

export function normalizeMessages(raw = [], options = {}) {
  if (!Array.isArray(raw)) return [];
  const visibleCount = Number(options?.visibleCount || Number.MAX_SAFE_INTEGER);
  const hasSyntheticRows = raw.some((item) => {
    const kind = String(item?._type || '').toLowerCase();
    return SYNTHETIC_RENDER_TYPES.has(kind) || !!item?._iterationData;
  });
  if (hasSyntheticRows) {
    const preservedQueueRows = raw
      .filter((item) => String(item?.status || '').toLowerCase() !== 'summarized')
      .filter((item) => String(item?._type || '').toLowerCase() === 'queue');
    const rebuiltBase = raw
      .filter((item) => String(item?.status || '').toLowerCase() !== 'summarized')
      .filter((item) => {
        const kind = String(item?._type || '').toLowerCase();
        return !SYNTHETIC_RENDER_TYPES.has(kind) && !item?._iterationData;
      })
      .map((item) => {
        return normalizeOne(item);
      })
      .sort(compareTemporalEntries);
    const synthesized = synthesizeIterationMessages(collapseDuplicateUserRows(rebuiltBase), visibleCount);
    return [...synthesized, ...preservedQueueRows]
      .sort(compareTemporalEntries);
  }
  const normalized = raw
    .filter((item) => {
      const kind = String(item?._type || '').toLowerCase();
      if (kind === 'paginator') return false;
      if (kind === 'iteration' && !item?._iterationData?.optimistic) return false;
      if (String(item?.role || '').trim().toLowerCase() === 'user' && String(item?.type || '').trim().toLowerCase() === 'elicitation_response') return false;
      return true;
    })
    .filter((item) => String(item?.status || '').toLowerCase() !== 'summarized')
    .map((item) => normalizeOne(item))
    .sort(compareTemporalEntries);
  return synthesizeIterationMessages(collapseDuplicateUserRows(normalized), visibleCount);
}

export function normalizeOne(message = {}) {
  const role = String(message.role || '').toLowerCase();
  const turnId = message.turnId || '';
  const mode = String(message.mode || '').trim().toLowerCase();
  const content = normalizeVisibleContent({
    role,
    mode,
    content: pickString(
    message.rawContent,
    message.content,
    ''
    )
  });
  const embeddedElicitation = extractEmbeddedElicitation(content);
  const createdAt = normalizeTimestamp(message.createdAt || message.CreatedAt);
  const interim = Number(message.interim ?? message.Interim ?? 0) || 0;
  const userElicitationData = message.userElicitationData || message.UserElicitationData || null;
  const elicitationPayloadId = String(message.elicitationPayloadId || message.ElicitationPayloadId || '').trim();
  const elicitation = normalizeElicitation(
    mergeEmbeddedElicitation(
      message.elicitation || message.Elicitation,
      embeddedElicitation,
      message
    )
  );
  const iterationRaw = message.iteration;
  const iterationNum = Number(iterationRaw);
  const iteration = Number.isFinite(iterationNum) && iterationNum > 0 ? iterationNum : null;
  const modelCall = message.modelCall || message.ModelCall || null;
  const toolCall = message.toolCall || message.ToolCall || null;
  const toolMessage = Array.isArray(message.toolMessage || message.ToolMessage)
    ? (message.toolMessage || message.ToolMessage)
    : [];

  return {
    ...message,
    id: message.id || message.messageId || '',
    role,
    turnId,
    content,
    createdAt,
    mode,
    interim,
    iteration,
    status: message.status || '',
    turnStatus: message.turnStatus || '',
    errorMessage: pickString(
      message.errorMessage,
      message.statusMessage,
      message.StatusMessage
    ),
    userElicitationData,
    elicitationPayloadId,
    modelCall,
    toolCall,
    toolMessage,
    executions: Array.isArray(message.executions) ? message.executions : [],
    executionGroup: message.executionGroup || null,
    executionGroups: Array.isArray(message.executionGroups)
      ? message.executionGroups
      : [],
    executionGroupsTotal: Number(message.executionGroupsTotal || 0) || 0,
    executionGroupsOffset: Number(message.executionGroupsOffset || 0) || 0,
    executionGroupsLimit: Number(message.executionGroupsLimit || 0) || 0,
    elicitation,
    elicitationId: message.elicitationId || elicitation?.elicitationId || '',
    requestPayload: message.requestPayload || null,
    responsePayload: message.responsePayload || null
  };
}

function normalizeVisibleContent({ role = '', mode = '', content = '' } = {}) {
  const text = String(content || '');
  if (role !== 'user') return text;
  if (mode !== 'task') return text;
  return extractUserTaskPrompt(text) || text;
}

function userVisibleSignature(message = {}) {
  if (String(message?.role || '').trim().toLowerCase() !== 'user') return '';
  const turnId = String(message?.turnId || '').trim();
  const content = String(message?.content || '').trim();
  if (!turnId || !content) return '';
  return `${turnId}::${content}`;
}

function collapseDuplicateUserRows(messages = []) {
  const out = [];
  const seenUserBySignature = new Map();
  for (const message of Array.isArray(messages) ? messages : []) {
    const signature = userVisibleSignature(message);
    if (!signature) {
      out.push(message);
      continue;
    }
    const existingIndex = seenUserBySignature.get(signature);
    if (existingIndex == null) {
      seenUserBySignature.set(signature, out.length);
      out.push(message);
      continue;
    }
    const existing = out[existingIndex] || {};
    const existingTime = Date.parse(String(existing?.createdAt || ''));
    const incomingTime = Date.parse(String(message?.createdAt || ''));
    const keepExisting = Number.isFinite(existingTime) && Number.isFinite(incomingTime)
      ? existingTime <= incomingTime
      : true;
    out[existingIndex] = keepExisting
      ? {
          ...message,
          ...existing,
          id: existing?.id || message?.id,
          createdAt: existing?.createdAt || message?.createdAt || '',
          rawContent: existing?.rawContent || message?.rawContent || '',
          mode: existing?.mode || message?.mode || ''
        }
      : {
          ...existing,
          ...message,
          id: message?.id || existing?.id,
          createdAt: message?.createdAt || existing?.createdAt || '',
          rawContent: message?.rawContent || existing?.rawContent || '',
          mode: message?.mode || existing?.mode || ''
        };
  }
  return out;
}

function extractUserTaskPrompt(text = '') {
  const raw = String(text || '');
  if (!raw) return '';
  const trimmed = raw.trim();
  if (!trimmed.startsWith('User Query:')) return '';
  const lines = raw.split(/\r?\n/);
  let collecting = false;
  const collected = [];
  for (const line of lines) {
    const current = String(line || '');
    const trimmedLine = current.trim();
    if (!collecting) {
      if (trimmedLine === 'User Query:') {
        collecting = true;
      } else if (trimmedLine.startsWith('User Query:')) {
        const remainder = trimmedLine.slice('User Query:'.length).trim();
        if (remainder) collected.push(remainder);
        collecting = true;
      }
      continue;
    }
    if (trimmedLine === 'Context:' || trimmedLine.startsWith('Context:')) {
      break;
    }
    collected.push(current);
  }
  const result = collected.join('\n').trim();
  return result;
}

function normalizeElicitation(value = null) {
  if (!value || typeof value !== 'object') return null;
  const requestedSchema = value.requestedSchema || value.RequestedSchema || null;
  const elicitationId = String(value.elicitationId || '').trim();
  if (!requestedSchema || !elicitationId) return null;
  return {
    ...value,
    elicitationId,
    message: String(value.message || value.prompt || '').trim(),
    requestedSchema,
    callbackURL: String(value.callbackURL || '').trim()
  };
}

function mergeEmbeddedElicitation(explicit = null, embedded = null, message = {}) {
  const merged = {
    ...(embedded && typeof embedded === 'object' ? embedded : {}),
    ...(explicit && typeof explicit === 'object' ? explicit : {})
  };
  const elicitationId = String(
    merged.elicitationId
    || message?.elicitationId
    || ''
  ).trim();
  if (elicitationId) {
    merged.elicitationId = elicitationId;
  }
  const callbackURL = String(
    merged.callbackURL
    || message?.callbackURL
    || ''
  ).trim();
  if (callbackURL) {
    merged.callbackURL = callbackURL;
  }
  return merged;
}

function extractEmbeddedElicitation(text = '') {
  const raw = String(text || '').trim();
  if (!raw) return null;
  let candidate = raw;
  try {
    const fence = raw.match(/```(?:json)?\s*([\s\S]*?)\s*```/i);
    if (fence && fence[1]) candidate = String(fence[1]).trim();
  } catch (_) {}
  const objectStart = candidate.indexOf('{');
  const objectEnd = candidate.lastIndexOf('}');
  if (objectStart === -1 || objectEnd === -1 || objectEnd <= objectStart) {
    return null;
  }
  candidate = candidate.slice(objectStart, objectEnd + 1).trim();
  try {
    const parsed = JSON.parse(candidate);
    if (!parsed || typeof parsed !== 'object') return null;
    if (String(parsed.type || '').toLowerCase() !== 'elicitation') return null;
    return {
      message: String(parsed.message || '').trim(),
      requestedSchema: parsed.requestedSchema || null
    };
  } catch (_) {
    return null;
  }
}

function chooseRichString(...values) {
  for (const value of values) {
    if (typeof value === 'string' && value.trim() !== '') return value;
  }
  return '';
}

function normalizeAgentId(value = '') {
  const text = String(value || '').trim();
  if (!text) return '';
  if (text.startsWith('anonymous:')) return '';
  return text;
}

function mergeStepFields(existing = {}, incoming = {}) {
  const merged = { ...existing, ...incoming };
  merged.id = chooseRichString(incoming?.id, existing?.id);
  merged.kind = chooseRichString(incoming?.kind, existing?.kind);
  merged.reason = chooseRichString(incoming?.reason, existing?.reason);
  merged.toolName = chooseRichString(incoming?.toolName, existing?.toolName);
  merged.provider = chooseRichString(incoming?.provider, existing?.provider);
  merged.model = chooseRichString(incoming?.model, existing?.model);
  merged.status = chooseRichString(incoming?.status, existing?.status);
  merged.requestPayloadId = chooseRichString(incoming?.requestPayloadId, existing?.requestPayloadId);
  merged.responsePayloadId = chooseRichString(incoming?.responsePayloadId, existing?.responsePayloadId);
  merged.providerRequestPayloadId = chooseRichString(incoming?.providerRequestPayloadId, existing?.providerRequestPayloadId);
  merged.providerResponsePayloadId = chooseRichString(incoming?.providerResponsePayloadId, existing?.providerResponsePayloadId);
  merged.streamPayloadId = chooseRichString(incoming?.streamPayloadId, existing?.streamPayloadId);
  merged.linkedConversationId = chooseRichString(incoming?.linkedConversationId, existing?.linkedConversationId);
  merged.latencyMs = Number.isFinite(Number(existing?.latencyMs)) && Number(existing?.latencyMs) > 0
    ? existing.latencyMs
    : (Number.isFinite(Number(incoming?.latencyMs)) ? incoming.latencyMs : null);
  merged.requestPayload = incoming?.requestPayload ?? existing?.requestPayload ?? null;
  merged.responsePayload = incoming?.responsePayload ?? existing?.responsePayload ?? null;
  merged.providerRequestPayload = incoming?.providerRequestPayload ?? existing?.providerRequestPayload ?? null;
  merged.providerResponsePayload = incoming?.providerResponsePayload ?? existing?.providerResponsePayload ?? null;
  merged.streamPayload = incoming?.streamPayload ?? existing?.streamPayload ?? null;
  return merged;
}

function mergePreamble(existing = null, incoming = null) {
  if (!existing) return incoming;
  if (!incoming) return existing;
  return {
    ...existing,
    ...incoming,
    content: chooseRichString(incoming?.content, existing?.content),
    status: chooseRichString(incoming?.status, existing?.status),
    turnStatus: chooseRichString(incoming?.turnStatus, existing?.turnStatus),
    steps: mergeStepList(existing?.steps, incoming?.steps)
  };
}

function stepIdentity(step = {}) {
  const explicitID = String(step?.id || '').trim();
  if (explicitID) return explicitID;
  const requestId = String(step?.requestPayloadId || '');
  const responseId = String(step?.responsePayloadId || '');
  const providerRequestId = String(step?.providerRequestPayloadId || '');
  const providerResponseId = String(step?.providerResponsePayloadId || '');
  const streamId = String(step?.streamPayloadId || '');
  if (requestId || responseId || providerRequestId || providerResponseId || streamId) {
    return [
      String(step?.kind || ''),
      String(step?.toolName || ''),
      requestId,
      responseId,
      providerRequestId,
      providerResponseId,
      streamId
    ].join('::');
  }
  return [
    String(step?.id || ''),
    String(step?.kind || ''),
    String(step?.toolName || ''),
    String(step?.reason || '')
  ].join('::');
}

function mergeStepList(existing = [], incoming = []) {
  const out = [];
  const indexByKey = new Map();
  for (const list of [existing, incoming]) {
    for (const step of Array.isArray(list) ? list : []) {
      const key = stepIdentity(step);
      if (!key.trim()) {
        out.push(step);
        continue;
      }
      const found = indexByKey.get(key);
      if (found == null) {
        indexByKey.set(key, out.length);
        out.push(step);
        continue;
      }
      out[found] = mergeStepFields(out[found], step);
    }
  }
  return out;
}

function mergeIterationItems(existing = {}, incoming = {}) {
  const merged = {
    ...existing,
    ...incoming,
    turnId: chooseRichString(existing?.turnId, incoming?.turnId),
    agentId: chooseRichString(incoming?.agentId, existing?.agentId),
    status: chooseRichString(incoming?.status, existing?.status),
    errorMessage: chooseRichString(incoming?.errorMessage, existing?.errorMessage),
    streamContent: chooseRichString(incoming?.streamContent, existing?.streamContent),
    summary: incoming?.summary || existing?.summary || null,
    response: incoming?.response || existing?.response || null,
    preamble: mergePreamble(existing?.preamble, incoming?.preamble),
    preambles: [],
    toolCalls: mergeStepList(existing?.toolCalls, incoming?.toolCalls),
    linkedConversations: [],
    executionGroups: mergeExecutionGroups(existing?.executionGroups, incoming?.executionGroups),
    executionGroupsTotal: Number(incoming?.executionGroupsTotal || existing?.executionGroupsTotal || 0) || 0,
    executionGroupsOffset: Number(incoming?.executionGroupsOffset || existing?.executionGroupsOffset || 0) || 0,
    executionGroupsLimit: Number(incoming?.executionGroupsLimit || existing?.executionGroupsLimit || 0) || 0
  };
  const linked = [];
  const linkedSeen = new Set();
  for (const item of [...(Array.isArray(existing?.linkedConversations) ? existing.linkedConversations : []), ...(Array.isArray(incoming?.linkedConversations) ? incoming.linkedConversations : [])]) {
    const id = String(item?.conversationId || item?.linkedConversationId || '').trim();
    if (!id || linkedSeen.has(id)) continue;
    linkedSeen.add(id);
    linked.push(item);
  }
  merged.linkedConversations = linked;
  const preambles = [];
  const seen = new Map();
  for (const item of [...(Array.isArray(existing?.preambles) ? existing.preambles : []), ...(Array.isArray(incoming?.preambles) ? incoming.preambles : [])]) {
    const key = chooseRichString(item?.id, item?.createdAt, item?.content);
    if (!key) {
      preambles.push(item);
      continue;
    }
    const idx = seen.get(key);
    if (idx == null) {
      seen.set(key, preambles.length);
      preambles.push(item);
    } else {
      preambles[idx] = mergePreamble(preambles[idx], item);
    }
  }
  merged.preambles = preambles;
  if (!merged.preamble && merged.preambles.length > 0) {
    merged.preamble = merged.preambles[merged.preambles.length - 1];
  }
  return merged;
}

function mergeExecutionGroups(existing = [], incoming = []) {
  const out = [];
  const seen = new Map();
  for (const list of [existing, incoming]) {
    for (const group of Array.isArray(list) ? list : []) {
      const key = chooseRichString(
        group?.assistantMessageId,
        group?.parentMessageId,
        group?.modelMessageId,
        ''
      );
      if (!key) {
        out.push(group);
        continue;
      }
      const found = seen.get(key);
      if (found == null) {
        seen.set(key, out.length);
        out.push(group);
      } else {
        out[found] = { ...out[found], ...group };
      }
    }
  }
  return out.sort((left, right) => compareExecutionGroups(left, right));
}

export function groupIntoIterations(messages = []) {
  const items = [];
  let current = null;
  const lastUserContentByTurn = new Map();
  const attachSummaryToLatestIteration = (message = {}) => {
    const turnId = String(message?.turnId || '').trim();
    for (let index = items.length - 1; index >= 0; index -= 1) {
      const item = items[index];
      if (item?.type !== 'iteration') continue;
      if (String(item?.turnId || '').trim() !== turnId) continue;
      item.summary = message;
      return true;
    }
    return false;
  };
  const sameIteration = (a = null, message = {}) => {
    if (!a) return false;
    const currentTurn = String(a?.turnId || '').trim();
    const nextTurn = String(message?.turnId || '').trim();
    if (currentTurn !== nextTurn) return false;
    const currentIterationRaw = Number(a?.iteration);
    const nextIterationRaw = Number(message?.iteration);
    const currentIteration = Number.isFinite(currentIterationRaw) && currentIterationRaw > 0 ? currentIterationRaw : null;
    const nextIteration = Number.isFinite(nextIterationRaw) && nextIterationRaw > 0 ? nextIterationRaw : null;
    if (currentIteration == null && nextIteration == null) return true;
    if (nextIteration == null) return true;
    return currentIteration === nextIteration;
  };
  const flushCurrent = () => {
    if (!current) return;
    items.push(current);
    if (current.response) {
      items.push({ type: 'response', message: current.response });
    }
    current = null;
  };
  const ensureCurrent = (message = {}) => {
    if (!sameIteration(current, message)) {
      flushCurrent();
      current = {
        type: 'iteration',
        turnId: message?.turnId || '',
        iteration: (() => {
          const raw = Number(message?.iteration);
          return Number.isFinite(raw) && raw > 0 ? raw : null;
        })(),
        agentId: '',
        preambles: [],
        preamble: null,
        streamContent: '',
        toolCalls: [],
        linkedConversations: [],
        executionGroups: [],
        executionGroupsTotal: 0,
        executionGroupsOffset: 0,
        executionGroupsLimit: 0,
        summary: null,
        response: null,
        status: String(message?.turnStatus || message?.status || 'running'),
        errorMessage: String(message?.errorMessage || '').trim()
      };
    }
    return current;
  };
  const attachSteps = (steps = [], message = {}) => {
    if (!Array.isArray(steps) || steps.length === 0) return;
    const target = ensureCurrent(message);
    target.toolCalls = mergeStepList(target.toolCalls, steps);
    const preambles = Array.isArray(target.preambles) ? target.preambles : [];
    const lastPreamble = preambles[preambles.length - 1];
    if (lastPreamble) {
      lastPreamble.steps = mergeStepList(lastPreamble.steps, steps);
    }
  };
  const attachLinkedConversations = (message = {}) => {
    const target = ensureCurrent(message);
    const next = Array.isArray(target.linkedConversations) ? [...target.linkedConversations] : [];
    const seen = new Set(next.map((entry) => String(entry?.conversationId || entry?.linkedConversationId || '').trim()).filter(Boolean));
    const append = (entry = {}) => {
      const id = String(entry?.conversationId || entry?.linkedConversationId || '').trim();
      if (!id || seen.has(id)) return;
      seen.add(id);
      next.push(entry);
    };
    (Array.isArray(message?.linkedConversations) ? message.linkedConversations : []).forEach((entry) => append(entry));
    const directId = String(message?.linkedConversationId || '').trim();
    if (directId) {
      append({
        conversationId: directId,
        linkedConversationId: directId,
        agentId: String(message?.linkedConversationAgentId || message?.LinkedConversationAgentId || '').trim(),
        title: String(message?.linkedConversationTitle || message?.LinkedConversationTitle || '').trim(),
        status: String(message?.status || message?.turnStatus || '').trim(),
        response: String(message?.response || '').trim(),
        createdAt: message?.createdAt || '',
        updatedAt: message?.updatedAt || ''
      });
    }
    target.linkedConversations = next;
  };
  const attachExecutionGroups = (message = {}) => {
    const groups = Array.isArray(message?.executionGroups) ? message.executionGroups : [];
    const hasSingleGroup = !!message?.executionGroup;
    if (groups.length === 0 && !hasSingleGroup) return;
    const target = ensureCurrent(message);
    if (groups.length > 0) {
      target.executionGroups = mergeExecutionGroups(target.executionGroups, groups);
      target.executionGroupsTotal = Number(message.executionGroupsTotal || target.executionGroupsTotal || groups.length) || groups.length;
      target.executionGroupsOffset = Number(message.executionGroupsOffset || target.executionGroupsOffset || 0) || 0;
      target.executionGroupsLimit = Number(message.executionGroupsLimit || target.executionGroupsLimit || groups.length) || groups.length;
    }
    if (hasSingleGroup) {
      target.executionGroups = mergeExecutionGroups(target.executionGroups, [message.executionGroup]);
      target.executionGroupsTotal = Number(message.executionGroupsTotal || target.executionGroupsTotal || 0) || 0;
      target.executionGroupsOffset = Number(message.executionGroupsOffset || target.executionGroupsOffset || 0) || 0;
      target.executionGroupsLimit = Number(message.executionGroupsLimit || target.executionGroupsLimit || 0) || 0;
    }
  };
  const attachAgent = (message = {}) => {
    const agentId = normalizeAgentId(
      message?.agentIdUsed
      || message?.AgentIdUsed
      || ''
    );
    if (!agentId) return;
    const target = ensureCurrent(message);
    if (!String(target.agentId || '').trim()) {
      target.agentId = agentId;
    }
  };

  for (const message of messages) {
    if (message?._type === 'queue') {
      items.push({ type: 'queue', message });
      continue;
    }

    if (message?._type === 'stream') {
      items.push({ type: 'stream', message });
      continue;
    }

    const role = String(message?.role || '').toLowerCase();
    const mode = String(message?.mode || '').trim().toLowerCase();
    const execSteps = flattenToolSteps(message);

    if (role === 'user') {
      lastUserContentByTurn.set(String(message?.turnId || '').trim(), String(message?.content || '').trim());
      flushCurrent();
      items.push({ type: 'user', message });
      continue;
    }

    if (mode === 'summary' || String(message?.status || '').trim().toLowerCase() === 'summary') {
      if (current) {
        current.summary = message;
      } else {
        attachSummaryToLatestIteration(message);
      }
      continue;
    }

    if (role === 'assistant' && Number(message.interim || 0) === 1) {
      const turnId = String(message?.turnId || '').trim();
      const content = String(message?.content || '').trim();
      const hasExecutionEvidence = execSteps.length > 0
        || ((Array.isArray(message?.executionGroups) ? message.executionGroups.length : 0) > 0)
        || !!message?.executionGroup;
      const latestUserContent = String(lastUserContentByTurn.get(turnId) || '').trim();
      const isUserEcho = turnId && content && content === latestUserContent;
      if (isUserEcho && !hasExecutionEvidence) {
        continue;
      }
      if (String(message?._bubbleSource || '').trim() === 'stream') {
        ensureCurrent(message);
        attachAgent(message);
        attachLinkedConversations(message);
        current.streamContent = chooseRichString(message?.content, current?.streamContent);
        current.status = String(message.turnStatus || message.status || current.status || 'running');
        if (execSteps.length > 0) {
          attachSteps(execSteps, message);
        }
        attachExecutionGroups(message);
        continue;
      }
      const preambleEntry = {
        ...message,
        content: isUserEcho ? '' : message.content,
        steps: []
      };
      ensureCurrent(message);
      attachAgent(message);
      attachLinkedConversations(message);
      current.preambles = Array.isArray(current.preambles) ? current.preambles : [];
      if (!String(preambleEntry.content || '').trim()) {
        current.preamble = current.preambles[current.preambles.length - 1] || null;
      } else if (current.preambles.length > 0) {
        current.preambles[current.preambles.length - 1] = mergePreamble(current.preambles[current.preambles.length - 1], preambleEntry);
      } else {
        current.preambles.push(preambleEntry);
        current.preamble = preambleEntry;
      }
      current.status = String(message.turnStatus || message.status || current.status || 'running');
      current.errorMessage = chooseRichString(message?.errorMessage, current?.errorMessage);
      debugIterationTimeline('interim-preamble', {
        turnId: current?.turnId || '',
        iteration: current?.iteration,
        content: String(preambleEntry?.content || '').trim(),
        stepCount: Array.isArray(current?.toolCalls) ? current.toolCalls.length : 0
      });
      if (execSteps.length > 0) {
        attachSteps(execSteps, message);
      }
      attachExecutionGroups(message);
      continue;
    }

    if (execSteps.length > 0 || role === 'tool' || (message.toolMessage || []).length > 0) {
      const assistantText = String(message.content || '').trim();
      const isFinalAssistant = role === 'assistant' && Number(message.interim || 0) === 0;
      const streamOwnsBubble = String(message?._bubbleSource || '').trim() === 'stream';
      const hasNonModelStep = execSteps.some((step) => String(step?.kind || '').toLowerCase() !== 'model');
      const latestUserContent = String(lastUserContentByTurn.get(String(message?.turnId || '').trim()) || '').trim();
      const isUserEcho = assistantText !== '' && assistantText === latestUserContent;
      if (isFinalAssistant && assistantText !== '' && hasNonModelStep && !streamOwnsBubble && !isUserEcho) {
        // Text arrived alongside tool calls (common in streaming). Treat as
        // preamble so it renders alongside the execution block. Not set as
        // response — the streaming bubble (_type:'stream') is a separate
        // rendering path and is unaffected.
        const preambleEntry = {
          ...message,
          content: assistantText,
          steps: []
        };
        ensureCurrent(message);
        attachAgent(message);
        attachLinkedConversations(message);
        current.preambles = Array.isArray(current.preambles) ? current.preambles : [];
        if (current.preambles.length > 0) {
          current.preambles[current.preambles.length - 1] = mergePreamble(current.preambles[current.preambles.length - 1], preambleEntry);
        } else {
          current.preambles.push(preambleEntry);
        }
        current.preamble = preambleEntry;
        current.status = String(message.turnStatus || message.status || current.status || 'running');
        current.errorMessage = chooseRichString(message?.errorMessage, current?.errorMessage);
      } else if (isFinalAssistant && assistantText !== '' && !streamOwnsBubble && !isUserEcho) {
        ensureCurrent(message);
        attachAgent(message);
        attachLinkedConversations(message);
        current.response = message;
        current.status = String(message.turnStatus || message.status || current.status || 'completed');
        current.errorMessage = chooseRichString(message?.errorMessage, current?.errorMessage);
      }
      attachLinkedConversations(message);
      if (execSteps.length > 0) {
        attachSteps(execSteps, message);
      } else {
        attachSteps([message], message);
      }
      attachExecutionGroups(message);
      continue;
    }

    if (role === 'assistant' && Number(message.interim || 0) === 0) {
      const streamOwnsBubble = String(message?._bubbleSource || '').trim() === 'stream';
      if (current) {
        attachAgent(message);
        if (!streamOwnsBubble) {
          current.response = message;
        }
        current.status = String(message.turnStatus || message.status || 'completed');
        current.errorMessage = chooseRichString(message?.errorMessage, current?.errorMessage);
        attachExecutionGroups(message);
        flushCurrent();
      } else {
        const hasExecutionGroups = (Array.isArray(message?.executionGroups) && message.executionGroups.length > 0)
          || !!message?.executionGroup;
        if (hasExecutionGroups) {
          ensureCurrent(message);
          attachAgent(message);
          attachLinkedConversations(message);
          if (!streamOwnsBubble) {
            current.response = message;
          }
          current.status = String(message.turnStatus || message.status || 'completed');
          current.errorMessage = chooseRichString(message?.errorMessage, current?.errorMessage);
          attachExecutionGroups(message);
          flushCurrent();
        } else {
          items.push({ type: 'response', message });
        }
      }
      continue;
    }

    items.push({ type: 'response', message });
  }

  flushCurrent();
  debugIterationTimeline('grouped', {
    itemCount: items.length,
    iterations: items.filter((item) => item?.type === 'iteration').map((item) => ({
      turnId: item?.turnId || '',
      iteration: item?.iteration,
      preambles: Array.isArray(item?.preambles) ? item.preambles.length : 0,
      toolCalls: Array.isArray(item?.toolCalls) ? item.toolCalls.length : 0,
      response: String(item?.response?.content || '').trim()
    }))
  });

  return items;
}

export function synthesizeIterationMessages(messages = [], visibleCount = Number.MAX_SAFE_INTEGER) {
  const grouped = collapseTurnIterations(dedupeGroupedIterations(groupIntoIterations(messages)));
  const iterationTurnIds = new Set(
    grouped
      .filter((item) => item?.type === 'iteration')
      .map((item) => String(item?.turnId || '').trim())
      .filter(Boolean)
  );
  const iterations = grouped.filter((item) => item.type === 'iteration');
  const hiddenCount = Math.max(0, iterations.length - visibleCount);
  const firstVisibleIndex = Math.max(0, iterations.length - visibleCount);
  let seenIterations = 0;
  const out = [];
  const deferredStreams = new Map();

  for (const item of grouped) {
    if (item.type === 'stream') {
      const turnId = String(item?.message?.turnId || '').trim();
      if (turnId && iterationTurnIds.has(turnId)) {
        const pending = deferredStreams.get(turnId) || [];
        pending.push(item.message);
        deferredStreams.set(turnId, pending);
        continue;
      }
      out.push(item.message);
      continue;
    }
    if (item.type === 'user' || item.type === 'summary' || item.type === 'queue') {
      out.push(item.message);
      continue;
    }
    if (item.type === 'response') {
      // Skip interim messages — they are already captured as preambles.
      const isInterim = Number(item.message?.interim || 0) === 1;
      if (isInterim) continue;
      const turnId = String(item?.message?.turnId || '').trim();
      const isElicitation = !!item?.message?.elicitation?.requestedSchema;
      if (turnId && iterationTurnIds.has(turnId) && !isElicitation) {
        continue;
      }
      out.push(item.message);
      continue;
    }

    if (item.type === 'iteration') {
      if (seenIterations >= firstVisibleIndex) {
        const createdAt = String(item?.response?.createdAt || item?.preamble?.createdAt || '').trim();
        const isLatestIteration = seenIterations === iterations.length - 1;
        const turnId = String(item?.turnId || '').trim();
        const pendingStreams = deferredStreams.get(turnId) || [];
        const streamContent = pendingStreams.length === 0 ? item?.streamContent || '' : '';
        const iterationContent = String(
          item?.response?.content
          || streamContent
          || item?.preamble?.content
          || ''
        ).trim();
        out.push({
          _type: 'iteration',
          id: `iteration:${item.turnId || seenIterations}:${item.iteration ?? seenIterations}`,
          role: 'assistant',
          createdAt,
          content: iterationContent,
          generatedFiles: mergeGeneratedFiles(
            item?.response?.generatedFiles,
            item?.preamble?.generatedFiles,
            ...(Array.isArray(item?.preambles) ? item.preambles.map((entry) => entry?.generatedFiles) : [])
          ),
          _iterationData: {
            ...item,
            index: seenIterations,
            hiddenCount,
            totalCount: iterations.length,
            isLatestIteration
          }
        });
        pendingStreams.forEach((streamMessage) => out.push(streamMessage));
        deferredStreams.delete(turnId);

      }
      seenIterations++;
    }
  }

  for (const pending of deferredStreams.values()) {
    for (const streamMessage of pending) {
      out.push(streamMessage);
    }
  }

  debugIterationTimeline('rendered', {
    groupedCount: grouped.length,
    renderedCount: out.length,
    renderedTypes: out.map((item) => String(item?._type || item?.type || item?.role || ''))
  });

  return out;
}

function collapseTurnIterations(grouped = []) {
  const out = [];
  let pendingIteration = null;
  let pendingTurnId = '';
  const sameTurnMaybeUnnumbered = (left = null, right = null) => {
    const leftTurnId = String(left?.turnId || '').trim();
    const rightTurnId = String(right?.turnId || '').trim();
    if (!leftTurnId || leftTurnId !== rightTurnId) return false;
    const leftRaw = Number(left?.iteration);
    const rightRaw = Number(right?.iteration);
    const leftIteration = Number.isFinite(leftRaw) && leftRaw > 0 ? leftRaw : null;
    const rightIteration = Number.isFinite(rightRaw) && rightRaw > 0 ? rightRaw : null;
    return leftIteration == null || rightIteration == null;
  };

  const flushPending = () => {
    if (!pendingIteration) return;
    out.push(pendingIteration);
    pendingIteration = null;
    pendingTurnId = '';
  };

  for (const item of Array.isArray(grouped) ? grouped : []) {
    if (item?.type !== 'iteration') {
      flushPending();
      out.push(item);
      continue;
    }
    const turnId = String(item?.turnId || '').trim();
    if (!pendingIteration) {
      pendingIteration = item;
      pendingTurnId = turnId;
      continue;
    }
    if (turnId && pendingTurnId && turnId === pendingTurnId && sameTurnMaybeUnnumbered(pendingIteration, item)) {
      pendingIteration = mergeIterationItems(pendingIteration, item);
      continue;
    }
    flushPending();
    pendingIteration = item;
    pendingTurnId = turnId;
  }

  flushPending();
  return out;
}

function dedupeGroupedIterations(grouped = []) {
  const seen = new Map();
  const out = [];
  for (const item of grouped) {
    if (item?.type !== 'iteration') {
      out.push(item);
      continue;
    }
    const key = iterationSignature(item);
    const found = seen.get(key);
    if (found == null) {
      seen.set(key, out.length);
      out.push(item);
      continue;
    }
    out[found] = mergeIterationItems(out[found], item);
  }
  return out;
}

function iterationSignature(item = {}) {
  const turnId = String(item?.turnId || '').trim();
  const raw = Number(item?.iteration);
  const iteration = Number.isFinite(raw) && raw > 0 ? raw : '';
  return `${turnId}::${iteration}`;
}

function flattenToolSteps(message = {}) {
  const fromRelatedMessages = flattenRelatedSteps(message);
  if (fromRelatedMessages.length > 0) return fromRelatedMessages;

  const out = [];
  const executions = Array.isArray(message.executions) ? message.executions : [];
  for (const execution of executions) {
    const steps = Array.isArray(execution?.steps) ? execution.steps : [];
    for (const step of steps) {
      const provider = step?.provider || '';
      const model = step?.model || '';
      const inferredKind = step?.kind || ((provider || model) ? 'model' : 'tool');
      out.push({
        id: step?.id || step?.messageId || `${step?.toolName || 'tool'}:${step?.opId || ''}`,
        role: 'tool',
        kind: inferredKind,
        reason: step?.reason || '',
        toolName: step?.toolName || step?.name || 'tool',
        provider,
        model,
        status: step?.status || '',
        latencyMs: step?.latencyMs || step?.durationMs || null,
        startedAt: step?.startedAt || step?.StartedAt || null,
        completedAt: step?.completedAt || step?.CompletedAt || null,
        requestPayload: step?.requestPayload || step?.request || null,
        responsePayload: step?.responsePayload || step?.response || null,
        requestPayloadId: step?.requestPayloadId || '',
        responsePayloadId: step?.responsePayloadId || '',
        providerRequestPayload: step?.providerRequestPayload || null,
        providerResponsePayload: step?.providerResponsePayload || null,
        providerRequestPayloadId: step?.providerRequestPayloadId || '',
        providerResponsePayloadId: step?.providerResponsePayloadId || '',
        streamPayload: step?.streamPayload || null,
        streamPayloadId: step?.streamPayloadId || '',
        linkedConversationId: step?.linkedConversationId || '',
        linkedConversationAgentId: step?.linkedConversationAgentId || '',
        linkedConversationTitle: step?.linkedConversationTitle || '',
        turnId: message.turnId
      });
    }
  }
  return out;
}

function flattenRelatedSteps(message = {}) {
  const out = [];
  const modelCall = message?.modelCall || null;
  if (modelCall) {
    const provider = modelCall?.provider || '';
    const model = modelCall?.model || '';
    out.push({
      id: modelCall?.messageId || message?.id || `model:${model || provider || 'step'}`,
      role: 'tool',
      kind: 'model',
      reason: String(message?.role || '').toLowerCase() === 'assistant' && Number(message?.interim || 0) === 0 ? 'final_response' : 'thinking',
      toolName: model ? `${provider ? `${provider}/` : ''}${model}` : (provider || 'model'),
      provider,
      model,
      status: modelCall?.status || message?.status || '',
      latencyMs: modelCall?.latencyMs || null,
      startedAt: modelCall?.startedAt || null,
      completedAt: modelCall?.completedAt || null,
      requestPayload: modelCall?.requestPayload || null,
      responsePayload: modelCall?.responsePayload || null,
      requestPayloadId: modelCall?.requestPayloadId || '',
      responsePayloadId: modelCall?.responsePayloadId || '',
      providerRequestPayload: modelCall?.providerRequestPayload || null,
      providerResponsePayload: modelCall?.providerResponsePayload || null,
      providerRequestPayloadId: modelCall?.providerRequestPayloadId || '',
      providerResponsePayloadId: modelCall?.providerResponsePayloadId || '',
      streamPayload: modelCall?.streamPayload || null,
      streamPayloadId: modelCall?.streamPayloadId || '',
      turnId: message.turnId
    });
  }

  const toolMessages = Array.isArray(message.toolMessage)
    ? [...message.toolMessage]
    : [];
  toolMessages.sort((a, b) => {
    const aSequence = Number(a?.sequence ?? a?.toolCall?.messageSequence ?? 0);
    const bSequence = Number(b?.sequence ?? b?.toolCall?.messageSequence ?? 0);
    if (aSequence !== bSequence) return aSequence - bSequence;
    return compareTemporalEntries(a, b);
  });
  for (let index = 0; index < toolMessages.length; index += 1) {
    const entry = toolMessages[index];
    const call = entry?.toolCall || {};
    const toolName = String(call?.toolName || entry?.toolName || '').trim();
    out.push({
      id: entry?.id || `${call?.opId || toolName || 'tool'}:${index}`,
      role: 'tool',
      kind: 'tool',
      reason: 'tool_call',
      toolName: toolName || 'tool',
      status: call?.status || entry?.status || '',
      latencyMs: call?.latencyMs || entry?.latencyMs || null,
      startedAt: call?.startedAt || entry?.startedAt || null,
      completedAt: call?.completedAt || entry?.completedAt || null,
      requestPayload: call?.requestPayload || null,
      responsePayload: call?.responsePayload || null,
      requestPayloadId: call?.requestPayloadId || '',
      responsePayloadId: call?.responsePayloadId || '',
      providerRequestPayload: call?.providerRequestPayload || null,
      providerResponsePayload: call?.providerResponsePayload || null,
      providerRequestPayloadId: call?.providerRequestPayloadId || '',
      providerResponsePayloadId: call?.providerResponsePayloadId || '',
      streamPayload: call?.streamPayload || null,
      streamPayloadId: call?.streamPayloadId || '',
      linkedConversationId: call?.linkedConversationId || entry?.linkedConversationId || '',
      linkedConversationAgentId: call?.linkedConversationAgentId || entry?.linkedConversationAgentId || '',
      linkedConversationTitle: call?.linkedConversationTitle || entry?.linkedConversationTitle || '',
      turnId: message.turnId,
      parentMessageId: entry?.parentMessageId || '',
      sequence: entry?.sequence || call?.messageSequence || null
    });
  }

  return out;
}

function normalizeTimestamp(ts) {
  if (!ts) return '';
  if (typeof ts === 'number') return new Date(ts).toISOString();
  const parsed = Date.parse(String(ts));
  if (Number.isNaN(parsed)) return '';
  return new Date(parsed).toISOString();
}

function pickString(...values) {
  for (const value of values) {
    if (typeof value === 'string' && value.trim() !== '') return value;
  }
  return '';
}
