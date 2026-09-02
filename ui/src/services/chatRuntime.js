import { conversationLifecyclePatchForStreamPhase, isLiveConversationState } from 'agently-core-ui-sdk';
import * as canonicalChatStore from './chatStore';

// Canonical chatStore is the authoritative projection owner. Tests may install
// an observer-compatible replacement, but production never runs without it.
let _chatStoreModule = canonicalChatStore;
const streamSubscriptionOwners = new Map();
let nextStreamSubscriptionID = 0;
function hasActiveConversationTurnStream(conversationID = '') {
  const id = String(conversationID || '').trim();
  const owner = id ? streamSubscriptionOwners.get(id) : null;
  return !!(owner?.active && owner?.subscription && owner?.liveTurn);
}
function _chatStoreRef() {
  return _chatStoreModule;
}

/**
 * Install a chatStore-compatible target for tests. Passing null restores the
 * production canonical store instead of disabling projection updates.
 */
export function installChatStoreMirror(chatStoreModule) {
  _chatStoreModule = chatStoreModule || canonicalChatStore;
}
import { rememberConversationSeedTitle } from './conversationTitle';
import {
  removePendingElicitation,
  replacePendingElicitationsForConversation,
  setPendingElicitation
} from './elicitationBus';
import { applyFeedEvent, clearFeedState, clearFeedStateForConversation, isFeedInactive } from './toolFeedBus';
import { publishUsage } from './usageBus';
import { request } from './httpClient';
import {
  clearWorkspaceWindowsForNewConversation,
  getWindowById,
  MAIN_CHAT_WINDOW_ID,
  getScopedConversationSelection,
  isMainChatWindowId,
  publishConversationSelection,
  syncHydratedWorkspaceStateFromTranscriptTurns
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
const STREAM_DEBUG_PREFIX = '[agently-stream]';
const EXECUTOR_DEBUG_PREFIX = '[agently-executor]';
const SIDEBAR_ACTIVITY_EVENT_TYPES = new Set([
  'turn_started',
  'turn_completed',
  'turn_failed',
  'turn_canceled',
  'linked_conversation_attached',
]);

function scheduleTimeout(callback, delay = 0) {
  const scheduler = typeof window !== 'undefined' && typeof window.setTimeout === 'function'
    ? window.setTimeout.bind(window)
    : (typeof globalThis.setTimeout === 'function' ? globalThis.setTimeout.bind(globalThis) : null);
  if (!scheduler) return 0;
  return scheduler(callback, delay);
}

export function resetRuntimeStreamState(chatState = {}, options = {}) {
  chatState.lastStreamEventAt = 0;
  chatState.activeStreamTurnId = '';
  chatState.activeStreamStartedAt = 0;
  chatState.liveOwnedConversationID = '';
  chatState.liveOwnedTurnIds = [];
  chatState.pendingTextDeltaQueue = [];
  if (!options?.preservePrompt) chatState.activeStreamPrompt = '';
}

function markRuntimeLiveTurn(chatState = {}, conversationID = '', turnID = '') {
  const nextConversationID = String(conversationID || '').trim();
  const nextTurnID = String(turnID || '').trim();
  if (nextConversationID) chatState.liveOwnedConversationID = nextConversationID;
  chatState.liveOwnedTurnIds = nextTurnID ? [nextTurnID] : [];
}

function finalizeRuntimeLiveTurn(chatState = {}, payload = {}) {
  const turnID = String(payload?.turnId || chatState.activeStreamTurnId || chatState.runningTurnId || '').trim();
  chatState.activeStreamTurnId = '';
  chatState.activeStreamStartedAt = 0;
  chatState.activeStreamPrompt = '';
  if (turnID) {
    chatState.liveOwnedTurnIds = (Array.isArray(chatState.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [])
      .map((value) => String(value || '').trim())
      .filter((value) => value && value !== turnID);
  }
  if (chatState.liveOwnedTurnIds.length === 0) chatState.liveOwnedConversationID = '';
}

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

export function logExecutorDebug(event, detail = {}) {
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
    canonicalActiveTurnId(chatState)
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
    const record = {
      seq,
      ts: new Date().toISOString(),
      event: String(event || '').trim() || 'unknown',
      elapsedMs,
      conversationId: String(chatState?.activeConversationID || chatState?.lastConversationID || '').trim(),
      activeStreamTurnId: String(chatState?.activeStreamTurnId || '').trim(),
      runningTurnId: String(chatState?.runningTurnId || '').trim(),
      ...detail
    };
    if (typeof window !== 'undefined') {
      const records = Array.isArray(window.__agentlyStreamDebug) ? window.__agentlyStreamDebug : [];
      records.push(record);
      if (records.length > 5000) records.splice(0, records.length - 5000);
      window.__agentlyStreamDebug = records;
    }
    console.log(STREAM_DEBUG_PREFIX, JSON.stringify(record));
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

export function publishConversationMetaUpdated(conversationID = '', patch = {}) {
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

function applyStreamConversationState(context, phase, payload = {}) {
  const patch = conversationLifecyclePatchForStreamPhase(phase, payload);
  if (!patch) return;
  updateConversationLiveState(context, patch);
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
  chatState.pendingStreamReconnect = scheduleTimeout(() => {
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

function canonicalActiveTurnId(chatState = {}, conversationID = '') {
  const targetID = String(conversationID || chatState?.activeConversationID || chatState?.lastConversationID || '').trim();
  return targetID ? String(_chatStoreRef()?.getActiveTurnId?.(targetID) || '').trim() : '';
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
  const starterTaskCategories = resolveStarterTaskCategories({
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
    starterTaskCategories,
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
      categoryId: String(entry.categoryId || '').trim(),
      title,
      prompt,
      description: String(entry.description || '').trim(),
      icon: String(entry.icon || '').trim(),
      agentId: String(agent?.id || '').trim(),
      agentName: String(agent?.name || '').trim()
    };
  }).filter(Boolean);
}

function normalizeStarterTaskCategoryEntries(entries = [], agent = null) {
  return (Array.isArray(entries) ? entries : []).map((entry) => {
    if (!entry || typeof entry !== 'object') return null;
    const id = String(entry.id || '').trim();
    const title = String(entry.title || '').trim();
    if (!id || !title) return null;
    return {
      id,
      title,
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

export function resolveStarterTaskCategories({ agentInfos = [], selectedAgent = '' } = {}) {
  const normalizedSelectedAgent = sanitizeAutoSelection(selectedAgent || '');
  const normalizedAgents = Array.isArray(agentInfos) ? agentInfos : [];
  const selectedEntries = normalizedSelectedAgent === 'auto'
    ? normalizedAgents
    : normalizedAgents.filter((entry) => String(entry?.id || '').trim() === normalizedSelectedAgent);
  const categories = selectedEntries.flatMap((entry) => normalizeStarterTaskCategoryEntries(entry?.starterTaskCategories, entry));
  const seen = new Set();
  return categories.filter((entry) => {
    const key = `${entry.agentId}|${entry.id}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export function mapTranscriptToRows(turns = [], options = {}) {
  const list = Array.isArray(turns) ? turns : [];
  const queuedTurns = list
    .filter((turn) => ['queued', 'pending', 'open'].includes(String(turn?.status || '').trim().toLowerCase()))
    .map((turn) => {
      const content = String(turn?.user?.content || '').trim();
      return {
        id: String(turn?.turnId || '').trim(),
        conversationId: String(turn?.conversationId || '').trim(),
        status: String(turn?.status || '').trim().toLowerCase(),
        queueSeq: turn?.queueSeq || null,
        content,
        preview: content.slice(0, 220),
        createdAt: turn?.createdAt || '',
        overrides: {
          agent: String(turn?.agentIdUsed || '').trim(),
          model: String(turn?.modelOverride || '').trim(),
          tools: [],
        },
      };
    })
    .sort((left, right) => Number(left.queueSeq || 0) - Number(right.queueSeq || 0) || left.id.localeCompare(right.id));
  return { rows: [], queuedTurns, runningTurnId: findLatestRunningTurnIdFromTurns(list) };
}

function isCanonicalTranscriptTurn(turn = {}) {
  return !!turn && typeof turn === 'object' && (
    Object.prototype.hasOwnProperty.call(turn, 'turnId')
    || Object.prototype.hasOwnProperty.call(turn, 'execution')
    || Object.prototype.hasOwnProperty.call(turn, 'assistant')
    || Object.prototype.hasOwnProperty.call(turn, 'user')
    || Object.prototype.hasOwnProperty.call(turn, 'elicitation')
  );
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

const settledConversationBootstrapCache = new Map();
const pendingConversationBootstrapIds = new Set();

export function cacheSettledConversationBootstrapSnapshot(conversationID = '', snapshot = null) {
  const id = String(conversationID || '').trim();
  if (!id || !snapshot || typeof snapshot !== 'object') return;
  const snapshotConversationID = String(snapshot?.conversation?.id || snapshot?.conversation?.Id || '').trim();
  if (!snapshotConversationID || snapshotConversationID !== id) {
    logExecutorDebug('settled-bootstrap-cache-rejected-conversation-mismatch', {
      conversationId: id,
      snapshotConversationId: snapshotConversationID
    });
    return;
  }
  settledConversationBootstrapCache.set(id, {
    conversation: snapshot.conversation && typeof snapshot.conversation === 'object' ? snapshot.conversation : null,
    turns: Array.isArray(snapshot.turns) ? snapshot.turns : [],
    pendingElicitations: Array.isArray(snapshot.pendingElicitations) ? snapshot.pendingElicitations : [],
    generatedFiles: Array.isArray(snapshot.generatedFiles) ? snapshot.generatedFiles : []
  });
  logExecutorDebug('settled-bootstrap-cache-set', {
    conversationId: id,
    turnCount: Array.isArray(snapshot.turns) ? snapshot.turns.length : 0,
    elicitationCount: Array.isArray(snapshot.pendingElicitations) ? snapshot.pendingElicitations.length : 0,
    generatedFileCount: Array.isArray(snapshot.generatedFiles) ? snapshot.generatedFiles.length : 0
  });
}

export function getSettledConversationBootstrapSnapshot(conversationID = '') {
  const id = String(conversationID || '').trim();
  if (!id) return null;
  const snapshot = settledConversationBootstrapCache.get(id) || null;
  if (!snapshot) return null;
  const snapshotConversationID = String(snapshot?.conversation?.id || snapshot?.conversation?.Id || '').trim();
  if (snapshotConversationID !== id) {
    settledConversationBootstrapCache.delete(id);
    logExecutorDebug('settled-bootstrap-cache-evicted-conversation-mismatch', {
      conversationId: id,
      snapshotConversationId: snapshotConversationID
    });
    return null;
  }
  return snapshot;
}

export function clearSettledConversationBootstrapSnapshot(conversationID = '') {
  const id = String(conversationID || '').trim();
  if (!id) return;
  settledConversationBootstrapCache.delete(id);
  logExecutorDebug('settled-bootstrap-cache-clear', {
    conversationId: id
  });
}

export function markPendingConversationBootstrap(conversationID = '') {
  const id = String(conversationID || '').trim();
  if (!id) return;
  pendingConversationBootstrapIds.add(id);
  logExecutorDebug('pending-conversation-bootstrap-set', {
    conversationId: id
  });
}

export function hasPendingConversationBootstrap(conversationID = '') {
  const id = String(conversationID || '').trim();
  return !!id && pendingConversationBootstrapIds.has(id);
}

export function clearPendingConversationBootstrap(conversationID = '') {
  const id = String(conversationID || '').trim();
  if (!id) return;
  pendingConversationBootstrapIds.delete(id);
  logExecutorDebug('pending-conversation-bootstrap-clear', {
    conversationId: id
  });
}

export function ensureContextResources(context) {
  context.resources = context.resources || {};
  context.resources.chat = context.resources.chat || {};
  context.resources.chat.liveOwnedConversationID = String(context.resources.chat.liveOwnedConversationID || '').trim();
  context.resources.chat.liveOwnedTurnIds = Array.isArray(context.resources.chat.liveOwnedTurnIds) ? context.resources.chat.liveOwnedTurnIds : [];
  context.resources.chat.generatedFiles = Array.isArray(context.resources.chat.generatedFiles) ? context.resources.chat.generatedFiles : [];
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
    chatState.postTurnRefreshTimer = scheduleTimeout(async () => {
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

export function renderMergedRowsForContext(context) {
  const chatState = ensureContextResources(context);
  const conversationID = String(getCurrentConversationID(context) || '').trim();
  const projection = conversationID ? (_chatStoreRef()?.getProjection?.(conversationID) || []) : [];
  const messagesDS = context?.Context?.('messages')?.handlers?.dataSource;
  messagesDS?.setCollection?.(projection);
  const queuedTurns = Array.isArray(chatState.lastQueuedTurns) ? chatState.lastQueuedTurns : [];
  const normalizedConversationID = conversationID;
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
  if (typeof window !== 'undefined') {
    window.__agentlyConversationDebug = {
      conversationId: conversationID,
      projection,
      runningTurnId: String(chatState.runningTurnId || '').trim(),
      activeStreamTurnId: String(chatState.activeStreamTurnId || '').trim(),
      lastHasRunning: !!chatState.lastHasRunning,
    };
  }
  return projection;
}

function canonicalHasAssistantRowForTurn(conversationID = '', turnID = '') {
  return !!_chatStoreRef()?.hasAssistantRowForTurn?.(conversationID, turnID);
}

export function latestAssistantRowForTurn(chatState = {}, conversationID = '', turnID = '') {
  const targetTurnID = String(turnID || '').trim();
  if (!targetTurnID) return null;
  const targetConversationID = String(conversationID || chatState?.activeConversationID || '').trim();
  return [...(_chatStoreRef()?.getProjection?.(targetConversationID) || [])]
    .reverse()
    .find((row) => String(row?.turnId || '').trim() === targetTurnID && (row?.kind === 'assistant' || row?.kind === 'iteration')) || null;
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

function resolveFeedResetConversationId(chatState = {}, explicitConversationId = '') {
  const direct = String(explicitConversationId || '').trim();
  if (direct) return direct;
  return String(
    chatState?.activeConversationID
    || chatState?.lastConversationID
    || ''
  ).trim();
}

function updateTranscriptFeedCache(chatState = {}, payload = {}, fallbackConversationID = '') {
  const conversationID = String(payload?.conversationId || payload?.streamId || fallbackConversationID || '').trim();
  const feedId = String(payload?.feedId || '').trim();
  if (!conversationID || !feedId) return;
  const current = chatState.lastTranscriptFeedsByConversation || {};
  const existing = Array.isArray(current[conversationID]) ? current[conversationID] : [];
  if (String(payload?.type || '').toLowerCase() === 'tool_feed_inactive') {
    const next = existing.filter((feed) => String(feed?.feedId || '').trim() !== feedId);
    chatState.lastTranscriptFeedsByConversation = {
      ...current,
      [conversationID]: next
    };
    return;
  }
  if (String(payload?.type || '').toLowerCase() !== 'tool_feed_active') return;
  const nextFeed = {
    feedId,
    title: payload?.feedTitle || feedId,
    developerOnly: payload?.feedDeveloperOnly === true,
    itemCount: payload?.feedItemCount || 0,
    data: payload?.feedData || null
  };
  const turnId = String(payload?.turnId || '').trim();
  if (turnId) nextFeed.turnId = turnId;
  if (payload?.feedIcon || payload?.feedAccent || payload?.feedTarget) {
    nextFeed.presentation = {
      icon: payload?.feedIcon || undefined,
      accent: payload?.feedAccent || undefined,
      target: payload?.feedTarget || undefined
    };
  }
  const next = [
    ...existing.filter((feed) => String(feed?.feedId || '').trim() !== feedId),
    nextFeed
  ];
  chatState.lastTranscriptFeedsByConversation = {
    ...current,
    [conversationID]: next
  };
}

function getContextWindowId(context) {
  return String(context?.identity?.windowId || '').trim() || MAIN_CHAT_WINDOW_ID;
}

export function publishActiveConversation(conversationID = '', context = null) {
  const id = String(conversationID || '').trim();
  const chatState = ensureContextResources(context);
  const contextWindowId = getContextWindowId(context);
  const contextWindow = getWindowById(contextWindowId);
  const targetWindowId = String(contextWindow?.windowKey || '').trim() === 'chat/new'
    ? contextWindowId
    : MAIN_CHAT_WINDOW_ID;
  const currentRouteConversationId = isMainChatWindowId(targetWindowId) && typeof window !== 'undefined'
    ? conversationIDFromPath(window.location?.pathname)
    : '';
  // The URL is the user's source of truth for which conversation they're
  // viewing. Only seed it when there is no route yet (deep-link bootstrap /
  // fresh new-conversation), or no-op when it already matches the published
  // id. NEVER rewrite the URL on behalf of a different id, even when the
  // calling context's own form happens to be on that id — that context is
  // very likely a stale/background completion for a conversation the user
  // just navigated away from (e.g. user clicked B, in-flight load for A
  // finishes and publishes A; without this guard the URL silently snaps
  // back to A while the visible chat is B). Convergence in the legitimate
  // case is handled by the explicit selection path (openConversationInMainWindow
  // and requestNewConversationInMainWindow both call publishConversationSelection
  // with syncPath: true) and by the poller's switchingConversationID guard
  // in startPolling.
  // A stale/background context must not overwrite shared selection state or
  // emit an active-conversation event after the user has selected another
  // route. The previous URL-only guard still allowed that stale event to move
  // the sidebar highlight back to the old conversation.
  if (
    isMainChatWindowId(targetWindowId)
    && currentRouteConversationId
    && id
    && currentRouteConversationId !== id
  ) {
    logExecutorDebug('publish-active-conversation-ignored-stale-route', {
      conversationId: id,
      routeConversationId: currentRouteConversationId,
      windowId: targetWindowId
    });
    return;
  }
  // An explicit New Conversation reset owns the empty route until the next
  // submit creates a new id. Ignore late async completion from the previously
  // selected conversation; otherwise it can repopulate both the route and the
  // old transcript/tool-feed surface after the reset.
  if (
    isMainChatWindowId(targetWindowId)
    && !currentRouteConversationId
    && id
    && chatState.explicitNewConversationRequested === true
  ) {
    logExecutorDebug('publish-active-conversation-ignored-explicit-new', {
      conversationId: id,
      windowId: targetWindowId
    });
    return;
  }
  publishConversationSelection(targetWindowId, id, {
    syncPath: isMainChatWindowId(targetWindowId)
      && (!currentRouteConversationId || currentRouteConversationId === id),
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
  const pendingBootstrap = hasPendingConversationBootstrap(conversationID);
  if (!pendingBootstrap && (hasActiveConversationTurnStream(conversationID) || latestTurnLiveOwned)) {
    logExecutorDebug('transcript-fetch-deferred-to-sse-owner', {
      conversationId: String(conversationID || '').trim(),
      pendingBootstrap,
      activeStreamOwner: hasActiveConversationTurnStream(conversationID),
      localLiveOwner: latestTurnLiveOwned
    });
    return [];
  }
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
        canonicalActiveTurnId: canonicalActiveTurnId(chatState),
        lastHasRunning: !!chatState?.lastHasRunning
      });
    }
  }
  const includeExecutionDetails = options?.includeExecutionDetails !== false;
  const includeFeeds = options?.includeFeeds !== false;
  const transcriptInput = {
    conversationId: conversationID,
    includeModelCalls: includeExecutionDetails,
    includeToolCalls: includeExecutionDetails,
    includeFeeds,
    since: since || undefined,
  };
  const transcriptOptions = options?.selectors
    ? { selectors: options.selectors }
    : undefined;
  const payload = pendingBootstrap && !since && typeof client.getLiveState === 'function'
    ? await client.getLiveState(transcriptInput, transcriptOptions)
    : await client.getTranscript(transcriptInput, transcriptOptions);
  const data = payload || {};
  const canonicalConversation = data?.conversation && typeof data.conversation === 'object'
    ? data.conversation
    : null;
  const canonicalTurns = Array.isArray(canonicalConversation?.turns) ? canonicalConversation.turns : null;
  const canonicalHasRunning = Array.isArray(canonicalTurns)
    && canonicalTurns.some((turn) => {
      const status = String(turn?.status || '').trim().toLowerCase();
      return ['running', 'thinking', 'processing', 'waiting_for_user', 'in_progress'].includes(status);
    });
  const liveOwnedTurnIds = Array.isArray(activeChatState?.liveOwnedTurnIds)
    ? activeChatState.liveOwnedTurnIds.map((value) => String(value || '').trim()).filter(Boolean)
    : [];
  const pendingBootstrapOwned = canonicalConversation?.conversationId
    ? hasPendingConversationBootstrap(canonicalConversation.conversationId)
    : false;
  if (canonicalConversation?.conversationId && !pendingBootstrapOwned) {
    const store = _chatStoreRef();
    if (!store || typeof store.onTranscript !== 'function') {
      throw new Error('canonical chatStore is not configured');
    }
    // Field provenance in the canonical reducer protects live SSE values while
    // allowing persisted transcript data to refine and settle the same entity.
    store.onTranscript(canonicalConversation.conversationId, canonicalConversation);
  }
  try {
    if (canonicalConversation?.conversationId && Array.isArray(canonicalTurns) && canonicalTurns.length > 0 && !canonicalHasRunning) {
      await syncHydratedWorkspaceStateFromTranscriptTurns(canonicalConversation.conversationId, canonicalTurns, {
        reopen: false,
        announce: true,
      });
    }
  } catch (_) { /* best-effort workspace restore cache */ }
  const resolvedFeeds = Array.isArray(data?.feeds)
    ? data.feeds
    : (Array.isArray(canonicalConversation?.feeds) ? canonicalConversation.feeds : []);
  if ((!latestTurnLiveOwned || !canonicalHasRunning) && activeChatState && conversationID) {
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

export async function fetchPendingElicitations(conversationID = '') {
  const id = String(conversationID || '').trim();
  if (!id) return [];
  return client.listPendingElicitations(id);
}

export async function fetchConversation(conversationID = '') {
  const id = String(conversationID || '').trim();
  if (!id) return null;
  const data = await client.getConversation(id);
  if (!data || typeof data !== 'object') return null;
  const resolvedID = String(data?.id || data?.Id || '').trim();
  if (resolvedID) {
    // Update usage display from conversation data.
    publishUsage(resolvedID, data);
    return data;
  }
  return null;
}

export async function refreshGoalFeed(conversationID = '') {
  const id = String(conversationID || '').trim();
  if (!id) return;
  try {
    const goal = await client.getGoal(id);
    if (!goal || typeof goal !== 'object') {
      applyFeedEvent({
        type: 'tool_feed_inactive',
        feedId: 'goal',
        conversationId: id,
      });
      return;
    }
    applyFeedEvent({
      type: 'tool_feed_active',
      feedId: 'goal',
      feedTitle: 'Goal',
      feedItemCount: 1,
      conversationId: id,
      feedData: {
        ui: { title: 'Goal' },
        data: { goal },
      },
      localOnly: true,
    });
  } catch (_) {
    applyFeedEvent({
      type: 'tool_feed_inactive',
      feedId: 'goal',
      conversationId: id,
    });
  }
}

export function hydrateConversationFromBootstrapSnapshot(context, snapshot = null) {
  if (!snapshot || typeof snapshot !== 'object') return false;
  const conversation = snapshot.conversation && typeof snapshot.conversation === 'object' ? snapshot.conversation : null;
  const conversationID = String(conversation?.id || conversation?.Id || '').trim();
  if (!conversationID) return false;
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  if (!conversationsDS) return false;
  const currentForm = conversationsDS.peekFormData?.() || {};
  conversationsDS.setFormData?.({
    values: applyConversationFormSnapshot(currentForm, conversation)
  });
  publishUsage(conversationID, conversation);
  const chatState = ensureContextResources(context);
  chatState.generatedFiles = Array.isArray(snapshot.generatedFiles) ? snapshot.generatedFiles : [];
  syncMessagesSnapshot(context, Array.isArray(snapshot.turns) ? snapshot.turns : [], 'bootstrap-cache', Array.isArray(snapshot.pendingElicitations) ? snapshot.pendingElicitations : []);
  renderMergedRowsForContext(context);
  return true;
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
  const metaContext = context?.Context?.('meta');
  const metaDS = metaContext?.handlers?.dataSource;
  if (!metaDS) return;
  const current = metaDS.peekFormData?.() || {};
  if (Array.isArray(current?.agentOptions) && current.agentOptions.length > 0 &&
      Array.isArray(current?.modelOptions) && current.modelOptions.length > 0) {
    return;
  }
  if (typeof metaDS.fetchCollection === 'function') {
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
  const normalizedTurns = Array.isArray(turns) ? turns : [];
  const mapped = mapTranscriptToRows(normalizedTurns, { pendingElicitations });
  const runningTurnId = String(
    mapped?.runningTurnId
    || findLatestRunningTurnIdFromTurns(normalizedTurns)
    || canonicalActiveTurnId(chatState, currentConversationID)
    || ''
  ).trim();
  const hasRunning = normalizedTurns.some((turn) => RUNNING_STATUSES.has(String(turn?.status || turn?.Status || '').trim().toLowerCase()));
  const queuedTurns = Array.isArray(mapped?.queuedTurns) ? mapped.queuedTurns : [];
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const convForm = conversationsDS?.peekFormData?.() || {};
  conversationsDS?.setFormData?.({ values: { ...convForm, running: hasRunning } });

  chatState.activeConversationID = currentConversationID;
  chatState.lastSyncReason = reason;
  chatState.lastQueuedTurns = queuedTurns;
  chatState.lastHasRunning = hasRunning;
  chatState.lastConversationID = currentConversationID;
  chatState.runningTurnId = hasRunning ? runningTurnId : '';
  if (hasRunning && currentConversationID && runningTurnId) {
    markRuntimeLiveTurn(chatState, currentConversationID, runningTurnId);
  } else if (!hasRunning) {
    finalizeRuntimeLiveTurn(chatState, { turnId: chatState.activeStreamTurnId || chatState.runningTurnId });
  }

  if (currentConversationID) {
    replacePendingElicitationsForConversation(currentConversationID, pendingElicitations);
  }
  const transcriptFeeds = Array.isArray(chatState.lastTranscriptFeedsByConversation?.[currentConversationID])
    ? chatState.lastTranscriptFeedsByConversation[currentConversationID]
    : [];
  for (const feed of transcriptFeeds) {
    const feedId = String(feed?.feedId || '').trim();
    if (!feedId || isFeedInactive(feedId, currentConversationID)) continue;
    applyFeedEvent({
      type: 'tool_feed_active',
      feedId,
      turnId: String(feed?.turnId || '').trim(),
      feedTitle: feed.title || feedId,
      feedDeveloperOnly: feed.developerOnly === true,
      feedIcon: feed.presentation?.icon,
      feedAccent: feed.presentation?.accent,
      feedTarget: feed.presentation?.target,
      feedItemCount: feed.itemCount || 0,
      feedData: feed.data || null,
      conversationId: currentConversationID,
    });
  }
  if (currentConversationID && !hasRunning) {
    Promise.resolve(syncHydratedWorkspaceStateFromTranscriptTurns(currentConversationID, normalizedTurns, {
      reopen: true,
      announce: true,
    })).catch(() => {});
  }
  if (hasRunning) {
    setStage({ phase: 'executing', text: 'Assistant executing…' });
  } else if (queuedTurns.length > 0) {
    setStage({ phase: 'waiting', text: `Queued turns: ${queuedTurns.length}` });
  } else if (reason === 'poll' || reason === 'fetch') {
    setStage({ phase: 'ready', text: 'Ready' });
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
    canonicalActiveTurnId(chatState, targetID)
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
  const pendingBootstrapOwned = hasPendingConversationBootstrap(requestedConversationID);
  const activeStreamOwned = hasActiveConversationTurnStream(requestedConversationID);
  if (pendingBootstrapOwned || (!options?.allowLiveHydration && (
    activeStreamOwned || shouldDeferTranscriptToLiveStream(context, requestedConversationID)
  ))) {
    logExecutorDebug('transcript-deferred-to-live', {
      conversationId: requestedConversationID,
      liveOwnedConversationID: String(chatState?.liveOwnedConversationID || '').trim(),
      liveOwnedTurnIds: Array.isArray(chatState?.liveOwnedTurnIds) ? chatState.liveOwnedTurnIds : [],
      runningTurnId: String(chatState?.runningTurnId || '').trim(),
      activeStreamTurnId: String(chatState?.activeStreamTurnId || '').trim(),
      canonicalActiveTurnId: canonicalActiveTurnId(chatState, requestedConversationID),
      lastHasRunning: !!chatState?.lastHasRunning
    });
    return {
      projection: _chatStoreRef()?.getProjection?.(requestedConversationID) || [],
      queuedTurns: chatState.lastQueuedTurns || [],
      hasRunning: true,
      runningTurnId:
        canonicalActiveTurnId(chatState, requestedConversationID)
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
      canonicalActiveTurnId: canonicalActiveTurnId(chatState, requestedConversationID),
      lastHasRunning: !!chatState?.lastHasRunning
    });
  }
  const conversationID = requestedConversationID;
  const since = String(chatState.lastSinceCursor || '').trim();
  const transcriptOptions = options?.transcript && typeof options.transcript === 'object'
    ? options.transcript
    : {};
  let turns = Array.isArray(options?.prefetchedTranscriptTurns)
    ? options.prefetchedTranscriptTurns
    : await fetchTranscript(conversationID, since, transcriptOptions);
  if (String(getCurrentConversationID(context) || '').trim() !== conversationID) return;
  if (since && turns.length === 0 && (chatState.lastHasRunning || (_chatStoreRef()?.getProjection?.(conversationID) || []).length > 0)) {
    turns = await fetchTranscript(conversationID, '', transcriptOptions);
    if (String(getCurrentConversationID(context) || '').trim() !== conversationID) return;
  }
  if (turns.length > 0) chatState.lastSinceCursor = resolveLastTranscriptCursor(turns);
  const pendingElicitations = Array.isArray(options?.prefetchedPendingElicitations)
    ? options.prefetchedPendingElicitations
    : await fetchPendingElicitations(conversationID);
  syncMessagesSnapshot(context, turns, String(options?.reason || 'poll').trim() || 'poll', pendingElicitations);
  const result = {
    projection: _chatStoreRef()?.getProjection?.(conversationID) || [],
    queuedTurns: chatState.lastQueuedTurns || [],
    hasRunning: !!chatState.lastHasRunning,
    runningTurnId: String(chatState.runningTurnId || '').trim(),
    conversationID,
  };
  if (conversationID) {
    await refreshGeneratedFiles(context, conversationID);
    renderMergedRowsForContext(context);
  }
  const transcriptReportedRunning = !!(
    result?.hasRunning
    || chatState?.lastHasRunning
    || String(chatState?.runningTurnId || '').trim()
    || String(chatState?.activeStreamTurnId || '').trim()
  );
  if (transcriptReportedRunning && conversationID && !chatState.stream) {
    const promoted = syncConversationTransport(context, conversationID);
    if (promoted) {
      return result;
    }
  }
  if (transcriptReportedRunning && conversationID && !chatState.stream) {
    queueTranscriptRefresh(context, { delay: 900 });
  }
  return result;
}

export function resetConversationSnapshotState(context) {
  const chatState = ensureContextResources(context);
  clearPendingStreamReconnect(chatState);
  chatState.lastSinceCursor = '';
  chatState.lastTranscriptFeedsByConversation = {};
  chatState.lastQueuedTurns = [];
  chatState.lastHasRunning = false;
  chatState.runningTurnId = '';
  resetRuntimeStreamState(chatState);
  chatState.generatedFiles = [];
  chatState.prefetchedTerminalConversationID = '';
  chatState.prefetchedTerminalTurnID = '';
  chatState.pendingTerminalRefreshSuppressionConversationID = '';
  chatState.pendingTerminalRefreshSuppressionTurnID = '';
}

export function queueTranscriptRefresh(context, { delay = 120, resetSince = false, force = false } = {}) {
  const currentConversationID = String(getCurrentConversationID(context) || '').trim();
  if (!force && shouldDeferTranscriptToLiveStream(context, currentConversationID)) {
    return null;
  }
  const chatState = ensureContextResources(context);
  if (resetSince) chatState.lastSinceCursor = '';
  if (chatState.refreshTimer) {
    clearTimeout(chatState.refreshTimer);
    chatState.refreshTimer = null;
  }
  chatState.refreshTimer = scheduleTimeout(async () => {
    chatState.refreshTimer = null;
    if (chatState.refreshInFlight) return;
    chatState.refreshInFlight = true;
    try {
      await dsTick(context);
    } finally {
      chatState.refreshInFlight = false;
    }
  }, Math.max(0, Number(delay) || 0));
  return chatState.refreshTimer;
}

export function connectStream(context, conversationID) {
  const chatState = ensureContextResources(context);
  const targetConversationID = String(conversationID || '').trim();
  clearPendingStreamReconnect(chatState);
  const generation = Number(chatState.streamGeneration || 0) + 1;
  nextStreamSubscriptionID += 1;
  const subscriptionID = `${targetConversationID}:${nextStreamSubscriptionID}:${Date.now()}`;
  chatState.streamGeneration = generation;
  chatState.activeStreamSubscriptionID = subscriptionID;
  chatState.streamSubscriptionEventSeq = 0;
  const previousOwner = streamSubscriptionOwners.get(targetConversationID);
  if (previousOwner) {
    previousOwner.active = false;
    try { previousOwner.subscription?.close?.(); } catch (_) {}
    if (previousOwner.chatState?.stream === previousOwner.subscription) {
      previousOwner.chatState.stream = null;
    }
    streamSubscriptionOwners.delete(targetConversationID);
  }
  if (chatState.stream) {
    logStreamDebug(chatState, 'stream-close-replaced', {
      conversationId: String(chatState.activeConversationID || '').trim()
    });
    chatState.stream.close();
    chatState.stream = null;
  }
  let subscription = null;
  const owner = {
    active: true,
    liveTurn: true,
    chatState,
    subscription: null,
    subscriptionID,
  };
  const isCurrentSubscription = () => (
    owner.active
    && streamSubscriptionOwners.get(targetConversationID) === owner
    && Number(chatState.streamGeneration || 0) === generation
    && String(chatState.activeStreamSubscriptionID || '') === subscriptionID
    && chatState.stream === subscription
  );
  subscription = client.streamEvents(conversationID, {
    onEvent: (payload) => {
      const content = String(payload?.content || '');
      if (!isCurrentSubscription()) {
        logStreamDebug(chatState, 'stream-event-ignored-stale-subscription', {
          subscriptionID,
          generation,
          type: String(payload?.type || '').trim(),
          contentLength: content.length,
          contentHash: streamDebugHash(content)
        });
        return;
      }
      const payloadType = String(payload?.type || '').trim().toLowerCase();
      if (payloadType === 'turn_completed' || payloadType === 'turn_failed' || payloadType === 'turn_canceled') {
        owner.liveTurn = false;
      } else if (payloadType === 'turn_started') {
        owner.liveTurn = true;
      }
      chatState.streamSubscriptionEventSeq = Number(chatState.streamSubscriptionEventSeq || 0) + 1;
      logStreamDebug(chatState, 'stream-js-event', {
        subscriptionID,
        generation,
        subscriptionEventSeq: chatState.streamSubscriptionEventSeq,
        type: String(payload?.type || '').trim(),
        eventSeq: Number(payload?.eventSeq || 0) || 0,
        messageID: String(payload?.messageId || payload?.assistantMessageId || payload?.id || '').trim(),
        contentLength: content.length,
        contentHash: streamDebugHash(content)
      });
      handleStreamEvent(chatState, context, conversationID, payload);
    },
    onError: (error) => {
      if (!isCurrentSubscription()) {
        logStreamDebug(chatState, 'stream-error-ignored-stale-subscription', {
          subscriptionID,
          generation,
          error: String(error || '').trim()
        });
        return;
      }
      logStreamDebug(chatState, 'stream-error', {
        conversationId: String(conversationID || '').trim(),
        error: String(error || '').trim()
      });
      if (String(error || '').trim().includes('unauthorized')) return;
      scheduleStreamReconnect(context, conversationID, error);
    },
  });
  owner.subscription = subscription;
  streamSubscriptionOwners.set(targetConversationID, owner);
  chatState.stream = subscription;
  chatState.activeConversationID = String(conversationID || '').trim();
  chatState.streamOpenedAt = Date.now();
  logStreamDebug(chatState, 'stream-connect', {
    conversationId: String(conversationID || '').trim(),
    subscriptionID,
    generation
  });
}

function streamDebugHash(value = '') {
  const text = String(value || '');
  let hash = 2166136261;
  for (let i = 0; i < text.length; i += 1) {
    hash ^= text.charCodeAt(i);
    hash = Math.imul(hash, 16777619);
  }
  return (hash >>> 0).toString(16).padStart(8, '0');
}

export function handleStreamEvent(chatState, context, conversationID, payload) {
    const type = String(payload?.type || '').toLowerCase();
    const turnId = String(payload?.turnId || payload?.patch?.turnId || '').trim();
    chatState.terminalTurns = chatState.terminalTurns || {};
    if (turnId && chatState.terminalTurns[turnId] && type !== 'turn_completed' && type !== 'turn_failed' && type !== 'turn_canceled') {
      const eventConversationID = resolveStreamEventConversationID(payload, conversationID);
      const contextWindowID = getContextWindowId(context);
      const scopedConversationID = getScopedConversationSelection(contextWindowID);
      const formConversationID = getCurrentConversationID(context);
      const visibleConversationID = String(formConversationID || scopedConversationID || '').trim();
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
    // Canonical projection is updated before compatibility side effects. A
    // reducer error is not swallowed: continuing with only legacy state would
    // make the visible feed diverge from transport/lifecycle state.
    const cid = String(payload?.conversationId || conversationID || '').trim();
    if (cid) {
      const store = _chatStoreRef();
      if (!store || typeof store.onSSE !== 'function') {
        throw new Error('canonical chatStore is not configured');
      }
      store.onSSE(cid, payload);
    }
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

    if (type === 'text_delta') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      applyStreamConversationState(context, 'thinking', payload);
      setStage({ phase: 'streaming', text: 'Streaming response…', startedAt: stageStartedAtValue(payload, chatState), completedAt: 0 });
      const streamID = String(payload?.streamId || conversationID);
      const streamMessageID = String(payload?.id || payload?.messageId || payload?.assistantMessageId || '').trim();
      const projection = _chatStoreRef()?.getProjection?.(eventConversationID) || [];
      const activeStreamRow = [...projection].reverse().find((row) => row?.kind === 'assistant' || row?.kind === 'iteration');
      logStreamDebug(chatState, 'stream-chunk-merged', {
        streamId: streamID,
        streamMessageId: streamMessageID,
        chunkChars: String(payload?.content || '').length,
        totalChars: String(activeStreamRow?.content || '').length,
        rowCount: projection.length,
        turnId: String(activeStreamRow?.turnId || '').trim()
      });
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
        markRuntimeLiveTurn(chatState, conversationID, String(payload.turnId).trim());
      }
      setStage({ phase: 'executing', text: 'Assistant executing…', startedAt: stageStartedAtValue(payload, chatState), completedAt: 0 });
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'model_completed') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
      if (String(payload?.turnId || '').trim()) {
        markRuntimeLiveTurn(chatState, conversationID, String(payload.turnId).trim());
      }
      if (payload?.finalResponse) {
        finalizeRuntimeLiveTurn(chatState, payload);
        setStage({ phase: 'done', text: 'Done', completedAt: stageCompletedAtValue(payload) });
        scheduleTimeout(() => setStage({ phase: 'ready', text: 'Ready' }), 1100);
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
          markRuntimeLiveTurn(chatState, conversationID, turnId);
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
        renderMergedRowsForContext(context);
      } else if (op === 'message_add') {
        chatState.lastHasRunning = true;
        logStreamDebug(chatState, 'stream-control-message-add', {
          op: String(payload?.op || '').trim(),
          messageId: String(payload?.id || '').trim()
        });
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
        finalRowId: String(finalRow?.id || '').trim(),
        finalRowContentLen: String(finalRow?.content || '').trim().length,
        canonicalRowCount: (_chatStoreRef()?.getProjection?.(resolvedConversationID) || []).length,
        canonicalHasAssistantRow: canonicalHasAssistantRowForTurn(resolvedConversationID, completedTurnID),
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
      if (completedTurnID) {
        chatState.terminalTurns[completedTurnID] = String(payload?.completedAt || payload?.createdAt || type || 'terminal').trim();
      }
      logStreamDebug(chatState, 'stream-done', {
        status: String(payload?.status || type).trim()
      });
      finalizeRuntimeLiveTurn(chatState, payload);
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
      publishConversationMetaUpdated(resolvedConversationID, {
        ...(conversationLifecyclePatchForStreamPhase('terminal', { ...payload, type, status: terminalStatus }) || {})
      });
      if (type === 'turn_failed') {
        setStage({ phase: 'error', text: String(payload?.error || 'Turn failed'), completedAt: stageCompletedAtValue(payload) });
      } else if (type === 'turn_canceled') {
        setStage({ phase: 'done', text: 'Canceled', completedAt: stageCompletedAtValue(payload) });
      } else {
        setStage({ phase: 'done', text: 'Done', completedAt: stageCompletedAtValue(payload) });
      }
      logExecutorDebug('turn-terminal-stream-settled', {
        conversationId: resolvedConversationID,
        turnId: completedTurnID,
        hasFinalContent: finalContent !== ''
      });
      // Active conversation rendering remains SSE-owned through the terminal
      // event. Re-fetching the transcript here replaces stable live entities,
      // remounts progressive reports, and can overwrite a newer streamed
      // snapshot with a stale persisted one. Transcript hydration is reserved
      // for explicit conversation navigation/recovery.
      chatState.prefetchedTerminalConversationID = '';
      chatState.prefetchedTerminalTurnID = '';
      chatState.pendingTerminalRefreshSuppressionConversationID = '';
      chatState.pendingTerminalRefreshSuppressionTurnID = '';
      if (resolvedConversationID) {
        clearPendingConversationBootstrap(resolvedConversationID);
      }
      // Don't clear feeds on turn end — they persist until a tool_feed_inactive
      // SSE event arrives (e.g., after revert/commit removes the feed's data).
      scheduleTimeout(() => setStage({ phase: 'ready', text: 'Ready' }), 1100);
      renderMergedRowsForContext(context);
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
      applyStreamConversationState(context, 'thinking', payload);
      setStage({ phase: 'streaming', text: 'Assistant thinking…', startedAt: stageStartedAtValue(payload, chatState), completedAt: 0 });
      renderMergedRowsForContext(context);
      return;
    }

    if (type === 'assistant') {
      chatState.lastStreamEventAt = Date.now();
      chatState.lastHasRunning = true;
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
        markRuntimeLiveTurn(chatState, conversationID, turnId);
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
          mode: elicMode,
          source: 'stream'
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
      removePendingElicitation({
        conversationId: String(payload?.conversationId || payload?.streamId || conversationID || '').trim(),
        elicitationId: String(payload?.elicitationId || '').trim()
      }, { allConversationsForElicitation: true });
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
      updateTranscriptFeedCache(chatState, payload, conversationID);
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
  const targetConversationID = String(
    chatState.activeConversationID || getCurrentConversationID(context) || ''
  ).trim();
  const owner = streamSubscriptionOwners.get(targetConversationID);
  if (owner?.chatState === chatState) {
    owner.active = false;
    streamSubscriptionOwners.delete(targetConversationID);
  }
  chatState.streamGeneration = Number(chatState.streamGeneration || 0) + 1;
  chatState.activeStreamSubscriptionID = '';
  if (chatState.stream) {
    logStreamDebug(chatState, 'stream-close-manual', {
      conversationId: String(chatState.activeConversationID || '').trim()
    });
    chatState.stream.close();
    chatState.stream = null;
  }
  // A pending bootstrap is owned by the live subscription. Once that
  // subscription is intentionally torn down, a later visit must recover from
  // canonical conversation/transcript state instead of waiting forever for a
  // terminal event on the closed stream.
  if (targetConversationID) {
    clearPendingConversationBootstrap(targetConversationID);
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
  const trackerRunning = !!canonicalActiveTurnId(chatState, targetID);
  const localRunning = !!String(chatState.runningTurnId || chatState.activeStreamTurnId || '').trim();
  const conversationLiveish = formRunning || trackerRunning || localRunning;
  if (currentConversationID && currentConversationID === targetID) {
    return conversationLiveish || ownedConversationID === targetID;
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
  const immediateSubmit = !!options?.immediateSubmit;
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
    if (immediateSubmit) {
      chatState.pendingInitialSubmitConversationID = id;
    }
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
  const chatState = ensureContextResources(context);
  chatState.requestedConversationID = targetID;
  const isCurrentRequest = () => String(chatState.requestedConversationID || '').trim() === targetID;
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const messagesDS = context?.Context?.('messages')?.handlers?.dataSource;
  if (!conversationsDS || !messagesDS) return;

  const form = conversationsDS.peekFormData?.() || {};
  const currentID = String(form?.id || '').trim();
  const lastConversationID = String(chatState.lastConversationID || '').trim();
  const terminalHydrationPending = String(chatState.pendingTerminalHydrationConversationID || '').trim();
  const initialSubmitPending = String(chatState.pendingInitialSubmitConversationID || '').trim();
  const canonicalProjection = _chatStoreRef()?.getProjection?.(targetID) || [];
  const hasSettledTranscript = canonicalProjection.length > 0 && !chatState.lastHasRunning;
  if (
    currentID === targetID
    && lastConversationID === targetID
    && !chatState.switchingConversationID
    && !terminalHydrationPending
    && !initialSubmitPending
    && !isConversationLiveish(form)
    && hasSettledTranscript
  ) {
    logExecutorDebug('switch-conversation-skip-settled-self', {
      conversationId: targetID,
      lastConversationId: lastConversationID,
      canonicalRowCount: canonicalProjection.length
    });
    publishActiveConversation(targetID, context);
    return;
  }
  const cachedSettledSnapshot = getSettledConversationBootstrapSnapshot(targetID);
  if (
    cachedSettledSnapshot
    && !terminalHydrationPending
    && !initialSubmitPending
    && !isConversationLiveish(form)
    && hydrateConversationFromBootstrapSnapshot(context, cachedSettledSnapshot)
  ) {
    logExecutorDebug('switch-conversation-hydrate-settled-cache', {
      conversationId: targetID,
      cachedTurnCount: Array.isArray(cachedSettledSnapshot?.turns) ? cachedSettledSnapshot.turns.length : 0
    });
    await refreshGoalFeed(targetID);
    publishActiveConversation(targetID, context);
    return;
  }
  logExecutorDebug('switch-conversation-run', {
    conversationId: targetID,
    currentId: currentID,
    lastConversationId: lastConversationID,
    hasSettledTranscript,
    terminalHydrationPending,
    initialSubmitPending,
    switchingConversationId: String(chatState.switchingConversationID || '').trim()
  });
  clearFeedStateForConversation(resolveFeedResetConversationId(chatState, currentID));
  if (currentID !== targetID) {
    chatState.switchingConversationID = targetID;
    disconnectStream(context);
    // A fresh submit already populated the shared store with the local user
    // entity. The route-mounted replacement context must adopt that live
    // state rather than blanking it before the first server transcript exists.
    if (!hasPendingConversationBootstrap(targetID)) {
      messagesDS.setCollection?.([]);
      messagesDS.setError?.('');
      resetConversationSnapshotState(context);
    }
  }
  let existing;
  try {
    existing = await fetchConversation(targetID);
  } catch (err) {
    if (
      isCurrentRequest()
      && String(chatState.switchingConversationID || '').trim() === targetID
    ) {
      chatState.switchingConversationID = '';
    }
    throw err;
  }
  if (!isCurrentRequest()) return;
  if (!existing) {
    chatState.switchingConversationID = '';
    await createNewConversation(context);
    return;
  }
  const staleConversationState = String(chatState.lastConversationID || '').trim() !== targetID;
  if (currentID === targetID) {
    chatState.switchingConversationID = '';
    if (staleConversationState && !hasPendingConversationBootstrap(targetID)) {
      messagesDS.setCollection?.([]);
      messagesDS.setError?.('');
      resetConversationSnapshotState(context);
    }
    conversationsDS.setFormData?.({
      values: applyConversationFormSnapshot(form, existing)
    });
    const conversationLiveish = isConversationLiveish(existing);
    const initialTransportActive = syncConversationTransport(context, targetID);
    const snapshot = await dsTick(context, {
      conversationID: targetID,
      transcript: {
        includeExecutionDetails: true,
      },
      reason: conversationLiveish ? 'late-join' : 'poll',
    });
    if (!isCurrentRequest()) return;
    if ((snapshot?.hasRunning || conversationLiveish) && !initialTransportActive) {
      syncConversationTransport(context, targetID);
    } else {
      if (!initialTransportActive) {
        disconnectStream(context);
      }
    }
    publishActiveConversation(targetID, context);
    void refreshGoalFeed(targetID);
    return;
  }

  conversationsDS.setFormData?.({
    values: applyConversationFormSnapshot(form, existing)
  });
  chatState.switchingConversationID = '';
  const conversationLiveish = isConversationLiveish(existing);
  const initialTransportActive = syncConversationTransport(context, targetID);
  const snapshot = await dsTick(context, {
    conversationID: targetID,
    transcript: {
      includeExecutionDetails: true,
    },
    reason: conversationLiveish ? 'late-join' : 'poll',
  });
  if (!isCurrentRequest()) return;
  if ((snapshot?.hasRunning || conversationLiveish) && !initialTransportActive) {
    syncConversationTransport(context, targetID);
  } else {
    if (!initialTransportActive) {
      disconnectStream(context);
    }
  }
  publishActiveConversation(targetID, context);
  void refreshGoalFeed(targetID);
}

export function enqueueConversationSwitch(context, conversationID = '') {
  const chatState = ensureContextResources(context);
  const targetID = String(conversationID || '').trim();
  if (!targetID) return Promise.resolve();
  // History clicks, route bootstrap, and the sidebar can all announce the
  // same selection. Reuse the in-flight request instead of starting another
  // fetch/transcript hydration pass for the same conversation.
  if (
    chatState.switchQueue
    && String(chatState.switchQueueTarget || '').trim() === targetID
  ) {
    return chatState.switchQueue;
  }
  chatState.requestedConversationID = targetID;
  const request = switchConversation(context, targetID);
  chatState.switchQueueTarget = targetID;
  const settled = request.finally(() => {
    if (String(chatState.switchQueueTarget || '').trim() === targetID) {
      chatState.switchQueue = null;
      chatState.switchQueueTarget = '';
    }
  });
  chatState.switchQueue = settled;
  return settled;
}

export function bootstrapConversationSelection(context) {
  const windowId = getContextWindowId(context);
  const win = getWindowById(windowId);
  const explicitWindowConversationID = typeof window !== 'undefined'
    ? (
      String(win?.parameters?.conversations?.form?.id || '').trim()
      || String(win?.parameters?.conversations?.input?.parameters?.id || '').trim()
      || String(win?.parameters?.conversations?.input?.path?.id || '').trim()
      || String(win?.parameters?.conversations?.input?.id || '').trim()
      || String(win?.parameters?.conversationId || '').trim()
      || String(win?.parameters?.messages?.input?.parameters?.convID || '').trim()
      || String(win?.parameters?.messages?.input?.path?.convID || '').trim()
      || String(win?.parameters?.messages?.input?.convID || '').trim()
      || getScopedConversationSelection(windowId)
    )
    : '';
  const routeConversationID = typeof window !== 'undefined'
    ? conversationIDFromPath(window.location.pathname)
    : '';
  const bootstrapID = (
    routeConversationID && (isMainChatWindowId(windowId) || !String(explicitWindowConversationID || '').trim())
      ? routeConversationID
      : explicitWindowConversationID
  );
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
    const currentConversationID = String(getCurrentConversationID(context) || '').trim();
    const pendingInitialSubmitConversationID = String(chatState.pendingInitialSubmitConversationID || '').trim();
    if (currentConversationID && currentConversationID === id && pendingInitialSubmitConversationID === id) {
      return;
    }
    void enqueueConversationSwitch(context, id).catch((err) => {
      logExecutorDebug('conversation-switch-failed', {
        conversationId: id,
        error: String(err?.message || err || 'conversation switch failed')
      });
      context?.Context?.('messages')?.handlers?.dataSource?.setError?.(String(err?.message || err));
    });
  };
  chatState.onNewConversation = (event) => {
    const targetWindowId = String(event?.detail?.windowId || '').trim();
    if (targetWindowId && targetWindowId !== currentWindowId) return;
    const createdConversationID = String(event?.detail?.id || '').trim();
    if (createdConversationID) return;
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
  const chatState = ensureContextResources(context);
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  if (!conversationsDS) return false;
  const currentForm = conversationsDS.peekFormData?.() || {};
  // New chat can originate from the composer, Report Builder, or another
  // hosted workspace surface. Clear conversation-owned windows here as the
  // common boundary so a direct caller cannot leave the old report mounted.
  clearWorkspaceWindowsForNewConversation();
  clearFeedStateForConversation(resolveFeedResetConversationId(chatState, String(currentForm?.id || '').trim()));
  clearPendingConversationBootstrap(String(currentForm?.id || chatState.activeConversationID || '').trim());
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
  chatState.requestedConversationID = '';
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
    const starterTaskCategories = resolveStarterTaskCategories({
      agentInfos: Array.isArray(metaForm?.agentInfos) ? metaForm.agentInfos : [],
      selectedAgent: preferredAgent
    });
    metaDS.setFormData?.({
      values: {
        ...metaForm,
        agent: preferredAgent || metaForm?.agent || '',
        starterTasks,
        starterTaskCategories
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
    const switchingID = String(chatState.switchingConversationID || '').trim();
    // Don't enqueue another switch while one is already in flight; otherwise a
    // route-driven switch (Root.jsx) and this scoped-selection poller can keep
    // re-queuing opposite targets and ping-pong between conversations. The
    // in-flight switch always clears switchingConversationID before returning,
    // so the next tick re-evaluates against the settled state.
    if (desiredID && desiredID !== currentID && !switchingID) {
      void enqueueConversationSwitch(context, desiredID).catch((err) => {
        logExecutorDebug('conversation-switch-poll-failed', {
          conversationId: desiredID,
          error: String(err?.message || err || 'conversation switch failed')
        });
      });
      return;
    }
    const streamIsHot = !!chatState.stream
      && (Date.now() - Number(chatState.lastStreamEventAt || 0) < 6000);
    if (streamIsHot) return;
    if (shouldDeferTranscriptToLiveStream(context, getCurrentConversationID(context))) return;
    const pendingTerminalHydrationConversationID = String(chatState.pendingTerminalHydrationConversationID || '').trim();
    if (pendingTerminalHydrationConversationID && pendingTerminalHydrationConversationID === currentID) return;
    const hasFinishedSnapshot = (_chatStoreRef()?.getProjection?.(currentID) || []).length > 0 && !chatState.lastHasRunning;
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
  disconnectStream(context);
}

export function rememberSeedTitle(conversationID, query) {
  rememberConversationSeedTitle(conversationID, query);
}
