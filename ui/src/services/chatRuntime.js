import { normalizeMessages, normalizeOne } from './messageNormalizer';
import { compareTemporalEntries, isLiveConversationState } from 'agently-core-ui-sdk';

// App bootstrap installs the canonical chatStore mirror explicitly.
// chatRuntime never resolves the store through module-system fallbacks.
let _chatStoreModule = null;
function _chatStoreRef() {
  return _chatStoreModule;
}

/**
 * Install the chatStore module as the forwarding target for SSE events and
 * transcript fetches. Call once at app bootstrap in environments where
 * `require` is not available (pure ESM).
 */
export function installChatStoreMirror(chatStoreModule) {
  _chatStoreModule = chatStoreModule || null;
}
import { ConversationStreamTracker, hasLiveAssistantRowForTurn, latestEffectiveLiveAssistantRow } from 'agently-core-ui-sdk/internal';
import { rememberConversationSeedTitle } from './conversationTitle';
import { setPendingElicitation, clearPendingElicitation } from './elicitationBus';
import { applyFeedEvent, clearFeedState, isFeedInactive } from './toolFeedBus';
import { publishUsage } from './usageBus';
import { request } from './httpClient';
import {
  queueTranscriptRefresh as queueTranscriptRefreshStore,
  resetTranscriptState,
  syncTranscriptSnapshot as syncTranscriptSnapshotStore,
  tickTranscript
} from './transcriptStore';
import {
  applyAssistantMessageAddEvent,
  applyElicitationRequestedEvent,
  applyExecutionStreamEvent,
  applyMessagePatchEvent,
  applyPreambleEvent,
  applyStreamChunk,
  applyTurnStartedEvent,
  applyToolStreamEvent,
  finalizeStreamTurn,
  markLiveOwnedTurn,
  resetLiveStreamState
} from './liveStreamStore';
import { buildCanonicalTranscriptRows, buildConversationRenderRows, isCanonicalTranscriptTurn } from './renderRows';
import {
  getWindowById,
  MAIN_CHAT_WINDOW_ID,
  getScopedConversationSelection,
  isMainChatWindowId,
  publishConversationSelection
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
import { isExecutorDebugEnabled, isStreamDebugEnabled } from './debugFlags';

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

function normalizeGeneratedFiles(raw = null) {
  const list = Array.isArray(raw)
    ? raw
    : Array.isArray(raw?.data)
      ? raw.data
      : [];
  return list.map((item) => {
    const id = String(item?.id || item?.ID || '').trim();
    const conversationId = String(item?.conversationId || item?.ConversationID || item?.ConversationId || '').trim();
    const turnId = String(item?.turnId || item?.TurnID || item?.TurnId || '').trim();
    const messageId = String(item?.messageId || item?.MessageID || item?.MessageId || '').trim();
    const filename = String(
      item?.filename
      || item?.Filename
      || item?.providerFileId
      || item?.ProviderFileID
      || id
      || 'generated-file.bin'
    ).trim();
    const status = String(item?.status || item?.Status || '').trim();
    const mode = String(item?.mode || item?.Mode || '').trim();
    const mimeType = String(item?.mimeType || item?.MimeType || '').trim();
    const sizeBytesRaw = item?.sizeBytes ?? item?.SizeBytes;
    const sizeBytes = Number.isFinite(Number(sizeBytesRaw)) ? Number(sizeBytesRaw) : undefined;
    return {
      id,
      conversationId,
      turnId,
      messageId,
      filename,
      status,
      mode,
      mimeType,
      sizeBytes
    };
  }).filter((item) => !!item.id);
}

function mergeGeneratedFileLists(...lists) {
  const out = [];
  const seen = new Set();
  for (const list of lists) {
    if (!Array.isArray(list) || list.length === 0) continue;
    for (const item of list) {
      const id = String(item?.id || '').trim();
      if (!id || seen.has(id)) continue;
      seen.add(id);
      out.push(item);
    }
  }
  return out;
}

async function fetchGeneratedFiles(conversationID = '') {
  const id = String(conversationID || '').trim();
  if (!id) return [];
  try {
    const payload = await request(`/api/conversations/${encodeURIComponent(id)}/generated-files`, {
      method: 'GET',
      notify: false
    });
    return normalizeGeneratedFiles(payload);
  } catch (_) {
    return [];
  }
}

async function refreshGeneratedFiles(context, conversationID = '') {
  const id = String(conversationID || getCurrentConversationID(context) || '').trim();
  const chatState = ensureContextResources(context);
  if (!id) {
    chatState.generatedFiles = [];
    return [];
  }
  const files = await fetchGeneratedFiles(id);
  chatState.generatedFiles = files;
  return files;
}

function attachGeneratedFilesToRows(rows = [], files = []) {
  const generatedFiles = Array.isArray(files) ? files : [];
  if (generatedFiles.length === 0) {
    return Array.isArray(rows) ? rows : [];
  }
  return (Array.isArray(rows) ? rows : []).map((row) => {
    const rowId = String(row?.id || '').trim();
    const turnId = String(row?.turnId || '').trim();
    const role = String(row?.role || '').trim().toLowerCase();
    const scopedFiles = generatedFiles.filter((file) => {
      const fileMessageId = String(file?.messageId || '').trim();
      const fileTurnId = String(file?.turnId || '').trim();
      if (fileMessageId && rowId && fileMessageId === rowId) return true;
      if (role !== 'assistant') return false;
      return !!fileTurnId && !!turnId && fileTurnId === turnId;
    });
    if (scopedFiles.length === 0 && !Array.isArray(row?.generatedFiles)) {
      return row;
    }
    return {
      ...row,
      generatedFiles: mergeGeneratedFileLists(row?.generatedFiles, scopedFiles)
    };
  });
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

function transcriptShouldBeIdle(chatState = {}, conversationID = '') {
  const targetID = String(conversationID || '').trim();
  if (!targetID) return false;
  if (String(chatState?.liveOwnedConversationID || '').trim() !== targetID) return false;
  const ownedTurnIds = Array.isArray(chatState?.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [];
  if (ownedTurnIds.length === 0) return false;
  return !!(
    trackerActiveTurnId(chatState)
    || String(chatState?.runningTurnId || '').trim()
    || String(chatState?.activeStreamTurnId || '').trim()
    || chatState?.lastHasRunning
  );
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

function publishConversationMetaUpdated(conversationID = '', patch = {}) {
  if (typeof window === 'undefined') return;
  const id = String(conversationID || '').trim();
  if (!id) return;
  try {
    window.dispatchEvent(new CustomEvent('agently:conversation-meta-updated', {
      detail: { id, patch: patch || {} }
    }));
  } catch (_) {}
}

export function isConversationLiveish(conversation = null) {
  return isLiveConversationState(conversation);
}

function updateConversationLiveState(context, patch = {}) {
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  if (!conversationsDS) return;
  const current = conversationsDS.peekFormData?.() || {};
  const next = { ...current };
  if (patch.running != null) next.running = !!patch.running;
  if (patch.stage != null) next.stage = String(patch.stage || '').trim();
  if (patch.status != null) next.status = String(patch.status || '').trim();
  conversationsDS.setFormData?.({ values: next });
}

function liveStatusValue(payload = {}, fallback = 'running') {
  return String(payload?.status || fallback).trim() || fallback;
}

function terminalStageForEvent(type = '') {
  const normalized = String(type || '').trim().toLowerCase();
  if (normalized === 'turn_failed') return 'error';
  if (normalized === 'turn_canceled') return 'canceled';
  return 'done';
}

function applyStreamConversationState(context, phase, payload = {}) {
  switch (String(phase || '').trim().toLowerCase()) {
    case 'thinking':
      updateConversationLiveState(context, {
        running: true,
        stage: 'thinking',
        status: liveStatusValue(payload, 'running')
      });
      return;
    case 'executing':
      updateConversationLiveState(context, {
        running: true,
        stage: 'executing',
        status: liveStatusValue(payload, 'running')
      });
      return;
    case 'eliciting':
      updateConversationLiveState(context, {
        running: true,
        stage: 'eliciting',
        status: liveStatusValue(payload, 'pending')
      });
      return;
    case 'terminal':
      updateConversationLiveState(context, {
        running: false,
        stage: terminalStageForEvent(payload?.type),
        status: String(payload?.status || payload?.type || '').trim()
      });
      return;
    default:
      return;
  }
}

export function resolveStreamEventConversationID(payload = {}, subscribedConversationID = '') {
  return String(payload?.conversationId || payload?.streamId || '').trim();
}

export function shouldProcessStreamEvent({ payload = {}, subscribedConversationID = '', visibleConversationID = '', switchingConversationID = '' } = {}) {
  const eventConversationID = resolveStreamEventConversationID(payload, subscribedConversationID);
  const visibleID = String(visibleConversationID || '').trim();
  const switchingID = String(switchingConversationID || '').trim();
  if (!eventConversationID) return false;
  if (switchingID) return eventConversationID === switchingID;
  if (!visibleID) return true;
  return eventConversationID === visibleID;
}

function streamEventMode(payload = {}) {
  return String(payload?.mode || payload?.patch?.mode || '').trim().toLowerCase();
}

function stageStartedAtValue(payload = {}, chatState = {}) {
  return String(payload?.startedAt || payload?.createdAt || '').trim()
    || Number(chatState?.activeStreamStartedAt || 0)
    || 0;
}

function stageCompletedAtValue(payload = {}) {
  return String(payload?.completedAt || payload?.createdAt || '').trim() || 0;
}

function shouldIgnoreExecutionStreamEvent(payload = {}) {
  const mode = streamEventMode(payload);
  const phase = String(payload?.phase || '').trim().toLowerCase();
  const type = String(payload?.type || '').trim().toLowerCase();
  if (mode === 'summary' || phase === 'summary') return true;
  const isStreamingContentEvent = type === 'text_delta' || type === 'narration';
  if (!isStreamingContentEvent) return false;
  return phase === 'intake';
}

function textDeltaQueueKey(payload = {}, fallbackConversationID = '') {
  return [
    String(payload?.conversationId || payload?.streamId || fallbackConversationID || '').trim(),
    String(payload?.turnId || '').trim(),
    String(payload?.messageId || payload?.assistantMessageId || payload?.id || '').trim(),
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
    if (!String(last.messageId || '').trim() && String(payload?.messageId || '').trim()) {
      last.messageId = String(payload.messageId).trim();
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

function clearPendingStreamReconnect(chatState = {}) {
  if (!chatState) return;
  if (chatState.pendingStreamReconnect != null) {
    try { clearTimeout(chatState.pendingStreamReconnect); } catch (_) {}
    chatState.pendingStreamReconnect = null;
  }
}

function scheduleStreamReconnect(context, conversationID = '', reason = '') {
  const chatState = ensureContextResources(context);
  const targetID = String(conversationID || '').trim();
  if (!targetID) return;
  if (chatState.pendingStreamReconnect != null) return;
  chatState.pendingStreamReconnect = window.setTimeout(() => {
    chatState.pendingStreamReconnect = null;
    if (!shouldUseLiveStream(context, targetID)) return;
    queueTranscriptRefresh(context, { delay: 0, force: true });
    connectStream(context, targetID);
  }, 1000);
  logStreamDebug(chatState, 'stream-reconnect-scheduled', {
    conversationId: targetID,
    reason: String(reason || '').trim()
  });
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
    || eventType === 'narration'
    || eventType === 'elicitation_requested'
    || eventType === 'linked_conversation_attached'
    || eventType === 'turn_started') {
    return true;
  }
  if (eventType !== 'control') return false;
  const op = String(payload?.op || '').trim().toLowerCase();
  return op === 'turn_started';
}

function draftConversationValues(current = {}, defaults = {}, preferredAgent = '') {
  const values = {
    ...current,
    id: '',
    title: 'New conversation',
    agent: preferredAgent || current?.agent || defaults?.agent || '',
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

function trackerActiveTurnId(chatState = {}) {
  return String(chatState?.streamTracker?.canonicalState?.activeTurnId || '').trim();
}

function syncTrackerDerivedTurnState(chatState = {}) {
  const trackerTurnId = trackerActiveTurnId(chatState);
  if (trackerTurnId) {
    chatState.runningTurnId = trackerTurnId;
    if (!String(chatState.activeStreamTurnId || '').trim()) {
      chatState.activeStreamTurnId = trackerTurnId;
    }
    chatState.lastHasRunning = true;
    return trackerTurnId;
  }
  if (!chatState.lastHasRunning) {
    chatState.runningTurnId = '';
    chatState.activeStreamTurnId = '';
  }
  return '';
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
  const selectedAgent = resolveVisibleSelectedAgent(
    { defaults, agentInfos: normalizedAgentInfos, agentOptions },
    data?.agent,
    defaults.agent
  );
  const starterTasks = resolveStarterTasks({
    agentInfos: normalizedAgentInfos,
    selectedAgent
  });
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
    starterTasks,
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

export function mapTranscriptToRows(turns = [], options = {}) {
  if (!Array.isArray(turns) || turns.length === 0) {
    return { rows: [], queuedTurns: [], runningTurnId: '' };
  }
  if (!isCanonicalTranscriptTurn(turns[0])) {
    if (isStreamDebugEnabled()) {
      console.warn('[transcript-render] non-canonical turns passed to mapTranscriptToRows');
    }
    return { rows: [], queuedTurns: [], runningTurnId: '' };
  }
  return buildCanonicalTranscriptRows(turns, options);
}

function findLatestRunningTurnIdFromTurns(turns = []) {
  const list = Array.isArray(turns) ? turns : [];
  for (let i = list.length - 1; i >= 0; i -= 1) {
    const turn = list[i];
    const status = String(turn?.status || turn?.Status || '').toLowerCase().trim();
    if (!isRunningStatus(status)) continue;
    const id = String(turn?.turnId || turn?.id || turn?.Id || '').trim();
    if (id) return id;
  }
  return '';
}

export function resolveLastTranscriptCursor(turns = []) {
  const list = Array.isArray(turns) ? turns : [];
  for (let turnIndex = list.length - 1; turnIndex >= 0; turnIndex -= 1) {
    const turn = list[turnIndex];
    if (isCanonicalTranscriptTurn(turn)) {
      const pages = Array.isArray(turn?.execution?.pages) ? turn.execution.pages : [];
      for (let pageIndex = pages.length - 1; pageIndex >= 0; pageIndex -= 1) {
        const page = pages[pageIndex] || {};
        const assistantMessageId = String(page?.assistantMessageId || '').trim();
        if (assistantMessageId) return assistantMessageId;
        const pageId = String(page?.pageId || '').trim();
        if (pageId) return pageId;
      }
      const assistantFinalId = String(turn?.assistant?.final?.messageId || '').trim();
      if (assistantFinalId) return assistantFinalId;
      const userId = String(turn?.user?.messageId || '').trim();
      if (userId) return userId;
      const turnId = String(turn?.turnId || '').trim();
      if (turnId) return turnId;
      continue;
    }
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
  context.resources.chat.generatedFiles = Array.isArray(context.resources.chat.generatedFiles) ? context.resources.chat.generatedFiles : [];
  if (!context.resources.chat.streamTracker) {
    context.resources.chat.streamTracker = new ConversationStreamTracker(String(context.resources.chat.activeConversationID || '').trim());
  }
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
      publishConversationActivity(targetConversationID, {
        type: 'turn_refreshed',
        turnId: targetTurnID,
        status: 'refreshed'
      });
    } catch (_) {
      // Best-effort: preserve the current live render if refresh fails.
    }
  }, 90);
}

function latestTurnStillOwnedByLive(chatState = {}, conversationID = '', turnID = '') {
  const targetConversationID = String(conversationID || '').trim();
  const targetTurnID = String(turnID || '').trim();
  if (!targetConversationID || !targetTurnID) return false;
  if (String(chatState?.liveOwnedConversationID || '').trim() !== targetConversationID) return false;
  const ownedTurnIds = Array.isArray(chatState?.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [];
  return ownedTurnIds.includes(targetTurnID);
}

function applyTerminalTurnStatusToTranscriptRows(rows = [], turnId = '', status = '', errorMessage = '') {
  const targetTurnId = String(turnId || '').trim();
  const terminalStatus = String(status || '').trim();
  if (!targetTurnId || !terminalStatus) return Array.isArray(rows) ? rows : [];
  return (Array.isArray(rows) ? rows : []).map((row) => {
    if (String(row?.turnId || '').trim() !== targetTurnId) return row;
    const next = {
      ...row,
      turnStatus: terminalStatus
    };
    if (String(row?.role || '').trim().toLowerCase() === 'assistant') {
      next.status = terminalStatus;
      if (errorMessage && !String(next.errorMessage || '').trim()) {
        next.errorMessage = errorMessage;
      }
    }
    return next;
  });
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
  const currentConversationID = getCurrentConversationID(context);
  const effectiveRunningTurnId = String(
    trackerActiveTurnId(chatState)
    || chatState.runningTurnId
    || ''
  ).trim();
  const { effectiveLiveRows, mergedRows } = buildConversationRenderRows({
    transcriptRows: chatState.transcriptRows,
    streamState: chatState?.streamTracker?.canonicalState,
    liveRows: chatState.liveRows,
    currentConversationID,
    runningTurnId: effectiveRunningTurnId,
    hasRunning: chatState.lastHasRunning,
    findLatestRunningTurnId,
    liveOwnedConversationID: chatState.liveOwnedConversationID,
    liveOwnedTurnIds: chatState.liveOwnedTurnIds
  });
  chatState.renderRows = mergedRows;
  const normalizedRows = normalizeForContext(context, mergedRows);
  if (typeof window !== 'undefined' && window.__agentlyActiveChatState === chatState) {
    window.__agentlyActiveChatState.renderRows = mergedRows;
    window.__agentlyActiveChatState.normalizedRows = normalizedRows;
  }
  if (isStreamDebugEnabled()) {
    console.log('[render]', {
      ts: new Date().toISOString(),
      conversationId: getCurrentConversationID(context),
      trackerActiveTurnId: trackerActiveTurnId(chatState),
      runningTurnId: chatState.runningTurnId,
      liveOwnedConversationID: chatState.liveOwnedConversationID,
      liveOwnedTurnIds: Array.isArray(chatState.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [],
      liveCount: effectiveLiveRows.length,
      mergedCount: mergedRows.length,
      normalizedCount: normalizedRows.length,
      normalizedTypes: normalizedRows.map((row) => ({
        id: row?.id,
        type: row?._type || row?.role,
        mode: row?.mode || '',
        head: String(row?.content || '').slice(0, 60),
        groups: (row?.executionGroups || []).map((g) => ({
          kind: g?.groupKind || '',
          title: g?.title || '',
          toolSteps: (g?.toolSteps || []).map((step) => ({
            toolName: step?.toolName || '',
            status: step?.status || '',
            contentHead: String(step?.content || '').slice(0, 80)
          }))
        }))
      })),
      liveRows: effectiveLiveRows.map((r) => ({
        id: r?.id, role: r?.role, turnId: r?.turnId, interim: r?.interim,
        contentHead: String(r?.content || '').slice(0, 50),
        groups: (r?.executionGroups || []).length,
        toolSteps: (r?.executionGroups || []).flatMap((g) => g?.toolSteps || []).length,
        groupKinds: (r?.executionGroups || []).map((g) => g?.groupKind || ''),
        requestPayloadIds: (r?.executionGroups || []).flatMap((g) => (g?.modelSteps || []).map((step) => step?.requestPayloadId || '')),
        providerRequestPayloadIds: (r?.executionGroups || []).flatMap((g) => (g?.modelSteps || []).map((step) => step?.providerRequestPayloadId || ''))
      })),
      rawRows: mergedRows.map((row) => ({
        id: row?.id,
        role: row?.role,
        turnId: row?.turnId,
        mode: row?.mode || '',
        head: String(row?.content || '').slice(0, 60),
        groups: (row?.executionGroups || []).map((g) => ({
          kind: g?.groupKind || '',
          title: g?.title || '',
          modelRequestPayloadId: g?.modelSteps?.[0]?.requestPayloadId || '',
          modelProviderRequestPayloadId: g?.modelSteps?.[0]?.providerRequestPayloadId || '',
          toolSteps: (g?.toolSteps || []).map((step) => ({
            toolName: step?.toolName || '',
            status: step?.status || '',
            contentHead: String(step?.content || '').slice(0, 80)
          }))
        }))
      }))
    });
  }
  // liveStreamStore already normalizes streaming content into row.content.
  // Do not overwrite it with raw _streamContent here, or markdown/chart fences
  // leak into the bubble during streaming instead of rendering as rich content.
  const resolvedRows = attachGeneratedFilesToRows(mergedRows, chatState.generatedFiles);
  const normalizedResolvedRows = normalizeForContext(context, resolvedRows);
  const queuedTurns = Array.isArray(chatState?.lastQueuedTurns) ? chatState.lastQueuedTurns : [];
  const queuedTurnIds = new Set(queuedTurns.map((item) => String(item?.id || '').trim()).filter(Boolean));
  const queuedTurnPreviews = new Set(
    queuedTurns
      .map((item) => String(item?.preview || item?.content || '').trim())
      .filter(Boolean)
  );
  const normalizedConversationID = String(conversationForm?.id || '').trim();
  if (normalizedConversationID) {
    if (queuedTurns.length > 0) {
      applyFeedEvent({
        type: 'tool_feed_active',
        feedId: 'queue',
        feedTitle: 'Queue',
        feedItemCount: queuedTurns.length,
        feedData: {
          output: {
            queuedTurns,
          },
        },
        conversationId: normalizedConversationID,
        localOnly: true,
      });
    } else {
      applyFeedEvent({
        type: 'tool_feed_inactive',
        feedId: 'queue',
        conversationId: normalizedConversationID,
      });
    }
  }
  const selectedAgent = resolveVisibleSelectedAgent(
    metaForm,
    conversationForm?.agent,
    getPersistedSelectedAgent(),
    metaForm?.agent,
    metaForm?.defaults?.agent
  );
  const hasConversationId = String(conversationForm?.id || '').trim() !== '';
  const filteredResolvedRows = normalizedResolvedRows.filter((row) => {
    if (String(row?.role || '').trim().toLowerCase() !== 'user') return true;
    const turnId = String(row?.turnId || '').trim();
    if (turnId && queuedTurnIds.has(turnId)) return false;
    const content = String(row?.content || '').trim();
    if (content && queuedTurnPreviews.has(content)) return false;
    return true;
  });
  const hasVisibleConversationContent = filteredResolvedRows.some((row) => {
    const type = String(row?._type || '').toLowerCase();
    return type !== 'queue';
  });
  messagesDS.setCollection?.(filteredResolvedRows);
  return mergedRows;
}

function trackerHasAssistantRowForTurn(chatState = {}, conversationID = '', turnID = '') {
  return hasLiveAssistantRowForTurn(chatState?.streamTracker?.canonicalState, conversationID, turnID);
}

export function latestAssistantRowForTurn(chatState = {}, conversationID = '', turnID = '') {
  const targetTurnID = String(turnID || '').trim();
  if (!targetTurnID) return null;
  const targetConversationID = String(conversationID || chatState?.activeConversationID || '').trim();
  return latestEffectiveLiveAssistantRow(
    chatState?.streamTracker?.canonicalState,
    Array.isArray(chatState?.liveRows) ? chatState.liveRows : [],
    targetConversationID,
    targetTurnID
  );
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
  const explicit = String(form?.id || '').trim();
  if (explicit) return explicit;
  const chatState = context?.resources?.chat || {};
  const active = String(chatState?.activeConversationID || '').trim();
  if (active) return active;
  return '';
}

function getContextWindowId(context) {
  return String(context?.identity?.windowId || '').trim() || MAIN_CHAT_WINDOW_ID;
}

export function publishActiveConversation(conversationID = '', context = null) {
  const id = String(conversationID || '').trim();
  const windowId = getContextWindowId(context);
  publishConversationSelection(windowId, id, {
    syncPath: isMainChatWindowId(windowId),
    eventType: 'forge:conversation-active'
  });
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
  const activeChatState = typeof window !== 'undefined' ? window.__agentlyActiveChatState : null;
  const latestTurnLiveOwned = transcriptShouldBeIdle(activeChatState, conversationID);
  if (typeof window !== 'undefined') {
    const chatState = activeChatState;
    if (latestTurnLiveOwned) {
      logExecutorDebug('transcript-fetch-while-live-owned', {
        conversationId: conversationID,
        since,
        liveOwnedConversationID: String(chatState?.liveOwnedConversationID || '').trim(),
        liveOwnedTurnIds: Array.isArray(chatState?.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [],
        runningTurnId: String(chatState?.runningTurnId || '').trim(),
        activeStreamTurnId: String(chatState?.activeStreamTurnId || '').trim(),
        trackerActiveTurnId: trackerActiveTurnId(chatState),
        lastHasRunning: !!chatState?.lastHasRunning
      });
    }
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
  const canonicalConversation = data?.conversation && typeof data.conversation === 'object'
    ? data.conversation
    : null;
  const canonicalTurns = Array.isArray(canonicalConversation?.turns) ? canonicalConversation.turns : null;
  const liveOwnedTurnIds = Array.isArray(activeChatState?.liveOwnedTurnIds)
    ? activeChatState.liveOwnedTurnIds.map((value) => String(value || '').trim()).filter(Boolean)
    : [];
  // ── chatStore transcript forwarding shim (PR-0 side-consumer cutover) ───
  // Forward the raw canonical ConversationState to the new client store so
  // ChatFeedFromChatStore (and any downstream consumer) sees identical data
  // without waiting for full chatRuntime migration. When the latest turn is
  // live-owned, transcript must not reshape the active canonical feed; the
  // runtime already treats transcript as idle in that state.
  try {
    if (canonicalConversation && canonicalConversation.conversationId) {
      const store = _chatStoreRef();
      if (store) {
        const forwardedConversation = latestTurnLiveOwned
          ? filterCanonicalConversationForLiveOwnedTurns(canonicalConversation, liveOwnedTurnIds)
          : canonicalConversation;
        store.onTranscript(canonicalConversation.conversationId, forwardedConversation);
      }
    }
  } catch (_) { /* best-effort mirror */ }
  const resolvedFeeds = Array.isArray(data?.feeds)
    ? data.feeds
    : (Array.isArray(canonicalConversation?.feeds) ? canonicalConversation.feeds : []);
  // Populate tool feed bar from transcript feeds (for page reload / conversation switch).
  if (!latestTurnLiveOwned && Array.isArray(resolvedFeeds) && resolvedFeeds.length > 0) {
    for (const feed of resolvedFeeds) {
      if (feed?.feedId && !isFeedInactive(feed.feedId, conversationID)) {
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
  if (!latestTurnLiveOwned && activeChatState && conversationID) {
    const current = activeChatState.lastTranscriptFeedsByConversation || {};
    activeChatState.lastTranscriptFeedsByConversation = {
      ...current,
      [conversationID]: Array.isArray(resolvedFeeds) ? resolvedFeeds : []
    };
  }
  if (Array.isArray(canonicalTurns) && canonicalTurns.length > 0 && isCanonicalTranscriptTurn(canonicalTurns[0])) {
    return canonicalTurns;
  }
  return Array.isArray(data?.conversation?.turns) ? data.conversation.turns : [];
}

export function filterCanonicalConversationForLiveOwnedTurns(conversation = {}, liveOwnedTurnIds = []) {
  const owned = new Set((Array.isArray(liveOwnedTurnIds) ? liveOwnedTurnIds : []).map((item) => String(item || '').trim()).filter(Boolean));
  if (owned.size === 0) return conversation;
  const turns = Array.isArray(conversation?.turns) ? conversation.turns : [];
  return {
    ...conversation,
    turns: turns.map((turn) => {
      const turnId = String(turn?.turnId || '').trim();
      if (!turnId || !owned.has(turnId)) return turn;
      return {
        ...turn,
        execution: null,
        assistant: null,
        elicitation: null,
      };
    }),
  };
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
  const stage = String(conversation?.stage || conversation?.Stage || '').trim();
  const status = String(conversation?.status || conversation?.Status || '').trim();
  const agent = String(conversation?.agentId || conversation?.AgentId || '').trim();
  const model = String(conversation?.defaultModel || conversation?.DefaultModel || '').trim();
  const embedder = String(conversation?.defaultEmbedder || conversation?.DefaultEmbedder || '').trim();
  if (conversationID) next.id = conversationID;
  if (title) next.title = title;
  if (summary) next.summary = summary;
  if (stage) next.stage = stage;
  if (status) next.status = status;
  next.running = isConversationLiveish(conversation);
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
  const currentConversationID = String(getCurrentConversationID(context) || '').trim();
  if (transcriptShouldBeIdle(chatState, currentConversationID)) {
    logExecutorDebug('transcript-snapshot-skipped-live-owned', {
      conversationId: currentConversationID,
      reason,
      liveOwnedConversationID: String(chatState?.liveOwnedConversationID || '').trim(),
      liveOwnedTurnIds: Array.isArray(chatState?.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [],
      runningTurnId: String(chatState?.runningTurnId || '').trim(),
      activeStreamTurnId: String(chatState?.activeStreamTurnId || '').trim(),
      trackerActiveTurnId: trackerActiveTurnId(chatState),
      lastHasRunning: !!chatState?.lastHasRunning
    });
    return renderMergedRowsForContext(context);
  }
  if (chatState.streamTracker && Array.isArray(turns)) {
    chatState.streamTracker.applyTranscript(turns);
    syncTrackerDerivedTurnState(chatState);
  }
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
    applyFeedEvent,
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
  const hasPendingLiveTurnBootstrap = String(chatState.activeStreamPrompt || '').trim() !== ''
    && String(chatState.liveOwnedConversationID || '').trim() === targetID;
  return !!(
    hasPendingLiveTurnBootstrap
    || 
    trackerActiveTurnId(chatState)
    || String(chatState.runningTurnId || '').trim()
    || String(chatState.activeStreamTurnId || '').trim()
    || chatState.lastHasRunning
  );
}

export async function dsTick(context, options = {}) {
  const requestedConversationID = String(options?.conversationID || getCurrentConversationID(context) || '').trim();
  const chatState = ensureContextResources(context);
  if (typeof window !== 'undefined') {
    try {
      window.__agentlyActiveChatState = chatState;
    } catch (_) {}
  }
  if (shouldDeferTranscriptToLiveStream(context, requestedConversationID)) {
    logExecutorDebug('transcript-deferred-to-live', {
      conversationId: requestedConversationID,
      liveOwnedConversationID: String(chatState?.liveOwnedConversationID || '').trim(),
      liveOwnedTurnIds: Array.isArray(chatState?.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [],
      runningTurnId: String(chatState?.runningTurnId || '').trim(),
      activeStreamTurnId: String(chatState?.activeStreamTurnId || '').trim(),
      trackerActiveTurnId: trackerActiveTurnId(chatState),
      lastHasRunning: !!chatState?.lastHasRunning
    });
    return {
      transcriptRows: chatState.transcriptRows,
      liveRows: chatState.liveRows,
      queuedTurns: chatState.lastQueuedTurns || [],
      hasRunning: true,
      runningTurnId:
        trackerActiveTurnId(chatState)
        || chatState.runningTurnId
        || chatState.activeStreamTurnId
        || '',
      conversationID: requestedConversationID,
      deferredToLiveStream: true
    };
  }
  if (transcriptShouldBeIdle(chatState, requestedConversationID)) {
    logExecutorDebug('transcript-dstick-while-live-owned', {
      conversationId: requestedConversationID,
      liveOwnedConversationID: String(chatState?.liveOwnedConversationID || '').trim(),
      liveOwnedTurnIds: Array.isArray(chatState?.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [],
      runningTurnId: String(chatState?.runningTurnId || '').trim(),
      activeStreamTurnId: String(chatState?.activeStreamTurnId || '').trim(),
      trackerActiveTurnId: trackerActiveTurnId(chatState),
      lastHasRunning: !!chatState?.lastHasRunning
    });
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
  const conversationID = String(result?.conversationID || getCurrentConversationID(context) || '').trim();
  if (conversationID) {
    await refreshGeneratedFiles(context, conversationID);
    renderMergedRowsForContext(context);
  }
  const ownsLiveTransport = shouldUseLiveStream(context, conversationID);
  if (result?.hasRunning && conversationID && !ownsLiveTransport && !chatState.stream) {
    queueTranscriptRefresh(context, { delay: 900 });
  }
  return result;
}

export function resetConversationSnapshotState(context) {
  const chatState = ensureContextResources(context);
  clearPendingStreamScheduling(chatState);
  clearPendingStreamReconnect(chatState);
  resetTranscriptState({
    context,
    ensureContextResources,
    getCurrentConversationID
  });
  resetLiveStreamState(chatState);
  if (chatState.streamTracker) {
    chatState.streamTracker.reset();
  }
  chatState.renderRows = [];
  chatState.generatedFiles = [];
}

export function queueTranscriptRefresh(context, { delay = 120, resetSince = false, force = false } = {}) {
  const currentConversationID = String(getCurrentConversationID(context) || '').trim();
  if (!force && shouldDeferTranscriptToLiveStream(context, currentConversationID)) {
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
  clearPendingStreamReconnect(chatState);
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
    onError: (error) => {
      logStreamDebug(chatState, 'stream-error', {
        conversationId: String(conversationID || '').trim(),
        error: String(error || '').trim()
      });
      if (String(error || '').trim().includes('unauthorized')) return;
      scheduleStreamReconnect(context, conversationID, error);
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
    try {
      chatState?.streamTracker?.applyEvent?.(payload);
      syncTrackerDerivedTurnState(chatState);
    } catch (_) {}
    // ── chatStore forwarding shim (PR-0 side-consumer cutover) ──────────────
    // Every SSE event that chatRuntime dispatches is also forwarded to the
    // canonical client chatStore. The legacy stores (liveRows/transcriptRows/
    // renderRows) are still maintained below for backward compatibility with
    // not-yet-migrated surfaces; chatStore runs as a live mirror so any
    // consumer that has already switched (e.g. ChatFeedFromChatStore) sees
    // identical data in real time. See ui-improvement.md §7.3.
    try {
      const cid = String(payload?.conversationId || conversationID || '').trim();
      if (cid) {
        // Lazy-import to avoid a circular dependency at module load time.
        const store = _chatStoreRef();
        if (store) store.onSSE(cid, payload);
      }
    } catch (_) { /* best-effort mirror */ }
    const eventConversationID = resolveStreamEventConversationID(payload, conversationID);
    if (type === 'conversation_meta_updated' && eventConversationID) {
      publishConversationMetaUpdated(eventConversationID, payload?.patch || {});
    }
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
    if (!shouldProcessStreamEvent({
      payload,
      subscribedConversationID: conversationID,
      visibleConversationID,
      switchingConversationID: chatState.switchingConversationID
    })) {
      logStreamDebug(chatState, 'stream-event-ignored', {
        type,
        eventConversationId: eventConversationID,
        visibleConversationId: visibleConversationID,
        switchingConversationId: String(chatState.switchingConversationID || '').trim(),
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
        eventSeq: payload?.eventSeq,
        mode: payload?.mode || payload?.patch?.mode,
        agentIdUsed: payload?.agentIdUsed,
        agentName: payload?.agentName,
        userMessageId: payload?.userMessageId,
        messageId: payload?.messageId,
        assistantMessageId: payload?.assistantMessageId,
        parentMessageId: payload?.parentMessageId,
        modelCallId: payload?.modelCallId,
        status: payload?.status,
        finalResponse: payload?.finalResponse,
        iteration: payload?.iteration,
        pageIndex: payload?.pageIndex,
        pageCount: payload?.pageCount,
        pageId: payload?.pageId,
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
      payloadEventSeq: Number(payload?.eventSeq || 0) || 0,
      payloadMode: String(payload?.mode || payload?.patch?.mode || '').trim(),
      payloadAgentIdUsed: String(payload?.agentIdUsed || '').trim(),
      payloadAgentName: String(payload?.agentName || '').trim(),
      payloadCreatedAt: String(payload?.createdAt || '').trim(),
      payloadStartedAt: String(payload?.startedAt || '').trim(),
      payloadCompletedAt: String(payload?.completedAt || '').trim(),
      payloadUserMessageId: String(payload?.userMessageId || '').trim(),
      payloadMessageId: String(payload?.messageId || '').trim(),
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
      payloadPageId: String(payload?.pageId || '').trim(),
      payloadPageIndex: Number(payload?.pageIndex || 0) || 0,
      payloadPageCount: Number(payload?.pageCount || 0) || 0
    });

    if (type !== 'text_delta') {
      flushQueuedTextDeltas(chatState, context, conversationID);
    }

    if (type === 'text_delta') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      applyStreamConversationState(context, 'thinking', payload);
      setStage({ phase: 'streaming', text: 'Streaming response…', startedAt: stageStartedAtValue(payload, chatState), completedAt: 0 });
      enqueueTextDelta(chatState, payload, conversationID);
      const queue = Array.isArray(chatState.pendingTextDeltaQueue) ? chatState.pendingTextDeltaQueue : [];
      const mergedPayload = queue[queue.length - 1] || payload;
      const streamID = String(mergedPayload?.streamId || conversationID);
      const streamMessageID = String(mergedPayload?.id || '').trim();
      const activeStreamRow = [...(Array.isArray(chatState.liveRows) ? chatState.liveRows : [])].reverse().find((row) => row?.isStreaming && String(row?.role || '').toLowerCase() === 'assistant')
        || latestAssistantRowForTurn(chatState, conversationID, String(mergedPayload?.turnId || payload?.turnId || '').trim());
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
      applyStreamConversationState(context, 'thinking', payload);
      setStage({ phase: 'streaming', text: 'Assistant reasoning…', startedAt: stageStartedAtValue(payload, chatState), completedAt: 0 });
      return;
    }

    if (type === 'tool_call_delta') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      applyStreamConversationState(context, 'executing', payload);
      setStage({ phase: 'executing', text: `Building ${String(payload?.toolName || 'tool')} arguments…`, startedAt: stageStartedAtValue(payload, chatState), completedAt: 0 });
      return;
    }

      if (type === 'model_started') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      applyStreamConversationState(context, 'thinking', payload);
      if (!chatState.activeStreamStartedAt) {
        chatState.activeStreamStartedAt = Date.now();
      }
      if (String(payload?.turnId || '').trim()) {
        chatState.activeStreamTurnId = String(payload.turnId).trim();
        chatState.runningTurnId = String(payload.turnId).trim();
        markLiveOwnedTurn(chatState, conversationID, String(payload.turnId).trim());
        applyTurnStartedEvent(chatState, enrichPayloadWithTurnAgent(chatState, context, payload), conversationID);
      }
      const enrichedPayload = enrichPayloadWithTurnAgent(chatState, context, payload);
      applyExecutionStreamEvent(chatState, enrichedPayload, conversationID);
      setStage({ phase: 'executing', text: 'Assistant executing…', startedAt: stageStartedAtValue(payload, chatState), completedAt: 0 });
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
      applyExecutionStreamEvent(chatState, enrichedPayload, conversationID);
      if (payload?.finalResponse) {
        finalizeStreamTurn(chatState, payload, conversationID);
        setStage({ phase: 'done', text: 'Done', completedAt: stageCompletedAtValue(payload) });
        window.setTimeout(() => setStage({ phase: 'ready', text: 'Ready' }), 1100);
      } else {
        setStage({ phase: 'executing', text: 'Assistant thinking…', startedAt: stageStartedAtValue(payload, chatState), completedAt: 0 });
      }
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'tool_calls_planned') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      // tool_calls_planned is emitted by the reactor when the LLM plans tool
      // calls. It carries toolCallsPlanned and content/narration. Update the
      // execution row so planned tools appear immediately in the UI.
      applyExecutionStreamEvent(chatState, enrichPayloadWithTurnAgent(chatState, context, payload), conversationID);
      applyStreamConversationState(context, 'executing', payload);
      setStage({ phase: 'executing', text: 'Planning tool calls…', startedAt: stageStartedAtValue(payload, chatState), completedAt: 0 });
      renderMergedRowsForContext(context);
      return;
    }

    if (
      type === 'tool_call_started'
      || type === 'tool_call_waiting'
      || type === 'tool_call_completed'
      || type === 'tool_call_failed'
      || type === 'tool_call_canceled'
    ) {
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
      applyToolStreamEvent(chatState, toolPayload, conversationID);
      applyStreamConversationState(context, 'executing', payload);
      const toolLabel = String(payload?.toolName || 'tool');
      const stageText = type === 'tool_call_completed'
        ? `Completed ${toolLabel}…`
        : type === 'tool_call_waiting'
          ? `Waiting on ${toolLabel}…`
          : type === 'tool_call_failed'
            ? `${toolLabel} failed…`
            : type === 'tool_call_canceled'
              ? `${toolLabel} canceled…`
              : `Executing ${toolLabel}…`;
      setStage({
        phase: 'executing',
        text: stageText,
        startedAt: stageStartedAtValue(payload, chatState),
        completedAt: 0
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
        if (!chatState.activeStreamStartedAt) {
          chatState.activeStreamStartedAt = Date.now();
        }
        if (turnId) {
          chatState.activeStreamTurnId = turnId;
          chatState.runningTurnId = turnId;
          markLiveOwnedTurn(chatState, conversationID, turnId);
          applyTurnStartedEvent(chatState, {
            ...(payload?.patch && typeof payload.patch === 'object' ? payload.patch : {}),
            turnId,
            conversationId: conversationID,
            createdAt: String(payload?.createdAt || payload?.patch?.createdAt || '').trim(),
            userMessageId: String(payload?.userMessageId || payload?.patch?.userMessageId || payload?.startedByMessageId || payload?.patch?.startedByMessageId || '').trim(),
            startedByMessageId: String(payload?.startedByMessageId || payload?.patch?.startedByMessageId || payload?.userMessageId || payload?.patch?.userMessageId || '').trim(),
            agentName: String(chatState?.activeTurnAgentName || '').trim()
          }, conversationID);
        }
        logStreamDebug(chatState, 'stream-control-turn-started', {
          turnId,
          status: String(payload?.patch?.status || '').trim(),
          agentIdUsed: String(payload?.patch?.agentIdUsed || '').trim(),
          agentName: String(chatState?.activeTurnAgentName || '').trim()
        });
        applyStreamConversationState(context, 'thinking', payload?.patch || payload);
        setStage({ phase: 'executing', text: 'Assistant executing…', startedAt: stageStartedAtValue(payload?.patch || payload, chatState), completedAt: 0 });
      } else if (op === 'message_patch') {
        chatState.lastHasRunning = true;
        logStreamDebug(chatState, 'stream-control-message-patch', {
          op: String(payload?.op || '').trim(),
          messageId: String(payload?.id || '').trim()
        });
        applyMessagePatchEvent(chatState, payload);
        renderMergedRowsForContext(context);
      } else if (op === 'message_add') {
        chatState.lastHasRunning = true;
        logStreamDebug(chatState, 'stream-control-message-add', {
          op: String(payload?.op || '').trim(),
          messageId: String(payload?.id || '').trim()
        });
        applyAssistantMessageAddEvent(chatState, payload);
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
      const terminalStatus = String(payload?.status || type).trim();
      const terminalError = String(payload?.error || payload?.errorMessage || '').trim();
      const finalRow = latestAssistantRowForTurn(chatState, resolvedConversationID, completedTurnID);
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
      chatState.transcriptRows = applyTerminalTurnStatusToTranscriptRows(
        chatState.transcriptRows,
        completedTurnID,
        terminalStatus,
        terminalError
      );
      if (finalContent === '') {
        logExecutorDebug('phantom-terminal', {
          type,
          conversationId: resolvedConversationID,
          turnId: completedTurnID,
          reason: 'terminal-event-without-final-content'
        });
      }
      if (completedTurnID) {
        chatState.terminalTurns[completedTurnID] = String(payload?.completedAt || payload?.createdAt || type || 'terminal').trim();
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
      applyStreamConversationState(context, 'terminal', { ...payload, type, status: terminalStatus });
      if (type === 'turn_failed') {
        setStage({ phase: 'error', text: String(payload?.error || 'Turn failed'), completedAt: stageCompletedAtValue(payload) });
      } else if (type === 'turn_canceled') {
        setStage({ phase: 'done', text: 'Canceled', completedAt: stageCompletedAtValue(payload) });
      } else {
        setStage({ phase: 'done', text: 'Done', completedAt: stageCompletedAtValue(payload) });
      }
      // Don't clear feeds on turn end — they persist until a tool_feed_inactive
      // SSE event arrives (e.g., after revert/commit removes the feed's data).
      window.setTimeout(() => setStage({ phase: 'ready', text: 'Ready' }), 1100);
      renderMergedRowsForContext(context);
      if (
        resolvedConversationID
        && resolvedConversationID === String(getCurrentConversationID(context) || '').trim()
        && !latestTurnStillOwnedByLive(chatState, resolvedConversationID, completedTurnID)
      ) {
        queuePostTurnConversationRefresh(context, resolvedConversationID, completedTurnID);
      }
      return;
    }

    if (type === 'narration') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      const preamblePayload = enrichPayloadWithTurnAgent(chatState, context, payload);
      logStreamDebug(chatState, 'stream-assistant-narration', {
        turnId: String(preamblePayload?.turnId || '').trim(),
        assistantMessageId: String(preamblePayload?.assistantMessageId || '').trim(),
        preambleLen: String(preamblePayload?.content || '').length,
        agentIdUsed: String(preamblePayload?.agentIdUsed || '').trim()
      });
      applyPreambleEvent(chatState, preamblePayload, conversationID);
      applyStreamConversationState(context, 'thinking', payload);
      setStage({ phase: 'streaming', text: 'Assistant thinking…', startedAt: stageStartedAtValue(payload, chatState), completedAt: 0 });
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'assistant') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      applyAssistantMessageAddEvent(chatState, enrichPayloadWithTurnAgent(chatState, context, payload));
      applyStreamConversationState(context, 'thinking', payload);
      setStage({ phase: 'executing', text: 'Assistant responding…', startedAt: stageStartedAtValue(payload, chatState), completedAt: 0 });
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'turn_started') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      rememberTurnAgent(chatState, context, payload);
      applyStreamConversationState(context, 'thinking', payload);
      if (!chatState.activeStreamStartedAt) {
        chatState.activeStreamStartedAt = Date.now();
      }
      const turnId = String(payload?.turnId || '').trim();
      if (turnId) {
        delete chatState.terminalTurns[turnId];
        chatState.activeStreamTurnId = turnId;
        chatState.runningTurnId = turnId;
        markLiveOwnedTurn(chatState, conversationID, turnId);
        applyTurnStartedEvent(chatState, enrichPayloadWithTurnAgent(chatState, context, payload), conversationID);
      }
      logStreamDebug(chatState, 'stream-turn-started', {
        turnId,
        agentIdUsed: String(payload?.agentIdUsed || '').trim(),
        agentName: String(chatState?.activeTurnAgentName || '').trim()
      });
      setStage({ phase: 'executing', text: 'Assistant executing…', startedAt: stageStartedAtValue(payload, chatState), completedAt: 0 });
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

      applyStreamConversationState(context, 'eliciting', payload);
      setStage({ phase: 'waiting', text: 'Waiting for input…' });
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'elicitation_resolved') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      clearPendingElicitation();
      applyStreamConversationState(context, 'thinking', payload);
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
      applyToolStreamEvent(chatState, payload, conversationID);
      applyStreamConversationState(context, 'executing', payload);
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'usage') {
      const usageConversationID = String(payload?.conversationId || payload?.streamId || conversationID || '').trim();
      if (usageConversationID) {
        publishUsage(usageConversationID, payload);
      }
      return;
    }

    if (type === 'item_completed') {
      // Metadata event — no UI action needed
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
  clearPendingStreamReconnect(chatState);
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
  const ownedConversationID = String(chatState.liveOwnedConversationID || '').trim();
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const currentConversationForm = conversationsDS?.peekFormData?.() || {};
  const formRunning = !!currentConversationForm?.running || isConversationLiveish(currentConversationForm);
  const trackerRunning = !!trackerActiveTurnId(chatState);
  const localRunning = !!String(chatState.runningTurnId || chatState.activeStreamTurnId || '').trim();
  if (currentConversationID && currentConversationID === targetID) {
    return formRunning || trackerRunning || localRunning || ownedConversationID === targetID;
  }
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
  const chatState = ensureContextResources(context);
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const messagesDS = context?.Context?.('messages')?.handlers?.dataSource;
  if (!conversationsDS || !messagesDS) return;

  const form = conversationsDS.peekFormData?.() || {};
  const currentID = String(form?.id || '').trim();
  if (currentID !== targetID) {
    chatState.switchingConversationID = targetID;
    if (chatState.stream) {
      chatState.stream.close();
      chatState.stream = null;
    }
  }
  const existing = await fetchConversation(targetID);
  if (!existing) {
    chatState.switchingConversationID = '';
    await createNewConversation(context);
    return;
  }
  const staleConversationState = String(chatState.lastConversationID || '').trim() !== targetID;
  if (currentID === targetID) {
    chatState.switchingConversationID = '';
    if (staleConversationState) {
      messagesDS.setCollection?.([]);
      messagesDS.setError?.('');
      resetConversationSnapshotState(context);
    }
    conversationsDS.setFormData?.({
      values: applyConversationFormSnapshot(form, existing)
    });
    const snapshot = await dsTick(context, { conversationID: targetID });
    if (snapshot?.hasRunning || isConversationLiveish(existing)) {
      syncConversationTransport(context, targetID);
    } else {
      disconnectStream(context);
    }
    publishActiveConversation(targetID, context);
    return;
  }

  conversationsDS.setFormData?.({
    values: applyConversationFormSnapshot(form, existing)
  });
  messagesDS.setCollection?.([]);
  messagesDS.setError?.('');
  resetConversationSnapshotState(context);
  chatState.switchingConversationID = '';
  const snapshot = await dsTick(context, { conversationID: targetID });
  if (snapshot?.hasRunning || isConversationLiveish(existing)) {
    syncConversationTransport(context, targetID);
  } else {
    disconnectStream(context);
  }
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
  messagesDS.setCollection?.(normalizeForContext(context, rows));
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
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
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
  chatState.switchingConversationID = '';
  chatState.explicitNewConversationRequested = true;
  const current = conversationsDS.peekFormData?.() || {};
  if (typeof window !== 'undefined') {
    try {
      const key = 'forge.composerDrafts.v1';
      const raw = window.sessionStorage?.getItem(key) || '{}';
      const parsed = JSON.parse(raw);
      const next = parsed && typeof parsed === 'object' ? parsed : {};
      const currentId = String(current?.id || '').trim();
      if (currentId) {
        delete next[currentId];
      }
      delete next.__pending__;
      window.sessionStorage?.setItem(key, JSON.stringify(next));
    } catch (_) {}
  }
  const metaForm = context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.() || {};
  const metaDefaults = metaForm?.defaults || {};
  const persistedAgent = resolveVisibleSelectedAgent(metaForm, getPersistedSelectedAgent());
  const preferredAgent = resolveVisibleSelectedAgent(
    metaForm,
    current?.agent,
    persistedAgent,
    metaForm?.agent,
    metaDefaults?.agent
  );
  // Merge the user's current agent/model selection from meta into current
  // so draftConversationValues preserves it for the new conversation.
  const merged = { ...current };
  if (preferredAgent) {
    merged.agent = preferredAgent;
  } else if (isVisibleAgent(metaForm, metaForm?.agent)) {
    merged.agent = metaForm.agent;
  }
  if (metaForm?.model) merged.model = metaForm.model;
  conversationsDS.setFormData?.({
    values: draftConversationValues(merged, metaDefaults, preferredAgent)
  });
  if (metaDS) {
    const starterTasks = resolveStarterTasks({
      agentInfos: Array.isArray(metaForm?.agentInfos) ? metaForm.agentInfos : [],
      selectedAgent: preferredAgent
    });
    metaDS.setFormData?.({
      values: {
        ...metaForm,
        agent: preferredAgent || metaForm?.agent || '',
        starterTasks
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
    const transcriptRows = Array.isArray(chatState.transcriptRows) ? chatState.transcriptRows : [];
    const hasFinishedSnapshot = transcriptRows.length > 0 && !chatState.lastHasRunning;
    if (hasFinishedSnapshot) return;
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
