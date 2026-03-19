import { classifyMessage, normalizeMessages } from './messageNormalizer';
import { setStage } from './stageBus';
import { client } from './agentlyClient';
import {
  applyIterationVisibility,
  bindConversationWindowEvents,
  bootstrapConversationSelection,
  createNewConversation,
  dsTick,
  disconnectStream,
  ensureContextResources,
  ensureConversation,
  fetchConversation,
  fetchPendingElicitations,
  getVisibleIterations,
  hydrateMeta,
  logStreamDebug,
  mapTranscriptToRows,
  normalizeMetaResponse,
  publishActiveConversation,
  rememberSeedTitle,
  resolveUserID,
  sanitizeAutoSelection,
  syncConversationTransport,
  startPolling,
  stopPolling,
  syncMessagesSnapshot,
  unbindConversationWindowEvents
} from './chatRuntime';
import IterationBlock from '../components/chat/IterationBlock';
import IterationPaginator from '../components/chat/IterationPaginator';
import BubbleMessage from '../components/chat/BubbleMessage';
import ElicitationForm from '../components/chat/ElicitationForm';
import SteerQueue from '../components/chat/SteerQueue';
import { composerPresentation } from './composerPresentation';

const DEFAULT_VISIBLE_ITERATIONS = Number.MAX_SAFE_INTEGER;
const ITERATION_STEP = 1;
const pendingUploads = [];

function normalizeUploadItems(raw = null) {
  let list = raw;
  if (list && !Array.isArray(list)) {
    if (Array.isArray(list?.files)) {
      list = list.files;
    } else if (Array.isArray(list?.data)) {
      list = list.data;
    } else {
      list = [list];
    }
  }
  if (!Array.isArray(list)) return [];
  return list.map((item) => {
    const src = item?.data || item || {};
    const uri = String(src?.uri || src?.url || src?.path || src?.href || '').trim();
    const name = String(src?.name || (uri ? uri.split('/').pop() : '')).trim();
    const mime = String(src?.mime || src?.type || src?.contentType || '').trim();
    const stagingFolder = String(src?.stagingFolder || src?.folder || src?.staging || src?.dir || '').trim();
    const content = typeof src?.content === 'string' ? src.content : '';
    const data = Array.isArray(src?.data) || typeof src?.data === 'string' ? src.data : undefined;
    const size = Number(src?.size || src?.length || src?.bytes || 0) || undefined;
    const normalized = {
      name: name || undefined,
      mime: mime || undefined,
      stagingFolder: stagingFolder || undefined,
      uri: uri || undefined,
      content: content || undefined,
      data,
      size
    };
    return normalized;
  }).filter((item) => !!(item.uri || item.content || item.data));
}

function mergeAttachments(primary = [], secondary = []) {
  const out = [];
  const seen = new Set();
  for (const list of [primary, secondary]) {
    for (const item of list) {
      if (!item || typeof item !== 'object') continue;
      const key = JSON.stringify({
        uri: item.uri || '',
        name: item.name || '',
        mime: item.mime || '',
        stagingFolder: item.stagingFolder || '',
        content: item.content || '',
        hasData: item.data != null
      });
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(item);
    }
  }
  return out;
}

function matchesAgentIdentity(entry, selectedAgent) {
  const target = String(selectedAgent || '').trim();
  if (!target) return false;
  const value = String(entry?.value || entry?.id || '').trim();
  const label = String(entry?.label || entry?.name || entry?.title || '').trim();
  return value === target || label === target;
}

export async function onInit({ context }) {
  setStage({ phase: 'waiting', text: 'Initializing…' });
  try {
    bindConversationWindowEvents(context);
    await hydrateMeta(context);
    bootstrapConversationSelection(context);
    const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
    const messagesDS = context?.Context?.('messages')?.handlers?.dataSource;
    const conversationID = String(conversationsDS?.peekFormData?.()?.id || '').trim();
    if (conversationID) {
      const existing = await fetchConversation(conversationID);
      if (!existing) {
        const metaDefaults = context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.()?.defaults || {};
        conversationsDS?.setFormData?.({
          values: {
            ...(conversationsDS?.peekFormData?.() || {}),
            id: '',
            title: 'New conversation',
            agent: metaDefaults?.agent || '',
            model: metaDefaults?.model || '',
            embedder: metaDefaults?.embedder || ''
          }
        });
        messagesDS?.setCollection?.([]);
        messagesDS?.setError?.('');
        publishActiveConversation('', context);
      } else {
        syncConversationTransport(context, conversationID);
        await dsTick(context, { conversationID });
        publishActiveConversation(conversationID, context);
      }
    }
    setStage({ phase: 'ready', text: 'Ready' });
  } catch (err) {
    setStage({ phase: 'error', text: String(err?.message || err || 'Initialization failed') });
    context?.Context?.('messages')?.handlers?.dataSource?.setError?.(String(err?.message || err));
  }
  startPolling(context);
}

export function onDestroy({ context }) {
  stopPolling(context);
  unbindConversationWindowEvents(context);
  setStage({ phase: 'ready', text: 'Ready' });
}

export async function onFetchMeta({ context, data, result }) {
  const payload = normalizeMetaResponse(data || result || {});
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  if (metaDS) {
    metaDS.setFormData?.({ values: payload });
  }
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
  return [payload];
}

export async function onFetchMessages({ context, data, result }) {
  const turns = Array.isArray(data) ? data : (Array.isArray(result?.data) ? result.data : []);
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const conversationID = String(conversationsDS?.peekFormData?.()?.id || '').trim();
  const pendingElicitations = conversationID ? await fetchPendingElicitations(conversationID) : [];
  return syncMessagesSnapshot(context, turns, 'fetch', pendingElicitations);
}

export async function loadOlderExecutions({ context, all = false, reset = false } = {}) {
  const chatState = ensureContextResources(context);
  chatState.iterationVisibleCount = DEFAULT_VISIBLE_ITERATIONS;
  if (applyIterationVisibility(context)) {
    return true;
  }
  await dsTick(context);
  return true;
}

export function onFetchQueuedTurns({ context, data }) {
  const turns = Array.isArray(data) ? data : [];
  const queuedTurns = mapTranscriptToRows(turns).queuedTurns;
  const queueDS = context?.Context?.('queueTurns')?.handlers?.dataSource;
  queueDS?.setCollection?.(queuedTurns);
  return queuedTurns;
}

export async function submitMessage({ context, message, model, agent }) {
  const query = typeof message === 'string'
    ? message.trim()
    : String(message?.content || message?.text || message?.value || '').trim();
  if (!query) return;
  setStage({ phase: 'thinking', text: 'Assistant thinking…' });
  const convDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const metaForm = context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.() || {};
  const selectedAgent = sanitizeAutoSelection(agent || '');
  const selectedModel = sanitizeAutoSelection(model || '');
  const defaultModel = sanitizeAutoSelection(metaForm?.defaults?.model || metaForm?.defaultModel || '');
  const preferredAgentModel = (() => {
    if (!selectedAgent) return '';
    const options = Array.isArray(metaForm?.agentOptions) ? metaForm.agentOptions : [];
    const selectedOption = options.find((opt) => matchesAgentIdentity(opt, selectedAgent));
    if (selectedOption?.modelRef || selectedOption?.model) {
      return sanitizeAutoSelection(selectedOption?.modelRef || selectedOption?.model || '');
    }
    const mappedAgent = metaForm?.agentInfo?.[selectedAgent] || null;
    if (mappedAgent?.modelRef || mappedAgent?.model) {
      return sanitizeAutoSelection(mappedAgent?.modelRef || mappedAgent?.model || '');
    }
    const listedAgents = Array.isArray(metaForm?.agentInfos) ? metaForm.agentInfos : [];
    const listedAgent = listedAgents.find((entry) => matchesAgentIdentity(entry, selectedAgent));
    return sanitizeAutoSelection(listedAgent?.modelRef || listedAgent?.model || '');
  })();
  const effectiveModel = (() => {
    // Treat the workspace default model as implicit. When the user has not
    // actually changed the model away from the default, the selected agent's
    // preferred model should still win.
    const selectedIsImplicitDefault = !!selectedModel && !!defaultModel && selectedModel === defaultModel;
    if (preferredAgentModel && (!selectedModel || selectedIsImplicitDefault)) {
      return preferredAgentModel;
    }
    return selectedModel;
  })();
  const conversationID = await ensureConversation(context, { agent: selectedAgent, model: effectiveModel || selectedModel });
  rememberSeedTitle(conversationID, query);
  const convForm = convDS?.peekFormData?.() || {};

  const messageAttachments = normalizeUploadItems(message?.attachments || message?.files || []);
  const mergedAttachments = mergeAttachments(messageAttachments, pendingUploads);
  const payload = {
    conversationId: conversationID,
    query,
    agentId: selectedAgent || sanitizeAutoSelection(convForm?.agent || metaForm?.defaults?.agent || ''),
    model: effectiveModel || sanitizeAutoSelection(convForm?.model || ''),
    tools: Array.isArray(metaForm?.tool) ? metaForm.tool : undefined,
    reasoningEffort: metaForm?.reasoningEffort || undefined,
    attachments: mergedAttachments.length > 0 ? mergedAttachments : undefined
  };
  const resolvedUserID = resolveUserID(context);
  if (resolvedUserID) {
    payload.userId = resolvedUserID;
  }

  const chatState = ensureContextResources(context);
  const queueingDuringActiveTurn = !!chatState?.runningTurnId || !!chatState?.lastHasRunning;
  const submittedAt = Date.now();
  chatState.activeConversationID = conversationID;
  chatState.liveOwnedConversationID = conversationID;
  logStreamDebug(chatState, 'submit-start', {
    conversationId: conversationID,
    agentId: String(payload.agentId || '').trim(),
    model: String(payload.model || '').trim(),
    queueingDuringActiveTurn,
    attachmentCount: Array.isArray(payload.attachments) ? payload.attachments.length : 0,
    queryChars: query.length
  });
  if (!queueingDuringActiveTurn) {
    chatState.activeStreamPrompt = query;
    chatState.activeStreamTurnId = '';
    chatState.activeStreamStartedAt = submittedAt;
    const nextConvForm = convDS?.peekFormData?.() || {};
    convDS?.setFormData?.({
      values: {
        ...nextConvForm,
        running: true
      }
    });
    syncConversationTransport(context, conversationID);
    setStage({ phase: 'executing', text: 'Executing…' });
  } else {
    setStage({ phase: 'waiting', text: 'Queued follow-up…' });
  }
  const queryResult = await client.query(payload);
  logStreamDebug(chatState, 'submit-query-response', {
    conversationId: conversationID,
    hasInlineContent: String(queryResult?.content || '').trim() !== '',
    resultKeys: queryResult && typeof queryResult === 'object' ? Object.keys(queryResult).length : 0
  });
  pendingUploads.length = 0;
  await new Promise((resolve) => setTimeout(resolve, queueingDuringActiveTurn ? 140 : 80));
  await dsTick(context, { conversationID });
  logStreamDebug(chatState, 'submit-post-dstick', {
    conversationId: conversationID,
    queueingDuringActiveTurn
  });
}

export async function abortConversation({ context }) {
  const conversationID = getConversationID(context);
  if (!conversationID) return false;
  const chatState = ensureContextResources(context);
  const activeTurnID = String(chatState.runningTurnId || chatState.activeStreamTurnId || '').trim();
  if (activeTurnID) {
    await client.cancelTurn(activeTurnID);
  } else {
    await client.terminateConversation(conversationID);
  }
  await dsTick(context);
  setStage({ phase: 'done', text: activeTurnID ? 'Cancel requested' : 'Terminated' });
  return true;
}

export async function cancelQueuedTurnByID({ context, conversationID, turnID }) {
  if (!conversationID || !turnID) return;
  await client.cancelQueuedTurn(conversationID, turnID);
  await dsTick(context);
}

export async function moveQueuedTurn({ context, conversationID, turnID, direction }) {
  if (!conversationID || !turnID) return;
  await client.moveQueuedTurn(conversationID, turnID, { direction });
  await dsTick(context);
}

export async function editQueuedTurn({ context, conversationID, turnID, content }) {
  if (!conversationID || !turnID) return;
  await client.editQueuedTurn(conversationID, turnID, { content });
  await dsTick(context);
}

export async function steerTurn({ context, conversationID, turnID, content }) {
  if (!conversationID || !turnID) return;
  await client.steerTurn(conversationID, turnID, { content, role: 'user' });
  await dsTick(context);
}

export async function forceSteerQueuedTurn({ context, conversationID, turnID }) {
  if (!conversationID || !turnID) return;
  await client.forceSteerQueuedTurn(conversationID, turnID);
  await dsTick(context);
}

function getConversationID(context) {
  const form = context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.() || {};
  return String(form?.id || '').trim();
}

function getQueueSelection(context) {
  const queueDS = context?.Context?.('queueTurns')?.handlers?.dataSource;
  const selected = queueDS?.peekSelection?.();
  return Array.isArray(selected) ? selected[0] : selected;
}

export function debugMessagesError({ context, error }) {
  context?.Context?.('messages')?.handlers?.dataSource?.setError?.(String(error?.message || error || 'messages fetch failed'));
}

export async function newConversation({ context }) {
  return createNewConversation(context);
}

export async function compactConversation({ context }) {
  const id = getConversationID(context);
  if (!id) return false;
  await client.compactConversation(id);
  await dsTick(context);
  return true;
}

export function compactReadonly({ context }) {
  const meta = context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.() || {};
  return !getConversationID(context) || !meta?.capabilities?.compactConversation;
}

export async function pruneConversation({ context }) {
  const id = getConversationID(context);
  if (!id) return false;
  await client.pruneConversation(id);
  await dsTick(context);
  return true;
}

export function pruneReadonly(args) {
  const meta = args?.context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.() || {};
  return compactReadonly(args) || !meta?.capabilities?.pruneConversation;
}

export function hasAgentChains() {
  return false;
}

export function showHeaderChainStatus() {
  return false;
}

export function toggleChains() {
  return false;
}

export async function updateVisibility({ context, value }) {
  const id = getConversationID(context);
  if (!id) return false;
  try {
    await client.updateConversation(id, { visibility: value || 'private' });
  } catch (_) {
    return false;
  }
  return true;
}

export function onSettings() {
  return true;
}

export async function onUpload(props = {}) {
  const { context } = props;
  const exec = props?.execution || {};
  const result = props?.result;
  const files = props?.files;
  const data = props?.data;
  const normalized = normalizeUploadItems(exec.result || exec.output || exec.data || result || files || data);
  if (normalized.length > 0) {
    const next = mergeAttachments(pendingUploads, normalized);
    pendingUploads.length = 0;
    pendingUploads.push(...next);
  }
  await dsTick(context);
  return true;
}

export function saveSettings() {
  return true;
}

export function selectAgent({ context, value }) {
  const ds = context?.Context?.('conversations')?.handlers?.dataSource;
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  const meta = metaDS?.peekFormData?.() || {};
  if (!ds) return false;
  const form = ds.peekFormData?.() || {};
  const nextAgent = value || '';
  const next = { ...form, agent: nextAgent };
  const options = Array.isArray(meta?.agentOptions) ? meta.agentOptions : [];
  const selected = options.find((opt) => matchesAgentIdentity(opt, nextAgent));
  const preferredModel = sanitizeAutoSelection(selected?.modelRef || '');
  const nextMeta = { ...meta, agent: nextAgent };
  if (preferredModel) {
    next.model = preferredModel;
    nextMeta.model = preferredModel;
  } else if (!nextAgent) {
    next.model = meta?.defaults?.model || '';
    nextMeta.model = meta?.defaults?.model || '';
  }
  ds.setFormData?.({ values: next });
  metaDS?.setFormData?.({ values: nextMeta });
  return true;
}

export function selectModel({ context, value }) {
  const ds = context?.Context?.('conversations')?.handlers?.dataSource;
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  if (!ds) return false;
  const form = ds.peekFormData?.() || {};
  const nextModel = value || '';
  ds.setFormData?.({ values: { ...form, model: nextModel } });
  const meta = metaDS?.peekFormData?.() || {};
  metaDS?.setFormData?.({ values: { ...meta, model: nextModel } });
  return true;
}

export async function moveQueuedTurnUp({ context }) {
  const conversationID = getConversationID(context);
  const selected = getQueueSelection(context);
  const turnID = String(selected?.id || selected?.Id || '').trim();
  if (!conversationID || !turnID) return false;
  await moveQueuedTurn({ context, conversationID, turnID, direction: 'up' });
  return true;
}

export async function moveQueuedTurnDown({ context }) {
  const conversationID = getConversationID(context);
  const selected = getQueueSelection(context);
  const turnID = String(selected?.id || selected?.Id || '').trim();
  if (!conversationID || !turnID) return false;
  await moveQueuedTurn({ context, conversationID, turnID, direction: 'down' });
  return true;
}

export async function cancelQueuedTurn({ context }) {
  const conversationID = getConversationID(context);
  const selected = getQueueSelection(context);
  const turnID = String(selected?.id || selected?.Id || '').trim();
  if (!conversationID || !turnID) return false;
  await cancelQueuedTurnByID({ context, conversationID, turnID });
  return true;
}

export async function editQueuedTurnBySelection({ context, content }) {
  const conversationID = getConversationID(context);
  const selected = getQueueSelection(context);
  const turnID = String(selected?.id || selected?.Id || '').trim();
  if (!conversationID || !turnID) return false;
  await editQueuedTurn({ context, conversationID, turnID, content: content || '' });
  return true;
}

export function selectFolder() {
  return true;
}

export const chatService = {
  classifyMessage,
  normalizeMessages,
  composerPresentation,
  renderers: {
    bubble: BubbleMessage,
    form: ElicitationForm,
    elicition: ElicitationForm,
    iteration: IterationBlock,
    paginator: IterationPaginator,
    queue: SteerQueue
  },
  onInit,
  onDestroy,
  onFetchMeta,
  onFetchMessages,
  onFetchQueuedTurns,
  debugMessagesError,
  newConversation,
  compactConversation,
  compactReadonly,
  pruneConversation,
  pruneReadonly,
  updateVisibility,
  hasAgentChains,
  showHeaderChainStatus,
  toggleChains,
  submitMessage,
  onSettings,
  onUpload,
  saveSettings,
  selectAgent,
  selectModel,
  loadOlderExecutions,
  abortConversation,
  moveQueuedTurnUp,
  moveQueuedTurnDown,
  cancelQueuedTurn,
  editQueuedTurn,
  cancelQueuedTurnByID,
  moveQueuedTurn,
  editQueuedTurnBySelection,
  steerTurn,
  forceSteerQueuedTurn,
  selectFolder
};
