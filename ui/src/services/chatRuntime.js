import { normalizeMessages, normalizeOne } from './messageNormalizer';
import { rememberConversationSeedTitle } from './conversationTitle';
import { clearChangeFeed, publishChangeFeed } from './changeFeedBus';
import { clearPlanFeed, publishPlanFeed } from './planFeedBus';
import { setPendingElicitation, clearPendingElicitation } from './elicitationBus';
import { applyFeedEvent, clearFeedState } from './toolFeedBus';
import { publishUsage } from './usageBus';
import {
  queueTranscriptRefresh as queueTranscriptRefreshStore,
  resetTranscriptState,
  syncTranscriptSnapshot as syncTranscriptSnapshotStore,
  tickTranscript
} from './transcriptStore';
import {
  applyAssistantFinalEvent,
  applyElicitationRequestedEvent,
  applyExecutionStreamEvent,
  applyLinkedConversationEvent,
  applyMessagePatchEvent,
  applyPreambleEvent,
  applyStreamChunk,
  applyTurnStartedEvent,
  applyToolStreamEvent,
  finalizeStreamTurn,
  markLiveOwnedTurn,
  resetLiveStreamState
} from './liveStreamStore';
import { mergeRenderedRows } from './rowMerge';
import {
  getWindowById,
  MAIN_CHAT_WINDOW_ID,
  getScopedConversationSelection,
  isMainChatWindowId,
  setScopedConversationSelection
} from './conversationWindow';
import { setStage } from './stageBus';
import { client } from './agentlyClient';
import {
  displayLabel,
  normalizeWorkspaceAgentInfos,
  normalizeWorkspaceAgentOptions,
  normalizeWorkspaceModelInfos,
  normalizeWorkspaceModelOptions
} from './workspaceMetadata';

const RUNNING_STATUSES = new Set(['running', 'thinking', 'processing', 'waiting_for_user', 'in_progress']);
const DEFAULT_VISIBLE_ITERATIONS = Number.MAX_SAFE_INTEGER;
const STREAM_DEBUG_PREFIX = '[agently-stream]';
const EXECUTOR_DEBUG_PREFIX = '[agently-executor]';
const SIDEBAR_ACTIVITY_EVENT_TYPES = new Set([
  'turn_started',
  'turn_completed',
  'turn_failed',
  'turn_canceled',
  'linked_conversation_attached',
]);

function isStreamDebugEnabled() {
  if (typeof window === 'undefined') return false;
  const envLevel = String(import.meta?.env?.VITE_FORGE_LOG_LEVEL || '').trim().toLowerCase();
  if (envLevel === 'debug') return true;
  try {
    const raw = String(window.localStorage?.getItem('agently.debugStream') || '').trim().toLowerCase();
    if (['0', 'false', 'off', 'no'].includes(raw)) return false;
    if (['1', 'true', 'on', 'yes'].includes(raw)) return true;
    return false;
  } catch (_) {
    return false;
  }
}

function isExecutorDebugEnabled() {
  if (typeof window === 'undefined') return false;
  const envLevel = String(import.meta?.env?.VITE_FORGE_LOG_LEVEL || '').trim().toLowerCase();
  if (envLevel === 'debug') return true;
  try {
    const raw = String(window.localStorage?.getItem('agently.debugExecutor') || '').trim().toLowerCase();
    return raw === '1' || raw === 'true' || raw === 'on';
  } catch (_) {
    return false;
  }
}

function logExecutorDebug(event, detail = {}) {
  if (!isExecutorDebugEnabled()) return;
  try {
    console.log(EXECUTOR_DEBUG_PREFIX, {
      event,
      ts: new Date().toISOString(),
      ...detail
    });
  } catch (_) {}
}

export function logStreamDebug(chatState = {}, event, detail = {}) {
  if (!isStreamDebugEnabled()) return;
  const startedAt = Number(chatState?.activeStreamStartedAt || chatState?.streamOpenedAt || 0);
  const elapsedMs = startedAt > 0 ? Math.max(0, Date.now() - startedAt) : null;
  const seq = Number(chatState?.debugSeq || 0) + 1;
  chatState.debugSeq = seq;
  try {
    console.log(STREAM_DEBUG_PREFIX, {
      seq,
      ts: new Date().toISOString(),
      event: String(event || '').trim() || 'unknown',
      elapsedMs,
      conversationId: String(chatState?.activeConversationID || chatState?.lastConversationID || '').trim(),
      activeStreamTurnId: String(chatState?.activeStreamTurnId || '').trim(),
      runningTurnId: String(chatState?.runningTurnId || '').trim(),
      ...detail
    });
  } catch (_) {}
}

function publishConversationActivity(conversationID = '', detail = {}) {
  if (typeof window === 'undefined') return;
  const id = String(conversationID || '').trim();
  if (!id) return;
  try {
    window.dispatchEvent(new CustomEvent('agently:conversation-activity', {
      detail: { id, ...detail }
    }));
  } catch (_) {}
}

export function resolveStreamEventConversationID(payload = {}, subscribedConversationID = '') {
  return String(payload?.conversationId || payload?.streamId || subscribedConversationID || '').trim();
}

export function shouldProcessStreamEvent({ payload = {}, subscribedConversationID = '', visibleConversationID = '' } = {}) {
  const eventConversationID = resolveStreamEventConversationID(payload, subscribedConversationID);
  const visibleID = String(visibleConversationID || '').trim();
  if (!eventConversationID) return false;
  if (!visibleID) return true;
  return eventConversationID === visibleID;
}

function streamEventMode(payload = {}) {
  return String(payload?.mode || payload?.patch?.mode || '').trim().toLowerCase();
}

function shouldIgnoreExecutionStreamEvent(payload = {}) {
  const mode = streamEventMode(payload);
  return mode === 'summary';
}

function textDeltaQueueKey(payload = {}, fallbackConversationID = '') {
  return [
    String(payload?.conversationId || payload?.streamId || fallbackConversationID || '').trim(),
    String(payload?.turnId || '').trim(),
    String(payload?.assistantMessageId || payload?.id || '').trim(),
    String(payload?.mode || payload?.patch?.mode || '').trim()
  ].join('::');
}

function enqueueTextDelta(chatState = {}, payload = {}, fallbackConversationID = '') {
  const queue = Array.isArray(chatState.pendingTextDeltaQueue) ? chatState.pendingTextDeltaQueue : [];
  const key = textDeltaQueueKey(payload, fallbackConversationID);
  const content = String(payload?.content || '');
  if (!content) {
    chatState.pendingTextDeltaQueue = queue;
    return queue;
  }
  const last = queue[queue.length - 1];
  if (last && last._queueKey === key) {
    last.content = `${String(last.content || '')}${content}`;
    last.createdAt = String(payload?.createdAt || last.createdAt || '').trim() || last.createdAt;
    if (!String(last.id || '').trim() && String(payload?.id || '').trim()) {
      last.id = String(payload.id).trim();
    }
    if (!String(last.assistantMessageId || '').trim() && String(payload?.assistantMessageId || '').trim()) {
      last.assistantMessageId = String(payload.assistantMessageId).trim();
    }
  } else {
    queue.push({
      ...payload,
      conversationId: String(payload?.conversationId || payload?.streamId || fallbackConversationID || '').trim(),
      content,
      _queueKey: key,
    });
  }
  chatState.pendingTextDeltaQueue = queue;
  return queue;
}

function flushQueuedTextDeltas(chatState = {}, context = null, conversationID = '') {
  const queue = Array.isArray(chatState.pendingTextDeltaQueue) ? [...chatState.pendingTextDeltaQueue] : [];
  if (queue.length === 0) return false;
  chatState.pendingTextDeltaQueue = [];
  for (const payload of queue) {
    applyStreamChunk(chatState, payload, conversationID);
  }
  renderMergedRowsForContext(context);
  return true;
}

function scheduleTextDeltaFlush(context, chatState = {}, conversationID = '') {
  if (!chatState) {
    renderMergedRowsForContext(context);
    return;
  }
  if (chatState.pendingTextDeltaFlush) return;
  const run = () => {
    chatState.pendingTextDeltaFlush = null;
    flushQueuedTextDeltas(chatState, context, conversationID);
  };
  const raf = typeof window !== 'undefined' && typeof window.requestAnimationFrame === 'function'
    ? window.requestAnimationFrame.bind(window)
    : null;
  if (raf) {
    chatState.pendingTextDeltaFlush = raf(run);
    return;
  }
  chatState.pendingTextDeltaFlush = window.setTimeout(run, 16);
}

function scheduleStreamRender(context, chatState = {}) {
  if (!chatState) {
    renderMergedRowsForContext(context);
    return;
  }
  if (chatState.pendingStreamRender) return;
  const run = () => {
    chatState.pendingStreamRender = null;
    renderMergedRowsForContext(context);
  };
  const raf = typeof window !== 'undefined' && typeof window.requestAnimationFrame === 'function'
    ? window.requestAnimationFrame.bind(window)
    : null;
  if (raf) {
    chatState.pendingStreamRender = raf(run);
    return;
  }
  chatState.pendingStreamRender = window.setTimeout(run, 16);
}

function clearPendingStreamScheduling(chatState = {}) {
  if (!chatState) return;
  if (chatState.pendingTextDeltaFlush != null) {
    if (typeof window !== 'undefined' && typeof window.cancelAnimationFrame === 'function') {
      try { window.cancelAnimationFrame(chatState.pendingTextDeltaFlush); } catch (_) {}
    }
    try { clearTimeout(chatState.pendingTextDeltaFlush); } catch (_) {}
    chatState.pendingTextDeltaFlush = null;
  }
  if (chatState.pendingStreamRender != null) {
    if (typeof window !== 'undefined' && typeof window.cancelAnimationFrame === 'function') {
      try { window.cancelAnimationFrame(chatState.pendingStreamRender); } catch (_) {}
    }
    try { clearTimeout(chatState.pendingStreamRender); } catch (_) {}
    chatState.pendingStreamRender = null;
  }
  chatState.pendingTextDeltaQueue = [];
}

function isLatePostTerminalExecutionEvent(type = '', payload = {}) {
  const eventType = String(type || '').trim().toLowerCase();
  if (!eventType) return false;
  if (eventType === 'text_delta'
    || eventType === 'reasoning_delta'
    || eventType === 'tool_call_delta'
    || eventType === 'model_started'
    || eventType === 'model_completed'
    || eventType === 'tool_calls_planned'
    || eventType === 'tool_call_started'
    || eventType === 'tool_call_completed'
    || eventType === 'assistant_preamble'
    || eventType === 'elicitation_requested'
    || eventType === 'linked_conversation_attached'
    || eventType === 'turn_started') {
    return true;
  }
  if (eventType !== 'control') return false;
  const op = String(payload?.op || '').trim().toLowerCase();
  return op === 'turn_started';
}

function draftConversationValues(current = {}, defaults = {}) {
  // Preserve the user's current agent/model selection when starting a new
  // conversation. Read from localStorage as the most reliable source (set by selectAgent).
  const persistedAgent = getPersistedSelectedAgent();
  const values = {
    ...current,
    id: '',
    title: 'New conversation',
    agent: persistedAgent || current?.agent || defaults?.agent || '',
    model: current?.model || defaults?.model || '',
    embedder: defaults?.embedder || ''
  };
  return values;
}

function getPersistedSelectedAgent() {
  try {
    return String(localStorage.getItem('agently.selectedAgent') || '').trim();
  } catch (_) {
    return '';
  }
}

function resolveStreamAgentName(context, agentId = '') {
  const target = String(agentId || '').trim();
  if (!target) return '';
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  const metaForm = metaDS?.peekFormData?.() || {};
  const byKey = metaForm?.agentInfo?.[target] || null;
  const keyedName = String(byKey?.label || byKey?.name || byKey?.title || '').trim();
  if (keyedName) return keyedName;
  const optionLists = [
    ...(Array.isArray(metaForm?.agentOptions) ? metaForm.agentOptions : []),
    ...(Array.isArray(metaForm?.agentInfos) ? metaForm.agentInfos : [])
  ];
  const matched = optionLists.find((entry) => {
    const candidates = [entry?.id, entry?.value, entry?.name, entry?.label, entry?.title]
      .map((value) => String(value || '').trim())
      .filter(Boolean);
    return candidates.includes(target);
  }) || null;
  return String(matched?.label || matched?.name || matched?.title || '').trim();
}

function rememberTurnAgent(chatState = {}, context, payload = {}) {
  const agentIdUsed = String(
    payload?.agentIdUsed
    || payload?.patch?.agentIdUsed
    || ''
  ).trim();
  if (!agentIdUsed) return;
  chatState.activeTurnAgentId = agentIdUsed;
  chatState.activeTurnAgentName = String(payload?.agentName || resolveStreamAgentName(context, agentIdUsed) || '').trim();
}

function enrichPayloadWithTurnAgent(chatState = {}, context, payload = {}) {
  const enriched = { ...payload };
  const agentIdUsed = String(enriched?.agentIdUsed || chatState?.activeTurnAgentId || '').trim();
  if (agentIdUsed) {
    enriched.agentIdUsed = agentIdUsed;
  }
  const agentName = String(enriched?.agentName || chatState?.activeTurnAgentName || resolveStreamAgentName(context, agentIdUsed) || '').trim();
  if (agentName) {
    enriched.agentName = agentName;
  }
  return enriched;
}

export function sanitizeAutoSelection(value) {
  return String(value || '').trim();
}

function matchesVisibleAgentEntry(entry = {}, target = '') {
  const normalizedTarget = sanitizeAutoSelection(target);
  if (!normalizedTarget) return false;
  const candidates = [
    entry?.id,
    entry?.value,
    entry?.name,
    entry?.label,
    entry?.title
  ].map((value) => String(value || '').trim()).filter(Boolean);
  return candidates.includes(normalizedTarget);
}

function isVisibleAgent(metaForm = {}, agent = '') {
  const normalizedAgent = sanitizeAutoSelection(agent);
  if (!normalizedAgent) return false;
  if (normalizedAgent === 'auto') return true;
  const visibleEntries = [
    ...(Array.isArray(metaForm?.agentInfos) ? metaForm.agentInfos : []),
    ...(Array.isArray(metaForm?.agentOptions) ? metaForm.agentOptions : [])
  ];
  return visibleEntries.some((entry) => matchesVisibleAgentEntry(entry, normalizedAgent));
}

function resolveVisibleSelectedAgent(metaForm = {}, ...candidates) {
  for (const candidate of candidates) {
    const normalized = sanitizeAutoSelection(candidate);
    if (!normalized) continue;
    if (isVisibleAgent(metaForm, normalized)) return normalized;
  }
  const defaultAgent = sanitizeAutoSelection(metaForm?.defaults?.agent || '');
  if (isVisibleAgent(metaForm, defaultAgent)) return defaultAgent;
  const firstVisible = [
    ...(Array.isArray(metaForm?.agentInfos) ? metaForm.agentInfos : []),
    ...(Array.isArray(metaForm?.agentOptions) ? metaForm.agentOptions : [])
  ].find((entry) => {
    const value = sanitizeAutoSelection(entry?.id || entry?.value || '');
    return value && value !== 'auto';
  });
  return sanitizeAutoSelection(firstVisible?.id || firstVisible?.value || '');
}

export function isRunningStatus(status) {
  return RUNNING_STATUSES.has(String(status || '').toLowerCase());
}

export function normalizeMetaResponse(payload) {
  const data = payload?.data || payload || {};
  const capabilities = {
    ...(data?.capabilities || {}),
    agentAutoSelection: !!data?.capabilities?.agentAutoSelection,
    modelAutoSelection: !!data?.capabilities?.modelAutoSelection,
    toolAutoSelection: !!data?.capabilities?.toolAutoSelection,
    compactConversation: !!data?.capabilities?.compactConversation,
    pruneConversation: !!data?.capabilities?.pruneConversation,
    anonymousSession: !!data?.capabilities?.anonymousSession,
    messageCursor: !!data?.capabilities?.messageCursor,
    structuredElicitation: !!data?.capabilities?.structuredElicitation,
    turnStartedEvent: !!data?.capabilities?.turnStartedEvent
  };
  const defaults = {
    ...(data?.defaults || {}),
    agent: data?.defaults?.agent || data?.defaultAgent || '',
    model: data?.defaults?.model || data?.defaultModel || '',
    embedder: data?.defaults?.embedder || data?.defaultEmbedder || '',
    autoSelectTools: !!data?.defaults?.autoSelectTools
  };
  const agentInfos = Array.isArray(data?.agentInfos) ? data.agentInfos : [];
  const modelInfos = Array.isArray(data?.modelInfos) ? data.modelInfos : [];
  const agents = agentInfos.length > 0 ? agentInfos : (Array.isArray(data?.agents) ? data.agents : []);
  const models = modelInfos.length > 0 ? modelInfos : (Array.isArray(data?.models) ? data.models : []);
  const tools = Array.isArray(data?.tools) ? data.tools : [];

  const normalizeOption = (entry) => {
    if (entry && typeof entry === 'object') {
      const value = String(entry.id || entry.value || entry.name || '').trim();
      if (!value) return null;
      const label = displayLabel(entry, 'generic');
      return { value, label };
    }
    const value = String(entry || '').trim();
    if (!value) return null;
    return { value, label: value };
  };

  const normalizedAgentInfos = normalizeWorkspaceAgentInfos(agents);
  const normalizedModelInfos = normalizeWorkspaceModelInfos(models);
  const agentOptions = normalizeWorkspaceAgentOptions(agents, defaults.agent);
  const modelOptions = normalizeWorkspaceModelOptions(models, defaults.model);
  const normalizedAgentInfo = normalizedAgentInfos.reduce((acc, entry) => {
    if (entry?.id) acc[entry.id] = entry;
    return acc;
  }, {});
  return {
    ...data,
    capabilities,
    defaults,
    agent: sanitizeAutoSelection(data?.agent || defaults.agent || ''),
    model: sanitizeAutoSelection(data?.model || defaults.model || ''),
    embedder: sanitizeAutoSelection(data?.embedder || defaults.embedder || ''),
    agentInfos: normalizedAgentInfos,
    modelInfos: normalizedModelInfos,
    agentInfo: normalizedAgentInfo,
    modelInfo: normalizedModelInfos.reduce((acc, entry) => {
      if (entry?.id) acc[entry.id] = entry;
      return acc;
    }, {}),
    agentOptions: capabilities.agentAutoSelection
      ? [{ value: 'auto', label: 'Auto-select agent' }, ...agentOptions]
      : agentOptions,
    modelOptions: capabilities.modelAutoSelection
      ? [{ value: 'auto', label: 'Auto-select model' }, ...modelOptions]
      : modelOptions,
    toolOptions: tools.map(normalizeOption).filter(Boolean)
  };
}

function normalizeStarterTaskEntries(entries = [], agent = null) {
  return (Array.isArray(entries) ? entries : []).map((entry, index) => {
    if (!entry || typeof entry !== 'object') return null;
    const prompt = String(entry.prompt || '').trim();
    const title = String(entry.title || '').trim();
    if (!prompt || !title) return null;
    return {
      id: String(entry.id || `starter-${index + 1}`).trim(),
      title,
      prompt,
      description: String(entry.description || '').trim(),
      icon: String(entry.icon || '').trim(),
      agentId: String(agent?.id || '').trim(),
      agentName: String(agent?.name || '').trim()
    };
  }).filter(Boolean);
}

export function resolveStarterTasks({ agentInfos = [], selectedAgent = '' } = {}) {
  const normalizedSelectedAgent = sanitizeAutoSelection(selectedAgent || '');
  const normalizedAgents = Array.isArray(agentInfos) ? agentInfos : [];
  const useAllAgents = normalizedSelectedAgent === 'auto';
  const selectedEntries = useAllAgents
    ? normalizedAgents
    : normalizedAgents.filter((entry) => String(entry?.id || '').trim() === normalizedSelectedAgent);
  const rawTasks = selectedEntries.flatMap((entry) => normalizeStarterTaskEntries(entry?.starterTasks, entry));
  const seen = new Set();
  return rawTasks.filter((entry) => {
    const key = `${String(entry?.id || '').trim()}|${String(entry?.title || '').trim()}|${String(entry?.prompt || '').trim()}`;
    if (!key.trim() || seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export function resolvePayload(payload = null) {
  if (!payload || typeof payload !== 'object') return null;
  const id = String(payload?.id ?? payload?.Id ?? '').trim();
  const compression = String(payload?.compression ?? payload?.Compression ?? 'none').toLowerCase();
  const inlineBody = payload?.inlineBody ?? payload?.InlineBody;
  if (compression && compression !== 'none') {
    return {
      id,
      compression,
      note: 'payload is compressed in transcript; use payload id to inspect raw body'
    };
  }
  if (typeof inlineBody === 'string' && inlineBody.trim() !== '') {
    try {
      return JSON.parse(inlineBody);
    } catch (_) {
      const preview = inlineBody.length > 4096 ? `${inlineBody.slice(0, 4096)}\n...[truncated]` : inlineBody;
      return {
        id,
        compression: compression || 'none',
        inlineBody: preview
      };
    }
  }
  return {
    id,
    compression: compression || 'none'
  };
}

function mapExecutionsFromMessage(message = {}) {
  const steps = [];
  const entryStatus = (entry = {}, call = {}) => (
    call?.status
    || call?.Status
    || entry?.status
    || entry?.Status
    || 'completed'
  );
  const entryPayload = (entry = {}, call = {}, kind = 'request') => {
    const key = kind.toLowerCase() === 'response' ? 'response' : 'request';
    const upper = key === 'response' ? 'Response' : 'Request';
    return resolvePayload(
      call?.[`${key}Payload`]
      || call?.[`${upper}Payload`]
      || entry?.[`${key}Payload`]
      || entry?.[`${upper}Payload`]
    );
  };
  const entryPayloadID = (entry = {}, call = {}, kind = 'request') => {
    const key = kind.toLowerCase() === 'response' ? 'response' : 'request';
    const upper = key === 'response' ? 'Response' : 'Request';
    return String(
      call?.[`${key}PayloadId`]
      || call?.[`${upper}PayloadId`]
      || entry?.[`${key}PayloadId`]
      || entry?.[`${upper}PayloadId`]
      || ''
    ).trim();
  };
  const topLevelCall = message?.toolCall || message?.ToolCall || null;
  const modelCall = message?.modelCall || message?.ModelCall || null;
  if (modelCall) {
    const role = String(message?.role || message?.Role || '').toLowerCase();
    const interim = Number(message?.interim ?? message?.Interim ?? 0) || 0;
    const provider = String(modelCall?.provider || modelCall?.Provider || '').trim();
    const model = String(modelCall?.model || modelCall?.Model || '').trim();
    steps.push({
      id: modelCall?.messageId || modelCall?.MessageId || `model:${model || provider || 'step'}`,
      kind: 'model',
      reason: role === 'assistant' && interim === 0 ? 'final_response' : 'thinking',
      toolName: model ? `${provider ? `${provider}/` : ''}${model}` : (provider || 'model'),
      provider,
      model,
      status: modelCall?.status || modelCall?.Status || 'completed',
      latencyMs: modelCall?.latencyMs || modelCall?.LatencyMs || null,
      startedAt: modelCall?.startedAt || modelCall?.StartedAt || null,
      completedAt: modelCall?.completedAt || modelCall?.CompletedAt || null,
      requestPayloadId: modelCall?.requestPayloadId || modelCall?.RequestPayloadId || '',
      responsePayloadId: modelCall?.responsePayloadId || modelCall?.ResponsePayloadId || '',
      providerRequestPayloadId: modelCall?.providerRequestPayloadId || modelCall?.ProviderRequestPayloadId || '',
      providerResponsePayloadId: modelCall?.providerResponsePayloadId || modelCall?.ProviderResponsePayloadId || '',
      streamPayloadId: modelCall?.streamPayloadId || modelCall?.StreamPayloadId || '',
      requestPayload: resolvePayload(
        modelCall?.modelCallRequestPayload
        || modelCall?.ModelCallRequestPayload
        || modelCall?.requestPayload
        || modelCall?.RequestPayload
      ),
      responsePayload: resolvePayload(
        modelCall?.modelCallResponsePayload
        || modelCall?.ModelCallResponsePayload
        || modelCall?.responsePayload
        || modelCall?.ResponsePayload
      ),
      providerRequestPayload: resolvePayload(modelCall?.modelCallProviderRequestPayload || modelCall?.ModelCallProviderRequestPayload),
      providerResponsePayload: resolvePayload(modelCall?.modelCallProviderResponsePayload || modelCall?.ModelCallProviderResponsePayload),
      streamPayload: resolvePayload(modelCall?.modelCallStreamPayload || modelCall?.ModelCallStreamPayload)
    });
  }

  const entries = Array.isArray(message?.toolMessage || message?.ToolMessage)
    ? (message.toolMessage || message.ToolMessage)
    : [];
  const fallbackType = String(message?.type || message?.Type || '').toLowerCase();
  const fallbackTool = String(
    message?.toolName
    || message?.ToolName
    || topLevelCall?.toolName
    || topLevelCall?.ToolName
    || ''
  ).trim();
  if (entries.length === 0 && (fallbackType === 'tool' || fallbackType === 'tool_op' || fallbackTool || topLevelCall)) {
    steps.push({
      id: message?.id || message?.Id || `${fallbackTool || fallbackType || 'tool'}:0`,
      kind: 'tool',
      reason: 'tool_call',
      toolName: fallbackTool || 'tool',
      status: entryStatus(message, topLevelCall),
      latencyMs: topLevelCall?.latencyMs || topLevelCall?.LatencyMs || message?.latencyMs || message?.LatencyMs || null,
      startedAt: topLevelCall?.startedAt || topLevelCall?.StartedAt || message?.startedAt || message?.StartedAt || null,
      completedAt: topLevelCall?.completedAt || topLevelCall?.CompletedAt || message?.completedAt || message?.CompletedAt || null,
      requestPayloadId: entryPayloadID(message, topLevelCall, 'request'),
      responsePayloadId: entryPayloadID(message, topLevelCall, 'response'),
      requestPayload: entryPayload(message, topLevelCall, 'request'),
      responsePayload: entryPayload(message, topLevelCall, 'response'),
      linkedConversationId: String(message?.linkedConversationId || message?.LinkedConversationId || '').trim()
    });
  }
  for (let index = 0; index < entries.length; index++) {
    const entry = entries[index];
    const call = entry?.toolCall || entry?.ToolCall || {};
    const toolName = String(call?.toolName || call?.ToolName || entry?.toolName || entry?.ToolName || '').trim();
    const linkedConversationId = String(
      call?.linkedConversationId
      || call?.LinkedConversationId
      || entry?.linkedConversationId
      || entry?.LinkedConversationId
      || ''
    ).trim();
    steps.push({
      id: entry?.id || entry?.Id || `${call?.opId || call?.OpId || 'step'}:${index}`,
      kind: 'tool',
      reason: 'tool_call',
      toolName: toolName || 'tool',
      status: entryStatus(entry, call),
      latencyMs: call?.latencyMs || call?.LatencyMs || entry?.latencyMs || entry?.LatencyMs || null,
      startedAt: call?.startedAt || call?.StartedAt || entry?.startedAt || entry?.StartedAt || null,
      completedAt: call?.completedAt || call?.CompletedAt || entry?.completedAt || entry?.CompletedAt || null,
      requestPayloadId: entryPayloadID(entry, call, 'request'),
      responsePayloadId: entryPayloadID(entry, call, 'response'),
      requestPayload: entryPayload(entry, call, 'request'),
      responsePayload: entryPayload(entry, call, 'response'),
      linkedConversationId
    });
  }
  return steps.length > 0 ? [{ steps }] : [];
}

export function mapTranscriptToRows(turns = [], options = {}) {
  const rows = [];
  const queuedTurns = [];
  let runningTurnId = '';
  const list = Array.isArray(turns) ? turns : [];
  const pendingRows = Array.isArray(options?.pendingElicitations) ? options.pendingElicitations : [];
  const pendingByMessageID = new Map();
  const pendingByElicitationID = new Map();
  pendingRows.forEach((entry) => {
    const msgID = String(entry?.messageId || entry?.MessageId || '').trim();
    const elicID = String(entry?.elicitationId || entry?.ElicitationId || '').trim();
    if (msgID) pendingByMessageID.set(msgID, entry);
    if (elicID) pendingByElicitationID.set(elicID, entry);
  });
  const runningTurnIndex = list.findIndex((turn) => isRunningStatus(turn?.status || turn?.Status));
  const holdAfterTurnId = String(options?.holdAfterTurnId || '').trim();
  const holdAfterTurnIndex = holdAfterTurnId
    ? list.findIndex((turn) => String(turn?.id || turn?.Id || '').trim() === holdAfterTurnId)
    : -1;

  const queuedRequestTagPrefix = 'agently:queued_request:';
  const extractQueuedRequest = (tags = '') => {
    try {
      const raw = String(tags || '');
      const idx = raw.indexOf(queuedRequestTagPrefix);
      if (idx === -1) return null;
      let jsonPart = raw.slice(idx + queuedRequestTagPrefix.length).trim();
      if (!jsonPart) return null;
      try {
        return JSON.parse(jsonPart);
      } catch (_) {
        const last = jsonPart.lastIndexOf('}');
        if (last !== -1) {
          jsonPart = jsonPart.slice(0, last + 1);
          return JSON.parse(jsonPart);
        }
      }
      return null;
    } catch (_) {
      return null;
    }
  };

  const turnPreview = (turn = {}, messages = []) => {
    const startedMessageID = turn?.startedByMessageId || turn?.StartedByMessageId || '';
    const starter = messages.find((entry) => (entry?.id || entry?.Id) === startedMessageID)
      || messages.find((entry) => String(entry?.role || entry?.Role || '').toLowerCase() === 'user');
    const queuedMeta = extractQueuedRequest(starter?.tags || starter?.Tags || '');
    const content = String(
      starter?.rawContent
      || starter?.RawContent
      || starter?.content
      || starter?.Content
      || ''
    ).trim();
    const agentOverride = String(
      queuedMeta?.agent
      || turn?.agentIdUsed
      || turn?.AgentIdUsed
      || ''
    ).trim();
    const modelOverride = String(
      queuedMeta?.model
      || turn?.modelOverride
      || turn?.ModelOverride
      || ''
    ).trim();
    const toolOverrides = Array.isArray(queuedMeta?.tools) ? queuedMeta.tools : [];
    return {
      id: turn?.id || turn?.Id || '',
      conversationId: turn?.conversationId || turn?.ConversationId || '',
      status: String(turn?.status || turn?.Status || '').toLowerCase(),
      queueSeq: turn?.queueSeq || turn?.QueueSeq || null,
      content,
      preview: content.slice(0, 220),
      createdAt: turn?.createdAt || turn?.CreatedAt || '',
      overrides: {
        agent: agentOverride,
        model: modelOverride,
        tools: toolOverrides
      }
    };
  };
  const hasPersistedAssistant = (messages = []) => {
    const entries = Array.isArray(messages) ? messages : [];
    return entries.some((entry) => {
      const role = String(entry?.role || entry?.Role || '').toLowerCase();
      const interim = Number(entry?.interim ?? entry?.Interim ?? 0) || 0;
      const content = String(entry?.content || entry?.Content || entry?.rawContent || entry?.RawContent || '').trim();
      return role === 'assistant' && interim === 0 && content !== '';
    });
  };

  for (let turnIndex = 0; turnIndex < list.length; turnIndex++) {
    const turn = list[turnIndex];
    const turnID = turn?.id || turn?.Id || '';
    const turnStatus = String(turn?.status || turn?.Status || '').toLowerCase();
    const messages = Array.isArray(turn?.message || turn?.Message) ? (turn.message || turn.Message) : [];
    const executionGroups = Array.isArray(turn?.executionGroups || turn?.ExecutionGroups)
      ? (turn.executionGroups || turn.ExecutionGroups)
      : [];
    const executionGroupByMessageID = new Map();
    const executionGroupByToolMessageID = new Map();
    for (const group of executionGroups) {
      const messageID = String(group?.modelMessageId || group?.ModelMessageID || group?.parentMessageId || group?.ParentMessageID || '').trim();
      if (messageID) executionGroupByMessageID.set(messageID, group);
      const toolMessages = Array.isArray(group?.toolMessages || group?.ToolMessages) ? (group.toolMessages || group.ToolMessages) : [];
      for (const toolMessage of toolMessages) {
        const toolMessageID = String(toolMessage?.id || toolMessage?.Id || '').trim();
        if (toolMessageID) executionGroupByToolMessageID.set(toolMessageID, group);
      }
      const toolCalls = Array.isArray(group?.toolCalls || group?.ToolCalls) ? (group.toolCalls || group.ToolCalls) : [];
      for (const toolCall of toolCalls) {
        const toolMessageID = String(toolCall?.messageId || toolCall?.MessageId || '').trim();
        if (toolMessageID && !executionGroupByToolMessageID.has(toolMessageID)) {
          executionGroupByToolMessageID.set(toolMessageID, group);
        }
      }
    }
    const linkedByToolName = new Map();
    const linkedByMessageID = new Map();
    const attachedToolMessagesByID = new Map();
    if (!runningTurnId && isRunningStatus(turnStatus)) {
      runningTurnId = turnID;
    }
    const shouldHoldBehindRunningTurn = runningTurnIndex >= 0
      && turnIndex > runningTurnIndex
      && !hasPersistedAssistant(messages);
    const shouldHoldBehindLiveStream = holdAfterTurnIndex >= 0
      && turnIndex > holdAfterTurnIndex;
    if (turnStatus === 'queued' || turnStatus === 'pending' || turnStatus === 'open' || shouldHoldBehindRunningTurn) {
      queuedTurns.push(turnPreview(turn, messages));
      continue;
    }
    if (shouldHoldBehindLiveStream) {
      queuedTurns.push(turnPreview(turn, messages));
      continue;
    }
    for (const message of messages) {
      const attached = Array.isArray(message?.toolMessage || message?.ToolMessage)
        ? (message.toolMessage || message.ToolMessage)
        : [];
      for (const item of attached) {
        const id = String(item?.id || item?.Id || '').trim();
        if (id) attachedToolMessagesByID.set(id, item);
      }
    }
    for (const message of messages) {
      const toolName = String(message?.toolName || message?.ToolName || '').trim();
      const linkedConversationId = String(message?.linkedConversationId || message?.LinkedConversationId || '').trim();
      const messageID = String(message?.id || message?.Id || '').trim();
      if (messageID && linkedConversationId) {
        linkedByMessageID.set(messageID, linkedConversationId);
      }
      if (!toolName || !linkedConversationId) continue;
      linkedByToolName.set(toolName.toLowerCase(), linkedConversationId);
    }
    for (const message of messages) {
      const messageID = String(message?.id || message?.Id || '').trim();
      const attachedToolMessage = attachedToolMessagesByID.get(messageID);
      const role = String(message?.role || message?.Role || '').toLowerCase();
      const type = String(message?.type || message?.Type || '').toLowerCase();
      const toolCall = attachedToolMessage?.toolCall || attachedToolMessage?.ToolCall || null;
      const mappedInput = (() => {
        const pendingElicitation = pendingByMessageID.get(messageID)
          || pendingByElicitationID.get(String(message?.elicitationId || message?.ElicitationId || '').trim())
          || null;
        if (role === 'user') {
          return {
            ...message,
            toolMessage: [],
            ToolMessage: [],
            errorMessage: turn?.errorMessage || turn?.ErrorMessage || message?.errorMessage || message?.ErrorMessage || '',
            executionGroup: null,
            executionGroups: [],
            executionGroupsTotal: 0,
            executionGroupsOffset: 0,
            executionGroupsLimit: 0
          };
        }
        if ((type === 'tool' || type === 'tool_op') && attachedToolMessage) {
          return {
            ...message,
            toolName: message?.toolName || message?.ToolName || attachedToolMessage?.toolName || attachedToolMessage?.ToolName || '',
            ToolName: message?.ToolName || message?.toolName || attachedToolMessage?.ToolName || attachedToolMessage?.toolName || '',
            toolCall,
            ToolCall: toolCall,
            requestPayload: toolCall?.requestPayload || toolCall?.RequestPayload || null,
            RequestPayload: toolCall?.RequestPayload || toolCall?.requestPayload || null,
            responsePayload: toolCall?.responsePayload || toolCall?.ResponsePayload || null,
            ResponsePayload: toolCall?.ResponsePayload || toolCall?.responsePayload || null,
            requestPayloadId: toolCall?.requestPayloadId || toolCall?.RequestPayloadId || '',
            RequestPayloadId: toolCall?.RequestPayloadId || toolCall?.requestPayloadId || '',
            responsePayloadId: toolCall?.responsePayloadId || toolCall?.ResponsePayloadId || '',
            ResponsePayloadId: toolCall?.ResponsePayloadId || toolCall?.responsePayloadId || '',
            linkedConversationId: message?.linkedConversationId || message?.LinkedConversationId || toolCall?.linkedConversationId || toolCall?.LinkedConversationId || '',
            LinkedConversationId: message?.LinkedConversationId || message?.linkedConversationId || toolCall?.LinkedConversationId || toolCall?.linkedConversationId || ''
          };
        }
        return message;
      })();
      const pendingElicitation = pendingByMessageID.get(messageID)
        || pendingByElicitationID.get(String(message?.elicitationId || message?.ElicitationId || '').trim())
        || null;
      const embeddedElicitation = extractEmbeddedElicitationPayload(
        mappedInput?.content
        || mappedInput?.Content
        || mappedInput?.rawContent
        || mappedInput?.RawContent
        || ''
      );
      const resolvedElicitation = mappedInput?.elicitation
        || mappedInput?.Elicitation
        || pendingElicitation?.elicitation
        || pendingElicitation?.Elicitation
        || embeddedElicitation
        || null;
      const suppressExecutionForElicitation = !!resolvedElicitation;
      const normalized = normalizeOne(role === 'user' ? {
        ...mappedInput,
        errorMessage: turn?.errorMessage || turn?.ErrorMessage || mappedInput?.errorMessage || mappedInput?.ErrorMessage || '',
        agentIdUsed: turn?.agentIdUsed || turn?.AgentIdUsed || '',
        AgentIdUsed: turn?.AgentIdUsed || turn?.agentIdUsed || '',
        turnId: turnID,
        turnStatus
      } : {
        ...mappedInput,
        errorMessage: turn?.errorMessage || turn?.ErrorMessage || mappedInput?.errorMessage || mappedInput?.ErrorMessage || '',
        elicitation: resolvedElicitation,
        Elicitation: resolvedElicitation,
        elicitationId: mappedInput?.elicitationId || mappedInput?.ElicitationId || pendingElicitation?.elicitationId || pendingElicitation?.ElicitationId || '',
        ElicitationId: mappedInput?.ElicitationId || mappedInput?.elicitationId || pendingElicitation?.elicitationId || pendingElicitation?.ElicitationId || '',
        conversationId: pendingElicitation?.conversationId || pendingElicitation?.ConversationId || '',
        ConversationId: pendingElicitation?.ConversationId || pendingElicitation?.conversationId || '',
        executionGroup: suppressExecutionForElicitation ? null : (executionGroupByMessageID.get(messageID) || executionGroupByToolMessageID.get(messageID) || null),
        executionGroups: suppressExecutionForElicitation ? [] : executionGroups,
        executionGroupsTotal: suppressExecutionForElicitation ? 0 : (turn?.executionGroupsTotal || turn?.ExecutionGroupsTotal || 0),
        executionGroupsOffset: suppressExecutionForElicitation ? 0 : (turn?.executionGroupsOffset || turn?.ExecutionGroupsOffset || 0),
        executionGroupsLimit: suppressExecutionForElicitation ? 0 : (turn?.executionGroupsLimit || turn?.ExecutionGroupsLimit || 0),
        agentIdUsed: turn?.agentIdUsed || turn?.AgentIdUsed || '',
        AgentIdUsed: turn?.AgentIdUsed || turn?.agentIdUsed || '',
        turnId: turnID,
        turnStatus
      });
      normalized.executions = mapExecutionsFromMessage(mappedInput);
      if (Array.isArray(normalized.executions)) {
        for (const execution of normalized.executions) {
          const steps = Array.isArray(execution?.steps) ? execution.steps : [];
          for (const step of steps) {
            if (String(step?.linkedConversationId || '').trim()) continue;
            const byMessageID = linkedByMessageID.get(String(step?.id || '').trim()) || '';
            if (byMessageID) {
              step.linkedConversationId = byMessageID;
              continue;
            }
            const toolName = String(step?.toolName || '').trim().toLowerCase();
            if (!toolName) continue;
            const linkedConversationId = linkedByToolName.get(toolName) || '';
            if (linkedConversationId) {
              step.linkedConversationId = linkedConversationId;
            }
          }
        }
      }
      rows.push(normalized);
    }
  }

  queuedTurns.sort((a, b) => {
    const aSeq = Number(a?.queueSeq || 0);
    const bSeq = Number(b?.queueSeq || 0);
    if (aSeq !== bSeq) return aSeq - bSeq;
    return String(a?.id || '').localeCompare(String(b?.id || ''));
  });

  rows.sort((a, b) => Date.parse(a.createdAt || 0) - Date.parse(b.createdAt || 0));
  return { rows, queuedTurns, runningTurnId };
}

function findLatestRunningTurnIdFromTurns(turns = []) {
  const list = Array.isArray(turns) ? turns : [];
  for (let i = list.length - 1; i >= 0; i -= 1) {
    const turn = list[i];
    const status = String(turn?.status || turn?.Status || '').toLowerCase().trim();
    if (!isRunningStatus(status)) continue;
    const id = String(turn?.id || turn?.Id || '').trim();
    if (id) return id;
  }
  return '';
}

export function resolveLastTranscriptCursor(turns = []) {
  const list = Array.isArray(turns) ? turns : [];
  for (let turnIndex = list.length - 1; turnIndex >= 0; turnIndex -= 1) {
    const turn = list[turnIndex];
    const messages = Array.isArray(turn?.message || turn?.Message) ? (turn.message || turn.Message) : [];
    for (let messageIndex = messages.length - 1; messageIndex >= 0; messageIndex -= 1) {
      const id = String(messages[messageIndex]?.id || messages[messageIndex]?.Id || '').trim();
      if (!id) continue;
      // Synthetic linked-conversation rows are client-side conveniences, not
      // real transcript anchors. Using them as "since" cursors causes the
      // backend to return overlapping/full transcript pages.
      if (id.startsWith('linked:')) continue;
      return id;
    }
    const turnId = String(turn?.id || turn?.Id || '').trim();
    if (turnId) return turnId;
  }
  return '';
}

function findLatestRunningTurnId(messages = []) {
  const list = Array.isArray(messages) ? messages : [];
  for (let index = list.length - 1; index >= 0; index--) {
    const item = list[index];
    if (isRunningStatus(item?.turnStatus || item?.status)) {
      return item?.turnId || '';
    }
  }
  return '';
}

export function ensureContextResources(context) {
  context.resources = context.resources || {};
  context.resources.chat = context.resources.chat || {};
  context.resources.chat.iterationVisibleCount = DEFAULT_VISIBLE_ITERATIONS;
  context.resources.chat.transcriptRows = Array.isArray(context.resources.chat.transcriptRows) ? context.resources.chat.transcriptRows : [];
  context.resources.chat.liveRows = Array.isArray(context.resources.chat.liveRows) ? context.resources.chat.liveRows : [];
  context.resources.chat.renderRows = Array.isArray(context.resources.chat.renderRows) ? context.resources.chat.renderRows : [];
  context.resources.chat.liveOwnedConversationID = String(context.resources.chat.liveOwnedConversationID || '').trim();
  context.resources.chat.liveOwnedTurnIds = Array.isArray(context.resources.chat.liveOwnedTurnIds) ? context.resources.chat.liveOwnedTurnIds : [];
  context.resources.chat.pendingTextDeltaQueue = Array.isArray(context.resources.chat.pendingTextDeltaQueue) ? context.resources.chat.pendingTextDeltaQueue : [];
  return context.resources.chat;
}

function queuePostTurnConversationRefresh(context, conversationID = '', turnID = '') {
  if (typeof window === 'undefined') return;
  const chatState = ensureContextResources(context);
  const targetConversationID = String(conversationID || '').trim();
  const targetTurnID = String(turnID || '').trim();
  if (!targetConversationID) return;
  const refreshKey = `${targetConversationID}:${targetTurnID}`;
  chatState.postTurnRefreshKey = refreshKey;
  if (chatState.postTurnRefreshTimer) {
    clearTimeout(chatState.postTurnRefreshTimer);
    chatState.postTurnRefreshTimer = null;
  }
  chatState.postTurnRefreshTimer = window.setTimeout(async () => {
    chatState.postTurnRefreshTimer = null;
    if (String(chatState.postTurnRefreshKey || '').trim() !== refreshKey) return;
    if (String(getCurrentConversationID(context) || '').trim() !== targetConversationID) return;
    const activeTurnID = String(chatState.runningTurnId || chatState.activeStreamTurnId || '').trim();
    if (activeTurnID && activeTurnID !== targetTurnID) return;
    try {
      await switchConversation(context, targetConversationID);
    } catch (_) {
      // Best-effort: preserve the current live render if refresh fails.
    }
  }, 90);
}

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

export function getVisibleIterations(context) {
  ensureContextResources(context);
  return DEFAULT_VISIBLE_ITERATIONS;
}

export function normalizeForContext(context, rows = []) {
  return normalizeMessages(rows, { visibleCount: getVisibleIterations(context) });
}

export function renderMergedRowsForContext(context) {
  const messagesDS = context?.Context?.('messages')?.handlers?.dataSource;
  if (!messagesDS) return [];
  const chatState = ensureContextResources(context);
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  const conversationForm = conversationsDS?.peekFormData?.() || {};
  const metaForm = metaDS?.peekFormData?.() || {};
  const hasLiveRows = Array.isArray(chatState.liveRows) && chatState.liveRows.length > 0;
  const mergedRows = mergeRenderedRows({
    transcriptRows: filterLiveOwnedTranscriptRows(
      chatState.transcriptRows,
      getCurrentConversationID(context),
      chatState.liveOwnedConversationID,
      chatState.liveOwnedTurnIds
    ),
    liveRows: chatState.liveRows,
    runningTurnId: chatState.runningTurnId,
    hasRunning: chatState.lastHasRunning || hasLiveRows,
    findLatestRunningTurnId,
    currentConversationID: getCurrentConversationID(context),
    liveOwnedConversationID: chatState.liveOwnedConversationID,
    liveOwnedTurnIds: chatState.liveOwnedTurnIds
  });
  chatState.renderRows = mergedRows;
  const normalizedRows = normalizeForContext(context, mergedRows);
  if (isStreamDebugEnabled()) {
    console.log('[render]', {
      conversationId: getCurrentConversationID(context),
      liveCount: Array.isArray(chatState.liveRows) ? chatState.liveRows.length : 0,
      mergedCount: mergedRows.length,
      normalizedCount: normalizedRows.length,
      normalizedTypes: normalizedRows.map((row) => ({
        id: row?.id,
        type: row?._type || row?.role,
        mode: row?.mode || '',
        head: String(row?.content || '').slice(0, 60)
      })),
      liveRows: chatState.liveRows.map((r) => ({
        id: r?.id, role: r?.role, turnId: r?.turnId, interim: r?.interim,
        contentHead: String(r?.content || '').slice(0, 50),
        groups: (r?.executionGroups || []).length,
        toolSteps: (r?.executionGroups || []).flatMap((g) => g?.toolSteps || []).length
      }))
    });
  }
  // liveStreamStore already normalizes streaming content into row.content.
  // Do not overwrite it with raw _streamContent here, or markdown/chart fences
  // leak into the bubble during streaming instead of rendering as rich content.
  const resolvedRows = mergedRows;
  const queuedTurns = Array.isArray(conversationForm?.queuedTurns) ? conversationForm.queuedTurns : [];
  const queueRow = queuedTurns.length > 0 ? {
    _type: 'queue',
    id: `queue:${String(conversationForm?.id || '').trim()}:${queuedTurns.map((item) => String(item?.id || '').trim()).filter(Boolean).join(',')}`,
    createdAt: new Date().toISOString(),
    running: !!conversationForm?.running,
    queuedTurns
  } : null;
  const selectedAgent = resolveVisibleSelectedAgent(
    metaForm,
    conversationForm?.agent,
    getPersistedSelectedAgent(),
    metaForm?.agent,
    metaForm?.defaults?.agent
  );
  const starterTasks = resolveStarterTasks({
    agentInfos: Array.isArray(metaForm?.agentInfos) ? metaForm.agentInfos : [],
    selectedAgent
  });
  const hasConversationId = String(conversationForm?.id || '').trim() !== '';
  const hasVisibleConversationContent = resolvedRows.some((row) => {
    const type = String(row?._type || '').toLowerCase();
    return type !== 'starter' && type !== 'queue';
  });
  const starterRow = resolvedRows.length === 0
    && !hasConversationId
    && !hasVisibleConversationContent
    && !queueRow
    && starterTasks.length > 0 ? {
    _type: 'starter',
    id: `starter:${selectedAgent || 'default'}`,
    createdAt: new Date().toISOString(),
    title: selectedAgent === 'auto' ? 'Explore All Agents' : 'Start With A Prompt',
    subtitle: selectedAgent === 'auto' ? 'Auto-select agent' : '',
    starterTasks
  } : null;
  const renderCollection = [
    ...resolvedRows,
    ...(starterRow ? [starterRow] : []),
    ...(queueRow ? [queueRow] : [])
  ];
  messagesDS.setCollection?.(renderCollection);
  return mergedRows;
}

function resolveTurnStarterPreview(turn = {}) {
  const messages = Array.isArray(turn?.message || turn?.Message) ? (turn.message || turn.Message) : [];
  const startedMessageID = String(turn?.startedByMessageId || turn?.StartedByMessageId || '').trim();
  const starter = messages.find((entry) => String(entry?.id || entry?.Id || '').trim() === startedMessageID)
    || messages.find((entry) => String(entry?.role || entry?.Role || '').toLowerCase() === 'user');
  return String(
    starter?.rawContent
    || starter?.RawContent
    || starter?.content
    || starter?.Content
    || ''
  ).trim();
}

function isVisibleExecutionPage(page = {}) {
  if (!page || typeof page !== 'object') return false;
  const status = String(page?.status || '').trim().toLowerCase();
  const hasVisibleContent = String(page?.preamble || '').trim() !== '' || String(page?.content || '').trim() !== '';
  const hasTools = (Array.isArray(page?.toolSteps) && page.toolSteps.length > 0)
    || (Array.isArray(page?.toolCallsPlanned) && page.toolCallsPlanned.length > 0);
  const isActive = ['running', 'thinking', 'streaming', 'processing', 'in_progress', 'waiting_for_user', 'tool_calls'].includes(status);
  return hasVisibleContent || hasTools || isActive;
}

function resolveActiveStreamTurnId(turns = [], chatState = {}) {
  const explicit = String(chatState?.activeStreamTurnId || '').trim();
  if (explicit) return explicit;
  const preview = String(chatState?.activeStreamPrompt || '').trim();
  if (!preview) return '';
  const list = Array.isArray(turns) ? turns : [];
  for (const turn of list) {
    const turnID = String(turn?.id || turn?.Id || '').trim();
    if (!turnID) continue;
    if (resolveTurnStarterPreview(turn) === preview) {
      return turnID;
    }
  }
  return '';
}

function getCurrentConversationID(context) {
  const form = context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.() || {};
  return String(form?.id || '').trim();
}

function getContextWindowId(context) {
  return String(context?.identity?.windowId || '').trim() || MAIN_CHAT_WINDOW_ID;
}

function extractEmbeddedElicitationPayload(text = '') {
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
    return parsed;
  } catch (_) {
    return null;
  }
}

export function publishActiveConversation(conversationID = '', context = null) {
  if (typeof window === 'undefined') return;
  const id = String(conversationID || '').trim();
  const windowId = getContextWindowId(context);
  try {
    setScopedConversationSelection(windowId, id);
    if (isMainChatWindowId(windowId)) {
      syncConversationPath(id);
    }
    window.dispatchEvent(new CustomEvent('agently:conversation-active', { detail: { id, windowId } }));
  } catch (_) {}
}

export function conversationIDFromPath(pathname = '') {
  const value = String(pathname || '').trim();
  if (!value) return '';
  const prefixes = ['/v1/conversation/', '/conversation/', '/ui/conversation/'];
  for (const prefix of prefixes) {
    if (value.startsWith(prefix)) {
      const raw = value.slice(prefix.length).split('/')[0];
      return String(raw || '').trim();
    }
  }
  return '';
}

function conversationPathForID(conversationID = '') {
  const id = String(conversationID || '').trim();
  if (!id) {
    if (typeof window !== 'undefined') {
      const current = String(window.location?.pathname || '').trim();
      if (current.startsWith('/ui/')) return '/ui';
    }
    return '/';
  }
  const encoded = encodeURIComponent(id);
  if (typeof window !== 'undefined') {
    const port = String(window.location?.port || '').trim();
    const host = String(window.location?.hostname || '').trim();
    if (port === '5173' || host === '127.0.0.1') {
      return `/conversation/${encoded}`;
    }
  }
  return `/v1/conversation/${encoded}`;
}

function syncConversationPath(conversationID = '') {
  if (typeof window === 'undefined') return;
  const target = conversationPathForID(conversationID);
  const current = `${window.location.pathname || ''}`;
  if (current === target) return;
  if (current.startsWith('/v1/api/')) return;
  try {
    window.history.replaceState(window.history.state, '', target);
  } catch (_) {}
}

export function resolveUserID(context) {
  const conversationsForm = context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.() || {};
  const metaForm = context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.() || {};
  const explicit = String(conversationsForm?.userId || metaForm?.defaults?.userId || '').trim();
  return explicit;
}

export async function fetchTranscript(conversationID, since = '', options = {}) {
  if (isStreamDebugEnabled()) {
    console.log('[transcript-fetch]', { conversationID, since });
  }
  const transcriptInput = {
    conversationId: conversationID,
    includeModelCalls: true,
    includeToolCalls: true,
    includeFeeds: true,
    since: since || undefined,
  };
  const transcriptOptions = options?.selectors
    ? { selectors: options.selectors }
    : undefined;
  const payload = await client.getTranscript(transcriptInput, transcriptOptions);
  const data = payload || {};
  // Populate tool feed bar from transcript feeds (for page reload / conversation switch).
  if (Array.isArray(data?.feeds) && data.feeds.length > 0) {
    for (const feed of data.feeds) {
      if (feed?.feedId) {
        applyFeedEvent({
          type: 'tool_feed_active',
          feedId: feed.feedId,
          feedTitle: feed.title || feed.feedId,
          feedItemCount: feed.itemCount || 0,
          feedData: feed.data || null,
          conversationId: conversationID,
        });
      }
    }
  }
  // Canonical ConversationState: extract turns with pages as executionGroups
  if (Array.isArray(data?.turns) && data.turns.length > 0 && ('turnId' in data.turns[0] || 'execution' in data.turns[0])) {
    return data.turns.map((turn) => {
      const pages = Array.isArray(turn.execution?.pages) ? turn.execution.pages : [];
      const summaryPages = pages.filter((page) => Number(page?.iteration || 0) === 0);
      const visiblePages = pages.filter((page) => Number(page?.iteration || 0) !== 0 && isVisibleExecutionPage(page));
      const messages = [];
      if (turn.user) {
        messages.push({
          id: turn.user.messageId || '',
          role: 'user',
          content: turn.user.content || '',
          turnId: turn.turnId || '',
          createdAt: turn.createdAt || ''
        });
      }
      // Create one assistant message per turn carrying execution pages.
      // The pages themselves hold all model/tool step data — no per-page
      // assistant messages to avoid duplicate rendering.
      if (visiblePages.length > 0) {
        const lastPage = visiblePages[visiblePages.length - 1];
        const finalPage = [...visiblePages].reverse().find((p) => p.finalResponse) || lastPage;
        messages.push({
          id: finalPage?.assistantMessageId || finalPage?.pageId || turn.turnId || '',
          role: 'assistant',
          interim: finalPage?.finalResponse ? 0 : 1,
          content: finalPage?.content || '',
          preamble: visiblePages[0]?.preamble || '',
          turnId: turn.turnId || '',
          status: finalPage?.status || '',
          createdAt: turn.createdAt || ''
        });
      }
      if (summaryPages.length > 0) {
        const summaryPage = summaryPages[summaryPages.length - 1];
        messages.push({
          id: summaryPage?.assistantMessageId || summaryPage?.pageId || `summary:${turn.turnId || ''}`,
          role: 'assistant',
          mode: 'summary',
          interim: 0,
          content: summaryPage?.content || '',
          turnId: turn.turnId || '',
          status: summaryPage?.status || '',
          createdAt: turn.createdAt || ''
        });
      }
      if (turn.elicitation) {
        const elic = turn.elicitation;
        messages.push({
          id: `elicitation:${elic.elicitationId || turn.turnId}`,
          role: 'assistant',
          interim: 0,
          content: elic.message || '',
          turnId: turn.turnId || '',
          status: elic.status || 'pending',
          elicitationId: elic.elicitationId || '',
          elicitation: {
            elicitationId: elic.elicitationId || '',
            message: elic.message || '',
            requestedSchema: elic.requestedSchema || null,
            callbackURL: elic.callbackUrl || ''
          }
        });
      }
      for (const lc of (turn.linkedConversations || [])) {
        if (!lc?.conversationId) continue;
        const existing = messages.find((m) => m.linkedConversationId === lc.conversationId);
        if (!existing) {
          messages.push({
            id: `linked:${lc.conversationId}`,
            role: 'tool',
            type: 'tool',
            turnId: turn.turnId || '',
            linkedConversationId: lc.conversationId,
            createdAt: lc.createdAt || ''
          });
        }
      }
      return {
        id: turn.turnId || '',
        status: turn.status || '',
        createdAt: turn.createdAt || '',
        message: messages,
        executionGroups: visiblePages,
        executionGroupsTotal: visiblePages.length,
        executionGroupsOffset: 0,
        executionGroupsLimit: visiblePages.length
      };
    });
  }
  return Array.isArray(data?.turns || data?.Turns) ? (data.turns || data.Turns) : [];
}

export async function fetchPendingElicitations(conversationID = '') {
  const id = String(conversationID || '').trim();
  if (!id) return [];
  return client.listPendingElicitations(id);
}

export async function fetchConversation(conversationID = '') {
  const id = String(conversationID || '').trim();
  if (!id) return null;
  try {
    const data = await client.getConversation(id);
    if (!data || typeof data !== 'object') return null;
    const resolvedID = String(data?.id || data?.Id || '').trim();
    if (resolvedID) {
      // Update usage display from conversation data.
      publishUsage(resolvedID, data);
      return data;
    }
    return null;
  } catch (_) {
    return null;
  }
}

function applyConversationFormSnapshot(base = {}, conversation = null) {
  if (!conversation || typeof conversation !== 'object') return { ...base };
  const next = { ...base };
  const conversationID = String(conversation?.id || conversation?.Id || '').trim();
  const title = String(conversation?.title || conversation?.Title || '').trim();
  const summary = String(conversation?.summary || conversation?.Summary || '').trim();
  const agent = String(conversation?.agentId || conversation?.AgentId || '').trim();
  const model = String(conversation?.defaultModel || conversation?.DefaultModel || '').trim();
  const embedder = String(conversation?.defaultEmbedder || conversation?.DefaultEmbedder || '').trim();
  if (conversationID) next.id = conversationID;
  if (title) next.title = title;
  if (summary) next.summary = summary;
  if (agent) next.agent = agent;
  if (model) next.model = model;
  if (embedder) next.embedder = embedder;
  return next;
}

export async function hydrateMeta(context) {
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  if (!metaDS) return;
  const current = metaDS.peekFormData?.() || {};
  if (Array.isArray(current?.agentOptions) && current.agentOptions.length > 0 &&
      Array.isArray(current?.modelOptions) && current.modelOptions.length > 0) {
    return;
  }
  try {
    const raw = await client.getWorkspaceMetadata();
    const payload = normalizeMetaResponse(raw);
    metaDS.setFormData?.({ values: payload });
    const convDS = context?.Context?.('conversations')?.handlers?.dataSource;
    if (convDS) {
      const form = convDS.peekFormData?.() || {};
      const next = { ...form };
      if (!String(next.id || '').trim()) {
        next.agent = payload?.defaults?.agent || '';
        next.model = payload?.defaults?.model || '';
        next.embedder = payload?.defaults?.embedder || '';
      } else {
        if (!next.agent && payload?.defaults?.agent) next.agent = payload.defaults.agent;
        if (!next.model && payload?.defaults?.model) next.model = payload.defaults.model;
        if (!next.embedder && payload?.defaults?.embedder) next.embedder = payload.defaults.embedder;
      }
      convDS.setFormData?.({ values: next });
    }
  } catch (_) {
    // best-effort: fall back to datasource-driven metadata fetch
  }
}

export function syncMessagesSnapshot(context, turns, reason = 'poll', pendingElicitations = []) {
  const chatState = ensureContextResources(context);
  const snapshot = syncTranscriptSnapshotStore({
    context,
    turns,
    pendingElicitations,
    reason,
    ensureContextResources,
    resolveActiveStreamTurnId,
    mapTranscriptToRows,
    findLatestRunningTurnIdFromTurns,
    findLatestRunningTurnId,
    publishChangeFeed,
    publishPlanFeed,
    setStage,
    liveRows: chatState.liveRows
  });
  if (Array.isArray(snapshot?.liveRows)) {
    chatState.liveRows = snapshot.liveRows;
  }
  return renderMergedRowsForContext(context);
}

function shouldDeferTranscriptToLiveStream(context, conversationID = '') {
  const chatState = ensureContextResources(context);
  const targetID = String(conversationID || getCurrentConversationID(context) || '').trim();
  if (!targetID) return false;
  if (!shouldUseLiveStream(context, targetID)) return false;
  return !!(
    String(chatState.runningTurnId || '').trim()
    || String(chatState.activeStreamTurnId || '').trim()
    || chatState.lastHasRunning
  );
}

export async function dsTick(context, options = {}) {
  const requestedConversationID = String(options?.conversationID || getCurrentConversationID(context) || '').trim();
  if (shouldDeferTranscriptToLiveStream(context, requestedConversationID)) {
    return {
      transcriptRows: ensureContextResources(context).transcriptRows,
      liveRows: ensureContextResources(context).liveRows,
      queuedTurns: ensureContextResources(context).lastQueuedTurns || [],
      hasRunning: true,
      runningTurnId: ensureContextResources(context).runningTurnId || ensureContextResources(context).activeStreamTurnId || '',
      conversationID: requestedConversationID,
      deferredToLiveStream: true
    };
  }
  const result = await tickTranscript({
    context,
    options,
    ensureContextResources,
    fetchTranscript,
    fetchPendingElicitations,
    resolveLastTranscriptCursor,
    syncTranscriptSnapshot: ({ context: nextContext, turns, pendingElicitations, reason }) => (
      syncMessagesSnapshot(nextContext, turns, reason, pendingElicitations)
    )
  });
  const chatState = ensureContextResources(context);
  const conversationID = String(result?.conversationID || getCurrentConversationID(context) || '').trim();
  const ownsLiveTransport = shouldUseLiveStream(context, conversationID);
  if (result?.hasRunning && conversationID && !ownsLiveTransport && !chatState.stream) {
    queueTranscriptRefresh(context, { delay: 900 });
  }
  return result;
}

export function resetConversationSnapshotState(context) {
  const chatState = ensureContextResources(context);
  clearPendingStreamScheduling(chatState);
  resetTranscriptState({
    context,
    ensureContextResources,
    clearChangeFeed,
    clearPlanFeed,
    getCurrentConversationID
  });
  resetLiveStreamState(chatState);
  chatState.renderRows = [];
}

export function queueTranscriptRefresh(context, { delay = 120, resetSince = false } = {}) {
  const currentConversationID = String(getCurrentConversationID(context) || '').trim();
  if (shouldDeferTranscriptToLiveStream(context, currentConversationID)) {
    return null;
  }
  return queueTranscriptRefreshStore({
    context,
    delay,
    resetSince,
    ensureContextResources,
    resetTranscriptState: ({ context: nextContext }) => resetConversationSnapshotState(nextContext),
    tickTranscript: ({ context: nextContext }) => dsTick(nextContext)
  });
}

export function connectStream(context, conversationID) {
  const chatState = ensureContextResources(context);
  if (chatState.stream) {
    logStreamDebug(chatState, 'stream-close-replaced', {
      conversationId: String(chatState.activeConversationID || '').trim()
    });
    chatState.stream.close();
    chatState.stream = null;
  }
  const subscription = client.streamEvents(conversationID, {
    onEvent: (payload) => {
      handleStreamEvent(chatState, context, conversationID, payload);
    },
    onError: () => {
      logStreamDebug(chatState, 'stream-error', {
        conversationId: String(conversationID || '').trim()
      });
    },
  });
  chatState.stream = subscription;
  chatState.activeConversationID = String(conversationID || '').trim();
  chatState.streamOpenedAt = Date.now();
  logStreamDebug(chatState, 'stream-connect', {
    conversationId: String(conversationID || '').trim()
  });
}

export function handleStreamEvent(chatState, context, conversationID, payload) {
    const type = String(payload?.type || '').toLowerCase();
    const eventConversationID = resolveStreamEventConversationID(payload, conversationID);
    if (eventConversationID && SIDEBAR_ACTIVITY_EVENT_TYPES.has(type)) {
      publishConversationActivity(eventConversationID, {
        type,
        turnId: String(payload?.turnId || '').trim(),
        linkedConversationId: String(payload?.linkedConversationId || '').trim(),
        status: String(payload?.status || '').trim()
      });
    }
    const contextWindowID = getContextWindowId(context);
    const scopedConversationID = getScopedConversationSelection(contextWindowID);
    const formConversationID = getCurrentConversationID(context);
    const visibleConversationID = String(formConversationID || scopedConversationID || '').trim();
    if (!shouldProcessStreamEvent({ payload, subscribedConversationID: conversationID, visibleConversationID })) {
      logStreamDebug(chatState, 'stream-event-ignored', {
        type,
        eventConversationId: eventConversationID,
        visibleConversationId: visibleConversationID,
        windowId: contextWindowID,
      });
      return;
    }
    if (shouldIgnoreExecutionStreamEvent(payload)) {
      logStreamDebug(chatState, 'stream-event-ignored-mode', {
        type,
        mode: streamEventMode(payload),
        eventConversationId: eventConversationID,
        visibleConversationId: visibleConversationID,
        windowId: contextWindowID,
      });
      return;
    }
    const payloadSize = (() => {
      try {
        return JSON.stringify(payload || {}).length;
      } catch (_) {
        return 0;
      }
    })();
    const turnId = String(payload?.turnId || payload?.patch?.turnId || '').trim();
    chatState.terminalTurns = chatState.terminalTurns || {};
    if (turnId && chatState.terminalTurns[turnId] && type !== 'turn_completed' && type !== 'turn_failed' && type !== 'turn_canceled') {
      logExecutorDebug('post-terminal-event', {
        type,
        conversationId: payload?.conversationId || payload?.streamId || conversationID,
        turnId,
        terminalAt: chatState.terminalTurns[turnId],
        status: String(payload?.status || payload?.patch?.status || '').trim()
      });
      if (isLatePostTerminalExecutionEvent(type, payload)) {
        logStreamDebug(chatState, 'stream-event-ignored-terminal', {
          type,
          turnId,
          terminalAt: chatState.terminalTurns[turnId],
          eventConversationId: eventConversationID,
          visibleConversationId: visibleConversationID
        });
        return;
      }
    }
    if (isStreamDebugEnabled()) {
      console.log('[stream-event]', type, {
        conversationId: payload?.conversationId || payload?.streamId || conversationID,
        turnId: payload?.turnId,
        mode: payload?.mode || payload?.patch?.mode,
        agentIdUsed: payload?.agentIdUsed,
        agentName: payload?.agentName,
        userMessageId: payload?.userMessageId,
        assistantMessageId: payload?.assistantMessageId,
        parentMessageId: payload?.parentMessageId,
        modelCallId: payload?.modelCallId,
        status: payload?.status,
        finalResponse: payload?.finalResponse,
        iteration: payload?.iteration,
        pageIndex: payload?.pageIndex,
        pageCount: payload?.pageCount,
        interim: payload?.patch?.interim ?? payload?.interim,
        createdAt: payload?.createdAt,
        startedAt: payload?.startedAt,
        completedAt: payload?.completedAt,
        contentLen: String(payload?.content || payload?.patch?.content || '').length,
        toolCallsPlanned: payload?.toolCallsPlanned?.length,
        toolCallId: payload?.toolCallId,
        toolMessageId: payload?.toolMessageId,
        toolName: payload?.toolName,
        linkedConversationId: payload?.linkedConversationId,
        elicitationId: payload?.elicitationId,
        requestPayloadId: payload?.requestPayloadId,
        responsePayloadId: payload?.responsePayloadId,
        providerRequestPayloadId: payload?.providerRequestPayloadId,
        providerResponsePayloadId: payload?.providerResponsePayloadId,
        streamPayloadId: payload?.streamPayloadId,
        op: payload?.op,
        id: payload?.id
      });
    }
    logStreamDebug(chatState, 'stream-event', {
      type,
      eventSize: payloadSize,
      payloadConversationId: eventConversationID,
      payloadTurnId: String(payload?.turnId || payload?.patch?.turnId || '').trim(),
      payloadMode: String(payload?.mode || payload?.patch?.mode || '').trim(),
      payloadAgentIdUsed: String(payload?.agentIdUsed || '').trim(),
      payloadAgentName: String(payload?.agentName || '').trim(),
      payloadCreatedAt: String(payload?.createdAt || '').trim(),
      payloadStartedAt: String(payload?.startedAt || '').trim(),
      payloadCompletedAt: String(payload?.completedAt || '').trim(),
      payloadUserMessageId: String(payload?.userMessageId || '').trim(),
      payloadAssistantMessageId: String(payload?.assistantMessageId || '').trim(),
      payloadParentMessageId: String(payload?.parentMessageId || '').trim(),
      payloadModelCallId: String(payload?.modelCallId || '').trim(),
      payloadToolCallId: String(payload?.toolCallId || '').trim(),
      payloadToolMessageId: String(payload?.toolMessageId || '').trim(),
      payloadRequestPayloadId: String(payload?.requestPayloadId || '').trim(),
      payloadResponsePayloadId: String(payload?.responsePayloadId || '').trim(),
      payloadProviderRequestPayloadId: String(payload?.providerRequestPayloadId || '').trim(),
      payloadProviderResponsePayloadId: String(payload?.providerResponsePayloadId || '').trim(),
      payloadStreamPayloadId: String(payload?.streamPayloadId || '').trim(),
      payloadLinkedConversationId: String(payload?.linkedConversationId || '').trim(),
      payloadIteration: Number(payload?.iteration || 0) || 0,
      payloadPageIndex: Number(payload?.pageIndex || 0) || 0,
      payloadPageCount: Number(payload?.pageCount || 0) || 0
    });

    if (type !== 'text_delta') {
      flushQueuedTextDeltas(chatState, context, conversationID);
    }

    if (type === 'text_delta') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      setStage({ phase: 'streaming', text: 'Streaming response…' });
      enqueueTextDelta(chatState, payload, conversationID);
      const queue = Array.isArray(chatState.pendingTextDeltaQueue) ? chatState.pendingTextDeltaQueue : [];
      const mergedPayload = queue[queue.length - 1] || payload;
      const streamID = String(mergedPayload?.streamId || conversationID);
      const streamMessageID = String(mergedPayload?.id || '').trim();
      const activeStreamRow = [...(Array.isArray(chatState.liveRows) ? chatState.liveRows : [])].reverse().find((row) => row?.isStreaming && String(row?.role || '').toLowerCase() === 'assistant');
      logStreamDebug(chatState, 'stream-chunk-merged', {
        streamId: streamID,
        streamMessageId: streamMessageID,
        chunkChars: String(payload?.content || '').length,
        queuedChars: String(mergedPayload?.content || '').length,
        totalChars: String(activeStreamRow?._streamContent || '').length,
        rowCount: Array.isArray(chatState.liveRows) ? chatState.liveRows.length : 0,
        turnId: String(activeStreamRow?.turnId || '').trim()
      });
      scheduleTextDeltaFlush(context, chatState, conversationID);
      return;
    }

    if (type === 'reasoning_delta') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      setStage({ phase: 'streaming', text: 'Assistant reasoning…' });
      return;
    }

    if (type === 'tool_call_delta') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      setStage({ phase: 'executing', text: `Building ${String(payload?.toolName || 'tool')} arguments…` });
      return;
    }

      if (type === 'model_started') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
      if (conversationsDS) {
        const convForm = conversationsDS.peekFormData?.() || {};
        conversationsDS.setFormData?.({
          values: {
            ...convForm,
            running: true
          }
        });
      }
      if (String(payload?.turnId || '').trim()) {
        chatState.activeStreamTurnId = String(payload.turnId).trim();
        chatState.runningTurnId = String(payload.turnId).trim();
        markLiveOwnedTurn(chatState, conversationID, String(payload.turnId).trim());
        applyTurnStartedEvent(chatState, enrichPayloadWithTurnAgent(chatState, context, payload), conversationID);
      }
      if (!chatState.activeStreamStartedAt) {
        chatState.activeStreamStartedAt = Date.now();
      }
      // Inject the active turn ID when the backend omits it (e.g. status='streaming')
      // so the execution row merges into the existing assistant row for this turn.
      const enrichedPayload = enrichPayloadWithTurnAgent(chatState, context, payload);
      if (!String(enrichedPayload.turnId || '').trim() && chatState.activeStreamTurnId) {
        enrichedPayload.turnId = chatState.activeStreamTurnId;
      }
      applyExecutionStreamEvent(chatState, enrichedPayload, conversationID);
      setStage({ phase: 'executing', text: 'Assistant executing…' });
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'model_completed') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      if (String(payload?.turnId || '').trim()) {
        markLiveOwnedTurn(chatState, conversationID, String(payload.turnId).trim());
      }
      const enrichedPayload = enrichPayloadWithTurnAgent(chatState, context, payload);
      if (!String(enrichedPayload.turnId || '').trim() && chatState.activeStreamTurnId) {
        enrichedPayload.turnId = chatState.activeStreamTurnId;
      }
      applyExecutionStreamEvent(chatState, enrichedPayload, conversationID);
      if (payload?.finalResponse) {
        finalizeStreamTurn(chatState, payload, conversationID);
        setStage({ phase: 'done', text: 'Done' });
        window.setTimeout(() => setStage({ phase: 'ready', text: 'Ready' }), 1100);
      } else {
        setStage({ phase: 'executing', text: 'Assistant thinking…' });
      }
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'tool_calls_planned') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      // tool_calls_planned is emitted by the reactor when the LLM plans tool
      // calls. It carries toolCallsPlanned and content/preamble. Update the
      // execution row so planned tools appear immediately in the UI.
      applyExecutionStreamEvent(chatState, enrichPayloadWithTurnAgent(chatState, context, payload), conversationID);
      setStage({ phase: 'executing', text: 'Planning tool calls…' });
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'tool_call_started' || type === 'tool_call_completed') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      logStreamDebug(chatState, `stream-${type}`, {
        turnId: String(payload?.turnId || '').trim(),
        assistantMessageId: String(payload?.assistantMessageId || '').trim(),
        toolCallId: String(payload?.toolCallId || '').trim(),
        toolMessageId: String(payload?.toolMessageId || '').trim(),
        toolName: String(payload?.toolName || '').trim(),
        status: String(payload?.status || '').trim()
      });
      const toolPayload = enrichPayloadWithTurnAgent(chatState, context, payload);
      if (!String(toolPayload.turnId || '').trim() && chatState.activeStreamTurnId) {
        toolPayload.turnId = chatState.activeStreamTurnId;
      }
      applyToolStreamEvent(chatState, toolPayload, conversationID);
      setStage({
        phase: type === 'tool_call_completed' ? 'executing' : 'executing',
        text: `${type === 'tool_call_completed' ? 'Completed' : 'Executing'} ${String(payload?.toolName || 'tool')}…`
      });
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'control') {
      chatState.lastStreamEventAt = Date.now();
      const op = String(payload?.op || '').toLowerCase();
      if (op === 'turn_started') {
        chatState.lastHasRunning = true;
        rememberTurnAgent(chatState, context, payload);
        const turnId = String(payload?.patch?.turnId || '').trim();
        if (turnId) {
          chatState.activeStreamTurnId = turnId;
          chatState.runningTurnId = turnId;
          markLiveOwnedTurn(chatState, conversationID, turnId);
          applyTurnStartedEvent(chatState, {
            ...(payload?.patch && typeof payload.patch === 'object' ? payload.patch : {}),
            turnId,
            conversationId: conversationID,
            createdAt: payload?.createdAt || payload?.patch?.createdAt || new Date().toISOString(),
            agentName: String(chatState?.activeTurnAgentName || '').trim()
          }, conversationID);
        }
        if (!chatState.activeStreamStartedAt) {
          chatState.activeStreamStartedAt = Date.now();
        }
        logStreamDebug(chatState, 'stream-control-turn-started', {
          turnId,
          status: String(payload?.patch?.status || '').trim(),
          agentIdUsed: String(payload?.patch?.agentIdUsed || '').trim(),
          agentName: String(chatState?.activeTurnAgentName || '').trim()
        });
        setStage({ phase: 'executing', text: 'Assistant executing…' });
      } else if (op === 'message_patch') {
        chatState.lastHasRunning = true;
        logStreamDebug(chatState, 'stream-control-message-patch', {
          op: String(payload?.op || '').trim(),
          messageId: String(payload?.id || '').trim()
        });
        applyMessagePatchEvent(chatState, payload);
        renderMergedRowsForContext(context);
      } else {
        logStreamDebug(chatState, 'stream-control', {
          op: String(payload?.op || '').trim()
        });
      }
      return;
    }

    if (type === 'turn_completed' || type === 'turn_failed' || type === 'turn_canceled') {
      const completedTurnID = String(payload?.turnId || '').trim();
      const resolvedConversationID = String(payload?.conversationId || payload?.streamId || conversationID || '').trim();
      const finalRow = Array.isArray(chatState.liveRows)
        ? [...chatState.liveRows].reverse().find((row) => String(row?.turnId || '').trim() === completedTurnID && String(row?.role || '').toLowerCase() === 'assistant')
        : null;
      const finalContent = String(payload?.content || finalRow?.content || '').trim();
      logExecutorDebug('turn-terminal', {
        type,
        conversationId: resolvedConversationID,
        turnId: completedTurnID,
        runningTurnId: String(chatState.runningTurnId || '').trim(),
        activeStreamTurnId: String(chatState.activeStreamTurnId || '').trim(),
        status: String(payload?.status || type).trim(),
        hasFinalContent: finalContent !== '',
        linkedConversationCount: Array.isArray(finalRow?.executionGroups)
          ? finalRow.executionGroups.flatMap((group) => group?.toolSteps || []).filter((step) => String(step?.linkedConversationId || '').trim()).length
          : 0
      });
      if (finalContent === '') {
        logExecutorDebug('phantom-terminal', {
          type,
          conversationId: resolvedConversationID,
          turnId: completedTurnID,
          reason: 'terminal-event-without-final-content'
        });
      }
      if (turnId) {
        chatState.terminalTurns[turnId] = new Date().toISOString();
      }
      logStreamDebug(chatState, 'stream-done', {
        status: String(payload?.status || type).trim()
      });
      finalizeStreamTurn(chatState, payload, conversationID);
      chatState.lastHasRunning = false;
      chatState.activeTurnAgentId = '';
      chatState.activeTurnAgentName = '';
      if (String(payload?.turnId || '').trim()) {
        if (String(chatState.runningTurnId || '').trim() === completedTurnID) {
          chatState.runningTurnId = '';
        }
      } else {
        chatState.runningTurnId = '';
      }
      // Clear the conversation running state directly — previously this was
      // done by syncTranscriptSnapshot during transcript refresh, but since
      // streaming events are now the sole source of truth for active turns,
      // we update the form data here.
      const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
      if (conversationsDS) {
        const convForm = conversationsDS.peekFormData?.() || {};
        conversationsDS.setFormData?.({
          values: {
            ...convForm,
            running: false
          }
        });
      }
      if (type === 'turn_failed') {
        setStage({ phase: 'error', text: String(payload?.error || 'Turn failed') });
      } else if (type === 'turn_canceled') {
        setStage({ phase: 'done', text: 'Canceled' });
      } else {
        setStage({ phase: 'done', text: 'Done' });
      }
      // Don't clear feeds on turn end — they persist until a tool_feed_inactive
      // SSE event arrives (e.g., after revert/commit removes the feed's data).
      window.setTimeout(() => setStage({ phase: 'ready', text: 'Ready' }), 1100);
      renderMergedRowsForContext(context);
      if (resolvedConversationID && resolvedConversationID === String(getCurrentConversationID(context) || '').trim()) {
        queuePostTurnConversationRefresh(context, resolvedConversationID, completedTurnID);
      }
      return;
    }

    if (type === 'assistant_preamble') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      const preamblePayload = enrichPayloadWithTurnAgent(chatState, context, payload);
      logStreamDebug(chatState, 'stream-assistant-preamble', {
        turnId: String(preamblePayload?.turnId || '').trim(),
        assistantMessageId: String(preamblePayload?.assistantMessageId || '').trim(),
        preambleLen: String(preamblePayload?.content || '').length,
        agentIdUsed: String(preamblePayload?.agentIdUsed || '').trim()
      });
      applyPreambleEvent(chatState, preamblePayload, conversationID);
      setStage({ phase: 'streaming', text: 'Assistant thinking…' });
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'assistant_final') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      // assistant_final carries the final response content + finalResponse: true.
      // Use applyAssistantFinalEvent (not applyExecutionStreamEvent) to update
      // the existing row's content without creating a new execution group —
      // assistant_final's assistantMessageId may differ from model_started's.
      applyAssistantFinalEvent(chatState, enrichPayloadWithTurnAgent(chatState, context, payload));
      setStage({ phase: 'executing', text: 'Assistant responding…' });
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'turn_started') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      rememberTurnAgent(chatState, context, payload);
      const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
      if (conversationsDS) {
        const convForm = conversationsDS.peekFormData?.() || {};
        conversationsDS.setFormData?.({
          values: {
            ...convForm,
            running: true
          }
        });
      }
      const turnId = String(payload?.turnId || '').trim();
      if (turnId) {
        delete chatState.terminalTurns[turnId];
        chatState.activeStreamTurnId = turnId;
        chatState.runningTurnId = turnId;
        markLiveOwnedTurn(chatState, conversationID, turnId);
        applyTurnStartedEvent(chatState, enrichPayloadWithTurnAgent(chatState, context, payload), conversationID);
      }
      if (!chatState.activeStreamStartedAt) {
        chatState.activeStreamStartedAt = Date.now();
      }
      logStreamDebug(chatState, 'stream-turn-started', {
        turnId,
        agentIdUsed: String(payload?.agentIdUsed || '').trim(),
        agentName: String(chatState?.activeTurnAgentName || '').trim()
      });
      setStage({ phase: 'executing', text: 'Assistant executing…' });
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'elicitation_requested') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      logStreamDebug(chatState, 'stream-elicitation-requested', {
        turnId: String(payload?.turnId || '').trim(),
        elicitationId: String(payload?.elicitationId || '').trim(),
        assistantMessageId: String(payload?.assistantMessageId || '').trim(),
        hasElicitationData: !!payload?.elicitationData
      });
      applyElicitationRequestedEvent(chatState, payload);

      // Store in the elicitation bus for overlay rendering (independent of row pipeline).
      const elicitationData = payload?.elicitationData && typeof payload.elicitationData === 'object'
        ? payload.elicitationData : null;
      const requestedSchema = elicitationData?.requestedSchema
        || elicitationData?.schema
        || elicitationData
        || null;
      const elicitationId = String(payload?.elicitationId || '').trim();
      if (isStreamDebugEnabled()) {
        console.log('[elicitation-overlay-debug]', {
          elicitationId,
          hasElicitationData: !!elicitationData,
          elicitationDataKeys: elicitationData ? Object.keys(elicitationData) : [],
          hasRequestedSchema: !!requestedSchema,
          requestedSchemaType: requestedSchema ? typeof requestedSchema : 'none',
          requestedSchemaKeys: requestedSchema && typeof requestedSchema === 'object' ? Object.keys(requestedSchema) : [],
          content: String(payload?.content || '').slice(0, 100),
          rawElicitationData: JSON.stringify(elicitationData).slice(0, 300)
        });
      }
      if (elicitationId) {
        const elicUrl = String(elicitationData?.url || elicitationData?.Url || '').trim();
        const elicMode = String(elicitationData?.mode || elicitationData?.Mode || '').trim();
        setPendingElicitation({
          elicitationId,
          conversationId: String(payload?.conversationId || payload?.streamId || conversationID || '').trim(),
          turnId: String(payload?.turnId || '').trim(),
          message: String(payload?.content || '').trim(),
          requestedSchema,
          callbackURL: String(payload?.callbackUrl || '').trim(),
          url: elicUrl,
          mode: elicMode
        });
      }

      setStage({ phase: 'waiting', text: 'Waiting for input…' });
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'elicitation_resolved') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      clearPendingElicitation();
      setStage({ phase: 'executing', text: 'Resuming…' });
      return;
    }

    if (type === 'linked_conversation_attached') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      logStreamDebug(chatState, 'stream-linked-conversation-attached', {
        turnId: String(payload?.turnId || '').trim(),
        toolCallId: String(payload?.toolCallId || '').trim(),
        linkedConversationId: String(payload?.linkedConversationId || '').trim()
      });
      applyLinkedConversationEvent(chatState, payload);
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'usage' || type === 'item_completed') {
      // Metadata events — no UI action needed
      return;
    }

    if (type === 'tool_feed_active' || type === 'tool_feed_inactive') {
      chatState.lastStreamEventAt = Date.now();
      applyFeedEvent(payload);
      return;
    }

    if (type === 'error') {
      logStreamDebug(chatState, 'stream-error-event', {
        error: String(payload?.error || 'stream error')
      });
      setStage({ phase: 'error', text: String(payload?.error || 'Stream error') });
      const messages = context?.Context?.('messages')?.handlers?.dataSource;
      messages?.setError?.(payload?.error || 'stream error');
    }
}

export function disconnectStream(context) {
  const chatState = ensureContextResources(context);
  if (chatState.stream) {
    logStreamDebug(chatState, 'stream-close-manual', {
      conversationId: String(chatState.activeConversationID || '').trim()
    });
    chatState.stream.close();
    chatState.stream = null;
  }
}

export function shouldUseLiveStream(context, conversationID = '') {
  const chatState = ensureContextResources(context);
  const targetID = String(conversationID || '').trim();
  if (!targetID) return false;
  const currentConversationID = String(getCurrentConversationID(context) || '').trim();
  if (currentConversationID && currentConversationID === targetID) {
    return true;
  }
  const ownedConversationID = String(chatState.liveOwnedConversationID || '').trim();
  if (!ownedConversationID || ownedConversationID !== targetID) return false;
  return true;
}

export function syncConversationTransport(context, conversationID = '') {
  const targetID = String(conversationID || '').trim();
  if (!targetID) {
    disconnectStream(context);
    return false;
  }
  if (shouldUseLiveStream(context, targetID)) {
    connectStream(context, targetID);
    return true;
  }
  disconnectStream(context);
  return false;
}

export async function ensureConversation(context, options = {}) {
  const chatState = ensureContextResources(context);
  if (chatState.pendingConversationPromise) {
    return await chatState.pendingConversationPromise;
  }
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  if (!conversationsDS) return '';
  const form = conversationsDS.peekFormData?.() || {};
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  const metaForm = metaDS?.peekFormData?.() || {};
  const explicitNewConversation = !!chatState.explicitNewConversationRequested;
  const recoveredExistingID = String(
    form?.id
    || (!explicitNewConversation ? chatState?.activeConversationID : '')
    || (!explicitNewConversation ? getScopedConversationSelection(getContextWindowId(context)) : '')
    || (!explicitNewConversation ? conversationIDFromPath(typeof window !== 'undefined' ? window.location?.pathname : '') : '')
    || ''
  ).trim();
  if (recoveredExistingID) {
    const existingID = recoveredExistingID;
    try {
      const existing = await fetchConversation(existingID);
      if (existing) {
        if (String(form?.id || '').trim() !== existingID) {
          conversationsDS.setFormData?.({
            values: {
              ...form,
              id: existingID,
              title: existing?.title || existing?.Title || form?.title || 'New conversation'
            }
          });
        }
        publishActiveConversation(existingID, context);
        chatState.explicitNewConversationRequested = false;
        return existingID;
      }
    } catch (_) {
      // Fall through to fresh conversation creation when the selected id is stale.
    }
    conversationsDS.setFormData?.({
      values: draftConversationValues(form, metaForm?.defaults || {})
    });
    resetConversationSnapshotState(context);
  }
  const preferredAgent = sanitizeAutoSelection(options?.agent || '');
  const preferredModel = sanitizeAutoSelection(options?.model || '');
  const persistedAgent = resolveVisibleSelectedAgent(metaForm, getPersistedSelectedAgent());
  const agentID = resolveVisibleSelectedAgent(
    metaForm,
    preferredAgent,
    persistedAgent,
    form.agent,
    metaForm?.agent,
    metaForm?.defaults?.agent
  );
  const createPromise = (async () => {
    const created = await client.createConversation({ agentId: agentID });
    const id = String(created?.id || created?.Id || '').trim();
    if (!id) throw new Error('conversation create returned empty id');

    conversationsDS.setFormData?.({
      values: {
        ...form,
        id,
        title: created?.title || 'New chat',
        agent: agentID,
        model: preferredModel || form.model || ''
      }
    });
    publishActiveConversation(id, context);
    chatState.activeConversationID = id;
    chatState.explicitNewConversationRequested = false;
    // Notify sidebar to refresh the conversation list immediately.
    try {
      window.dispatchEvent(new CustomEvent('agently:conversation-new', { detail: { id } }));
    } catch (_) {}
    return id;
  })();
  chatState.pendingConversationPromise = createPromise;
  try {
    return await createPromise;
  } finally {
    if (chatState.pendingConversationPromise === createPromise) {
      chatState.pendingConversationPromise = null;
    }
  }
}

export async function switchConversation(context, conversationID = '') {
  const targetID = String(conversationID || '').trim();
  if (!targetID) return;
  clearFeedState();
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const messagesDS = context?.Context?.('messages')?.handlers?.dataSource;
  if (!conversationsDS || !messagesDS) return;

  const form = conversationsDS.peekFormData?.() || {};
  const currentID = String(form?.id || '').trim();
  const existing = await fetchConversation(targetID);
  if (!existing) {
    await createNewConversation(context);
    return;
  }
  if (currentID === targetID) {
    conversationsDS.setFormData?.({
      values: applyConversationFormSnapshot(form, existing)
    });
    resetConversationSnapshotState(context);
    syncConversationTransport(context, targetID);
    await dsTick(context, { conversationID: targetID });
    publishActiveConversation(targetID, context);
    return;
  }

  conversationsDS.setFormData?.({
    values: applyConversationFormSnapshot(form, existing)
  });
  messagesDS.setCollection?.([]);
  messagesDS.setError?.('');
  resetConversationSnapshotState(context);
  syncConversationTransport(context, targetID);
  await dsTick(context, { conversationID: targetID });
  publishActiveConversation(targetID, context);
}

export function enqueueConversationSwitch(context, conversationID = '') {
  const chatState = ensureContextResources(context);
  const targetID = String(conversationID || '').trim();
  if (!targetID) return Promise.resolve();
  const queue = chatState.switchQueue || Promise.resolve();
  chatState.switchQueue = queue
    .catch(() => {})
    .then(() => switchConversation(context, targetID));
  return chatState.switchQueue;
}

export function applyIterationVisibility(context) {
  const chatState = ensureContextResources(context);
  const messagesDS = context?.Context?.('messages')?.handlers?.dataSource;
  if (!messagesDS) return false;
  const rows = Array.isArray(chatState.renderRows) ? chatState.renderRows : [];
  if (rows.length === 0) return false;
  messagesDS.setCollection?.(rows);
  return true;
}

export function bootstrapConversationSelection(context) {
  const windowId = getContextWindowId(context);
  const win = getWindowById(windowId);
  const bootstrapID = typeof window !== 'undefined'
    ? (
      String(win?.parameters?.conversations?.form?.id || '').trim()
      || String(win?.parameters?.conversations?.input?.parameters?.id || '').trim()
      || String(win?.parameters?.conversations?.input?.path?.id || '').trim()
      || String(win?.parameters?.conversations?.input?.id || '').trim()
      || String(win?.parameters?.conversationId || '').trim()
      || String(win?.parameters?.messages?.input?.parameters?.convID || '').trim()
      || String(win?.parameters?.messages?.input?.path?.convID || '').trim()
      || String(win?.parameters?.messages?.input?.convID || '').trim()
      || (isMainChatWindowId(windowId) ? conversationIDFromPath(window.location.pathname) : '')
      || getScopedConversationSelection(windowId)
    )
    : '';
  if (!bootstrapID) return;
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const current = conversationsDS?.peekFormData?.() || {};
  conversationsDS?.setFormData?.({ values: { ...current, id: bootstrapID } });
}

export function bindConversationWindowEvents(context) {
  const chatState = ensureContextResources(context);
  if (typeof window === 'undefined' || chatState.boundConversationEvents) return;
  const currentWindowId = getContextWindowId(context);
  chatState.onConversationSelect = (event) => {
    const targetWindowId = String(event?.detail?.windowId || '').trim();
    if (targetWindowId && targetWindowId !== currentWindowId) return;
    const id = String(event?.detail?.id || '').trim();
    if (!id) return;
    void enqueueConversationSwitch(context, id);
  };
  chatState.onNewConversation = (event) => {
    const targetWindowId = String(event?.detail?.windowId || '').trim();
    if (targetWindowId && targetWindowId !== currentWindowId) return;
    void createNewConversation(context);
  };
  window.addEventListener('agently:conversation-select', chatState.onConversationSelect);
  window.addEventListener('agently:conversation-new', chatState.onNewConversation);
  chatState.boundConversationEvents = true;
}

export function unbindConversationWindowEvents(context) {
  const chatState = ensureContextResources(context);
  if (typeof window !== 'undefined') {
    if (chatState.onConversationSelect) {
      window.removeEventListener('agently:conversation-select', chatState.onConversationSelect);
    }
    if (chatState.onNewConversation) {
      window.removeEventListener('agently:conversation-new', chatState.onNewConversation);
    }
  }
  chatState.boundConversationEvents = false;
  chatState.onConversationSelect = null;
  chatState.onNewConversation = null;
}

export async function createNewConversation(context) {
  clearFeedState();
  const chatState = ensureContextResources(context);
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  if (!conversationsDS) return false;
  if (chatState.pendingConversationPromise) {
    try {
      await chatState.pendingConversationPromise;
    } catch (_) {
      // best effort: continue with local draft reset
    }
  }
  if (chatState.stream) {
    chatState.stream.close();
    chatState.stream = null;
  }
  chatState.activeConversationID = '';
  chatState.lastConversationID = '';
  chatState.explicitNewConversationRequested = true;
  const current = conversationsDS.peekFormData?.() || {};
  const metaForm = context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.() || {};
  const metaDefaults = metaForm?.defaults || {};
  const persistedAgent = resolveVisibleSelectedAgent(metaForm, getPersistedSelectedAgent());
  // Merge the user's current agent/model selection from meta into current
  // so draftConversationValues preserves it for the new conversation.
  const merged = { ...current };
  if (persistedAgent) {
    merged.agent = persistedAgent;
  } else if (isVisibleAgent(metaForm, metaForm?.agent)) {
    merged.agent = metaForm.agent;
  }
  if (metaForm?.model) merged.model = metaForm.model;
  conversationsDS.setFormData?.({
    values: draftConversationValues(merged, metaDefaults)
  });
  if (persistedAgent && context?.Context?.('meta')?.handlers?.dataSource) {
    context.Context('meta').handlers.dataSource.setFormData?.({
      values: {
        ...metaForm,
        agent: persistedAgent
      }
    });
  }
  const messagesDS = context?.Context?.('messages')?.handlers?.dataSource;
  messagesDS?.setCollection?.([]);
  messagesDS?.setError?.('');
  resetConversationSnapshotState(context);
  renderMergedRowsForContext(context);
  setStage({ phase: 'ready', text: 'Ready' });
  publishActiveConversation('', context);
  return true;
}

export function startPolling(context) {
  const chatState = ensureContextResources(context);
  const windowId = getContextWindowId(context);
  logExecutorDebug('polling-start', {
    windowId,
    conversationId: getCurrentConversationID(context)
  });
  if (chatState.timer) {
    clearInterval(chatState.timer);
    chatState.timer = null;
  }
  chatState.timer = setInterval(() => {
    const desiredID = typeof window !== 'undefined'
      ? (
        getScopedConversationSelection(windowId)
        || (isMainChatWindowId(windowId) ? conversationIDFromPath(window.location.pathname) : '')
      )
      : '';
    const currentID = getCurrentConversationID(context);
    if (desiredID && desiredID !== currentID) {
      void enqueueConversationSwitch(context, desiredID);
      return;
    }
    const streamIsHot = !!chatState.stream
      && (Date.now() - Number(chatState.lastStreamEventAt || 0) < 6000);
    if (streamIsHot) return;
    if (shouldDeferTranscriptToLiveStream(context, getCurrentConversationID(context))) return;
    void dsTick(context);
  }, 4000);
}

export function stopPolling(context) {
  const chatState = ensureContextResources(context);
  logExecutorDebug('polling-stop', {
    conversationId: getCurrentConversationID(context),
    hadTimer: !!chatState.timer,
    hadRefreshTimer: !!chatState.refreshTimer,
    hadStream: !!chatState.stream
  });
  if (chatState.timer) {
    clearInterval(chatState.timer);
    chatState.timer = null;
  }
  if (chatState.refreshTimer) {
    clearTimeout(chatState.refreshTimer);
    chatState.refreshTimer = null;
  }
  if (chatState.postTurnRefreshTimer) {
    clearTimeout(chatState.postTurnRefreshTimer);
    chatState.postTurnRefreshTimer = null;
  }
  if (chatState.stream) {
    chatState.stream.close();
    chatState.stream = null;
  }
}

export function rememberSeedTitle(conversationID, query) {
  rememberConversationSeedTitle(conversationID, query);
}
