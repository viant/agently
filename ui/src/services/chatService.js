import { classifyMessage, normalizeMessages } from './messageNormalizer';
import { setStage } from './stageBus';
import { client } from './agentlyClient';
import { showToast } from './httpClient';
import { getFeedData, updateFeedData } from './toolFeedBus';
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
  renderMergedRowsForContext,
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
import StarterTasks from '../components/chat/StarterTasks';
import SteerQueue from '../components/chat/SteerQueue';
import { composerPresentation } from './composerPresentation';
import { openCodeDiffDialog, openFileViewDialog, updateCodeDiffDialog, updateFileViewDialog } from '../utils/dialogBus';

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

function getPersistedSelectedAgent() {
  try {
    return String(localStorage.getItem('agently.selectedAgent') || '').trim();
  } catch (_) {
    return '';
  }
}

export function resolveSubmitAgent({ selectedAgent = '', persistedAgent = '', metaForm = {}, convForm = {} } = {}) {
  return sanitizeAutoSelection(
    selectedAgent
    || persistedAgent
    || metaForm?.agent
    || convForm?.agent
    || metaForm?.defaults?.agent
    || ''
  );
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

function mergeConversationSnapshot(current = {}, conversation = null) {
  if (!conversation || typeof conversation !== 'object') return { ...current };
  const next = { ...current };
  const id = String(conversation?.id || conversation?.Id || '').trim();
  const title = String(conversation?.title || conversation?.Title || '').trim();
  const agent = String(conversation?.agentId || conversation?.AgentId || '').trim();
  const model = String(conversation?.defaultModel || conversation?.DefaultModel || '').trim();
  const embedder = String(conversation?.defaultEmbedder || conversation?.DefaultEmbedder || '').trim();
  if (id) next.id = id;
  if (title) next.title = title;
  if (agent) next.agent = agent;
  if (model) next.model = model;
  if (embedder) next.embedder = embedder;
  return next;
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
        conversationsDS?.setFormData?.({
          values: mergeConversationSnapshot(conversationsDS?.peekFormData?.() || {}, existing)
        });
        syncConversationTransport(context, conversationID);
        await dsTick(context, { conversationID });
        publishActiveConversation(conversationID, context);
      }
    }
    renderMergedRowsForContext(context);
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
  renderMergedRowsForContext(context);
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
  const persistedAgent = sanitizeAutoSelection(getPersistedSelectedAgent());
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
    agentId: resolveSubmitAgent({ selectedAgent, persistedAgent, metaForm, convForm }),
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
  if (typeof window !== 'undefined') {
    try {
      const raw = String(window.localStorage?.getItem('agently.debugExecutor') || '').trim().toLowerCase();
      if (raw === '1' || raw === 'true' || raw === 'on') {
        console.log('[agently-executor]', {
          event: 'abort-requested',
          ts: new Date().toISOString(),
          conversationId: conversationID,
          activeTurnId: activeTurnID,
          action: activeTurnID ? 'cancelTurn' : 'terminateConversation'
        });
      }
    } catch (_) {}
  }
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
  // Persist selection so new conversations inherit it.
  try { localStorage.setItem('agently.selectedAgent', nextAgent); } catch (_) {}
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

function readSelection(context, dataSourceRef = '') {
  const ds = dataSourceRef
    ? context?.Context?.(dataSourceRef)?.handlers?.dataSource
    : context?.handlers?.dataSource;
  return ds?.peekSelection?.()?.selected || ds?.getSelection?.()?.selected || null;
}

function readForm(context, dataSourceRef = '') {
  const ds = dataSourceRef
    ? context?.Context?.(dataSourceRef)?.handlers?.dataSource
    : context?.handlers?.dataSource;
  return ds?.peekFormData?.() || ds?.getFormData?.() || {};
}

function guessIsDirectory(entry = {}) {
  if (entry?.isFolder === true) return true;
  const path = String(entry?.path || entry?.Path || entry?.uri || entry?.URI || '').trim();
  if (!path) return false;
  const base = path.split('/').pop() || '';
  return !base.includes('.');
}

function normalizeToolResult(raw) {
  if (raw == null) return null;
  if (typeof raw === 'string') {
    const text = raw.trim();
    if (!text) return null;
    try {
      return JSON.parse(text);
    } catch (_) {
      return raw;
    }
  }
  return raw;
}

function normalizePath(value = '') {
  return String(value || '').replace(/\\/g, '/').trim();
}

async function fetchWorkspaceText(uri = '') {
  const value = String(uri || '').trim();
  if (!value) return '';
  try {
    return await client.downloadWorkspaceFile(value);
  } catch (_) {
    return '';
  }
}

export function explorerOpenIcon() {
  return 'document-open';
}

export async function explorerOpen(props = {}) {
  const row = props?.row || props?.item || props?.node || readSelection(props?.context, 'results') || {};
  const uri = String(row?.uri || row?.URI || row?.path || row?.Path || '').trim();
  if (!uri) return false;
  return explorerRead({ ...props, uri });
}

export async function explorerRead(props = {}) {
  const uri = String(props?.uri || '').trim()
    || String(readSelection(props?.context, 'results')?.uri || readSelection(props?.context, 'results')?.URI || '').trim();
  if (!uri) {
    showToast('Select a file to read.', { intent: 'warning' });
    return false;
  }
  const title = uri.split('/').pop() || uri;
  openFileViewDialog({ title, uri, loading: true, content: '' });
  try {
    const result = normalizeToolResult(await client.executeTool('resources-read', { uri, maxBytes: 200000 }));
    const content = String(result?.content ?? result?.Content ?? result ?? '');
    updateFileViewDialog({ title, uri, loading: false, content });
    return true;
  } catch (err) {
    updateFileViewDialog({ loading: false, content: String(err?.message || err || 'Failed to read file') });
    return false;
  }
}

export async function explorerSearch(props = {}) {
  const context = props?.context;
  const selected = readSelection(context, 'results') || {};
  const form = readForm(context, 'search');
  const pattern = String(
    props?.value ?? props?.pattern ?? form?.pattern ?? form?.query ?? ''
  ).trim();
  const path = String(form?.path || selected?.path || selected?.Path || '').trim();
  const rootId = String(form?.rootId || form?.rootID || selected?.rootId || selected?.rootID || '').trim();

  if (!pattern) {
    if (!props?.silent) {
      showToast('Explorer search needs a pattern from the feed input.', { intent: 'warning' });
    }
    return false;
  }
  if (!path) {
    if (!props?.silent) {
      showToast('Select a file or directory to search.', { intent: 'warning' });
    }
    return false;
  }

  const args = {
    path,
    pattern,
    recursive: guessIsDirectory(selected),
    mode: 'match',
    maxFiles: 20,
    maxBlocks: 40,
    lines: 24,
    bytes: 512,
  };
  const explorerConversationId = String(context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.()?.id || '').trim();
  const explorerFeed = getFeedData('explorer', explorerConversationId);
  const feedRoots = Array.isArray(explorerFeed?.data?.output?.roots)
    ? explorerFeed.data.output.roots
    : [];
  const matchingRoot = rootId
    ? feedRoots.find((entry) => String(entry?.id || '').trim() === rootId)
    : null;
  if (matchingRoot?.uri) {
    args.root = String(matchingRoot.uri || '').trim();
  } else if (rootId) {
    args.rootId = rootId;
  }

  try {
    const result = normalizeToolResult(await client.executeTool('resources-grepFiles', args));
    const existing = readSelection(context)?.feedId ? null : null;
    void existing;
    const files = Array.isArray(result?.files) ? result.files : [];
    try {
      const resultsDS = context?.Context?.('results')?.handlers?.dataSource;
      resultsDS?.setCollection?.(files);
      resultsDS?.setPage?.(1);
      resultsDS?.setSelection?.({ selected: null, rowIndex: -1 });
    } catch (_) {}
    updateFeedData('explorer', {
      data: {
        input: args,
        output: result,
      },
    }, explorerConversationId);
    const fileCount = files.length;
    showToast(fileCount > 0 ? `Explorer search returned ${fileCount} file(s).` : 'Explorer search returned no results.', {
      intent: fileCount > 0 ? 'success' : 'warning',
      ttlMs: 2600,
    });
    return true;
  } catch (err) {
    showToast(String(err?.message || err || 'Explorer search failed'), { intent: 'danger' });
    return false;
  }
}

let explorerSearchInputTimer = null;

export function explorerSearchInputChanged(props = {}) {
  const value = String(props?.value ?? '').trim();
  if (explorerSearchInputTimer) {
    clearTimeout(explorerSearchInputTimer);
    explorerSearchInputTimer = null;
  }
  explorerSearchInputTimer = setTimeout(() => {
    explorerSearch({ ...props, value, silent: true }).catch(() => {});
  }, 250);
  return true;
}

export function taskStatusIcon(props = {}) {
  const statusRaw = props?.status ?? props?.row?.status ?? props?.row?.Status ?? '';
  const status = String(statusRaw || '').trim().toLowerCase();
  if (['completed', 'succeeded', 'done', 'accepted', 'success'].includes(status)) return 'tick';
  if (['in_progress', 'running', 'processing'].includes(status)) return 'play';
  if (['pending', 'open', 'queued', 'waiting'].includes(status)) return 'time';
  return 'dot';
}

export async function onChangedFileSelect(props = {}) {
  const record = props?.item || props?.node || props?.row || {};
  const uri = String(props?.uri || record?.uri || record?.url || record?.path || '').trim();
  const prevUri = String(props?.origUri || record?.origUri || record?.origUrl || '').trim();
  const diff = String(props?.diff || record?.diff || '').trim();
  const title = uri.split('/').pop() || prevUri.split('/').pop() || 'Changed File';
  openCodeDiffDialog({ title, loading: true, currentUri: uri, prevUri, hasPrev: !!prevUri });
  try {
    const [current, prev] = await Promise.all([
      fetchWorkspaceText(uri),
      prevUri ? fetchWorkspaceText(prevUri) : Promise.resolve(''),
    ]);
    updateCodeDiffDialog({
      current,
      prev,
      diff,
      hasPrev: !!prev,
      loading: false,
    });
  } catch (err) {
    updateCodeDiffDialog({
      current: '',
      prev: '',
      diff: String(err?.message || err || diff),
      hasPrev: false,
      loading: false,
    });
  }
  return true;
}

export function prepareChangeFiles(props = {}) {
  const collection = Array.isArray(props?.collection) ? props.collection : [];
  const context = props?.context;
  const parentRef = String(context?.handlers?.dataSource?.dataSourceRef || '').trim();
  const form = readForm(context, parentRef);
  const workdir = normalizePath(form?.workdir || '');

  const nodesByPath = new Map();
  const ensureFolder = (parts) => {
    let acc = '';
    for (const part of parts) {
      acc = acc ? `${acc}/${part}` : part;
      if (!nodesByPath.has(acc)) {
        nodesByPath.set(acc, {
          uri: `/${acc}`,
          name: part,
          isFolder: true,
          isExpanded: true,
          icon: 'folder-open',
          childNodes: [],
          parentPath: acc.includes('/') ? acc.slice(0, acc.lastIndexOf('/')) : '',
        });
      }
    }
  };
  const relativize = (value) => {
    const normalized = normalizePath(value);
    if (!normalized) return '';
    if (workdir && (normalized === workdir || normalized.startsWith(`${workdir}/`))) {
      return normalized.slice(workdir.length).replace(/^\/+/, '');
    }
    return normalized.replace(/^\/+/, '');
  };
  for (const item of collection) {
    const rel = relativize(item?.url || item?.uri);
    if (!rel) continue;
    const parts = rel.split('/').filter(Boolean);
    if (!parts.length) continue;
    const fileName = parts[parts.length - 1];
    const folderParts = parts.slice(0, -1);
    if (folderParts.length) ensureFolder(folderParts);
    const parentPath = folderParts.join('/');
    const key = parentPath ? `${parentPath}/${fileName}` : fileName;
    if (!nodesByPath.has(key)) {
      nodesByPath.set(key, {
        uri: `/${key}`,
        name: fileName,
        isFolder: false,
        childNodes: [],
        parentPath,
        kind: item?.kind,
        diff: item?.diff,
        uriRaw: normalizePath(item?.url || item?.uri),
        uri: normalizePath(item?.url || item?.uri),
        origUri: normalizePath(item?.origUrl || item?.origUri),
      });
    }
  }
  for (const node of nodesByPath.values()) {
    if (node.parentPath && nodesByPath.has(node.parentPath)) {
      nodesByPath.get(node.parentPath).childNodes.push(node);
    }
  }
  const sortChildren = (items) => items.sort((left, right) => {
    if (!!left.isFolder !== !!right.isFolder) return left.isFolder ? -1 : 1;
    return String(left.name || '').localeCompare(String(right.name || ''));
  });
  for (const node of nodesByPath.values()) {
    if (Array.isArray(node.childNodes) && node.childNodes.length) sortChildren(node.childNodes);
  }
  const roots = Array.from(nodesByPath.values()).filter((node) => !node.parentPath);
  sortChildren(roots);
  return roots;
}

export async function runPatchCommit() {
  try {
    await client.executeTool('system_patch-commit', {});
    showToast('Patch session committed.', { intent: 'success', ttlMs: 2200 });
    return true;
  } catch (err) {
    showToast(String(err?.message || err || 'Patch commit failed'), { intent: 'danger' });
    return false;
  }
}

export async function runPatchRollback() {
  try {
    await client.executeTool('system_patch-rollback', {});
    showToast('Patch session rolled back.', { intent: 'success', ttlMs: 2200 });
    return true;
  } catch (err) {
    showToast(String(err?.message || err || 'Patch rollback failed'), { intent: 'danger' });
    return false;
  }
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
    starter: StarterTasks,
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
  selectFolder,
  taskStatusIcon,
  onChangedFileSelect,
  prepareChangeFiles,
  runPatchCommit,
  runPatchRollback,
  explorerOpenIcon,
  explorerOpen,
  explorerRead,
  explorerSearch,
  explorerSearchInputChanged
};
