import React from 'react';
import { publishUIBridgeSnapshotNow } from 'forge/core';
import {
  normalizeString,
  normalizeBool,
  ensureStringArray,
  defaultAgentTools,
  defaultAgentModel,
  resolveCurrentModel,
} from '../../../../forge/src/components/chatCommandCenterHelpers.js';
import {
  applyAgentSelection,
  applyModelSelection,
  applyReasoningSelection,
  applyToolsSelection,
  applyAutoSelectToolsSelection,
} from '../../../../forge/src/components/chatCommandCenterActions.js';
import { classifyMessage, normalizeMessages } from './messageNormalizer';
import { setStage } from './stageBus';
import { client } from './agentlyClient';
import { showToast } from './httpClient';
import { getFeedData, updateFeedData } from './toolFeedBus';
import { buildWebQueryContext } from './clientContext';
import { resetLiveStreamState } from './liveStreamStore';
import {
  applyIterationVisibility,
  bindConversationWindowEvents,
  cacheSettledConversationBootstrapSnapshot,
  bootstrapConversationSelection,
  clearPendingConversationBootstrap,
  clearSettledConversationBootstrapSnapshot,
  createNewConversation,
  dsTick,
  disconnectStream,
  ensureContextResources,
  ensureConversation,
  fetchConversation,
  fetchPendingElicitations,
  getSettledConversationBootstrapSnapshot,
  getVisibleIterations,
  hasPendingConversationBootstrap,
  hydrateMeta,
  hydrateConversationFromBootstrapSnapshot,
  isConversationLiveish,
  logExecutorDebug,
  markPendingConversationBootstrap,
  logStreamDebug,
  mapTranscriptToRows,
  normalizeMetaResponse,
  publishActiveConversation,
  publishConversationMetaUpdated,
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
import StarterTasks from '../components/chat/StarterTasks';
import SteerQueue from '../components/chat/SteerQueue';
import NamedLookupInput from '../components/lookups/NamedLookupInput.jsx';
import { flattenStored } from '../components/lookups/tokens.js';
import { listLookupRegistry } from '../components/lookups/client.js';
import { composerPresentation } from './composerPresentation';
import { publishWorkspaceMetadataSnapshot } from './workspaceMetadata';
import { connectForgeUIActionsToCallbacksOrChat } from './forgeUIActions';
import { openCodeDiffDialog, openFileViewDialog, updateCodeDiffDialog, updateFileViewDialog } from '../utils/dialogBus';
import ChatFeedFromChatStore from '../components/chat/ChatFeedFromChatStore.jsx';
import { onTranscript as applyTranscriptToChatStore, reset as resetChatStoreConversation, submit as submitToChatStore, steer as steerToChatStore } from './chatStore.js';
import { conversationIDFromPath } from './chatRuntime';
import { getScopedConversationSelection, MAIN_CHAT_WINDOW_ID, openConversationInMainWindow } from './conversationWindow';

const DEFAULT_VISIBLE_ITERATIONS = Number.MAX_SAFE_INTEGER;
const ITERATION_STEP = 1;
const pendingUploads = [];
const DEFAULT_REASONING_OPTIONS = [
  { value: 'low', label: 'Low' },
  { value: 'medium', label: 'Medium' },
  { value: 'high', label: 'High' },
];

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
  const stage = String(conversation?.stage || conversation?.Stage || '').trim();
  const status = String(conversation?.status || conversation?.Status || '').trim();
  const agent = String(conversation?.agentId || conversation?.AgentId || '').trim();
  const model = String(conversation?.defaultModel || conversation?.DefaultModel || '').trim();
  const embedder = String(conversation?.defaultEmbedder || conversation?.DefaultEmbedder || '').trim();
  if (id) next.id = id;
  if (title) next.title = title;
  if (stage) next.stage = stage;
  if (status) next.status = status;
  next.running = isConversationLiveish(conversation);
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
  logExecutorDebug('chat-service-init', {
    windowId: String(context?.identity?.windowId || '').trim(),
    conversationId: String(context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.()?.id || '').trim()
  });
  setStage({ phase: 'waiting', text: 'Initializing…' });
  try {
    const resources = ensureContextResources(context);
    if (!resources.forgeUIActionUnsub) {
      resources.forgeUIActionUnsub = connectForgeUIActionsToCallbacksOrChat(submitMessage, () => context);
    }
    bindConversationWindowEvents(context);
    await hydrateMeta(context);
    bootstrapConversationSelection(context);
    renderMergedRowsForContext(context);
    setStage({ phase: 'ready', text: 'Ready' });
    const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
    const messagesDS = context?.Context?.('messages')?.handlers?.dataSource;
    const conversationID = String(conversationsDS?.peekFormData?.()?.id || '').trim();
    if (conversationID) {
      if (hasPendingConversationBootstrap(conversationID)) {
        const currentForm = conversationsDS?.peekFormData?.() || {};
        conversationsDS?.setFormData?.({
          values: {
            ...currentForm,
            id: conversationID,
            running: true
          }
        });
        syncConversationTransport(context, conversationID);
        publishActiveConversation(conversationID, context);
        renderMergedRowsForContext(context);
        return;
      }
      const cachedSnapshot = getSettledConversationBootstrapSnapshot(conversationID);
      logStreamDebug(ensureContextResources(context), 'chat-init-bootstrap-snapshot', {
        conversationId: conversationID,
        hasCachedSnapshot: !!cachedSnapshot,
        cachedTurnCount: Array.isArray(cachedSnapshot?.turns) ? cachedSnapshot.turns.length : 0
      });
      if (cachedSnapshot && hydrateConversationFromBootstrapSnapshot(context, cachedSnapshot)) {
        publishActiveConversation(conversationID, context);
        renderMergedRowsForContext(context);
        return;
      }
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
        const mergedConversation = mergeConversationSnapshot(conversationsDS?.peekFormData?.() || {}, existing);
        conversationsDS?.setFormData?.({
          values: mergedConversation
        });
        publishConversationMetaUpdated(conversationID, {
          title: String(mergedConversation?.title || mergedConversation?.Title || '').trim(),
          stage: String(mergedConversation?.stage || mergedConversation?.Stage || '').trim(),
          status: String(mergedConversation?.status || mergedConversation?.Status || '').trim(),
          running: !!mergedConversation?.running,
        });
        const conversationLiveish = isConversationLiveish(existing);
        const initialTransportActive = syncConversationTransport(context, conversationID);
        const snapshot = await dsTick(context, {
          conversationID,
          transcript: {
            includeExecutionDetails: !conversationLiveish,
          },
        });
        if ((snapshot?.hasRunning || conversationLiveish) && !initialTransportActive) {
          syncConversationTransport(context, conversationID);
        } else {
          if (!initialTransportActive) {
            disconnectStream(context);
          }
        }
        publishActiveConversation(conversationID, context);
      }
    }
    renderMergedRowsForContext(context);
  } catch (err) {
    setStage({ phase: 'error', text: String(err?.message || err || 'Initialization failed') });
    context?.Context?.('messages')?.handlers?.dataSource?.setError?.(String(err?.message || err));
  }
  startPolling(context);
}

export function onDestroy({ context }) {
  logExecutorDebug('chat-service-destroy', {
    windowId: String(context?.identity?.windowId || '').trim(),
    conversationId: String(context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.()?.id || '').trim()
  });
  const resources = ensureContextResources(context);
  try { resources.forgeUIActionUnsub?.(); } catch (_) {}
  resources.forgeUIActionUnsub = null;
  stopPolling(context);
  unbindConversationWindowEvents(context);
  setStage({ phase: 'ready', text: 'Ready' });
}

export async function onFetchMeta({ context, data, result, payload, collection }) {
  const singletonCollectionPayload = Array.isArray(collection)
    && collection.length === 1
    && collection[0]
    && typeof collection[0] === 'object'
      ? collection[0]
      : null;
  const source = payload ?? result ?? data ?? singletonCollectionPayload ?? collection ?? {};
  const normalized = normalizeMetaResponse(source);
  publishWorkspaceMetadataSnapshot(normalized);
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  if (metaDS) {
    metaDS.setFormData?.({ values: normalized });
  }
  const convDS = context?.Context?.('conversations')?.handlers?.dataSource;
  if (convDS) {
    const form = convDS.peekFormData?.() || {};
    const next = { ...form };
    if (!String(next.id || '').trim()) {
      next.agent = normalized?.defaults?.agent || '';
      next.model = normalized?.defaults?.model || '';
      next.embedder = normalized?.defaults?.embedder || '';
    } else {
      if (!next.agent && normalized?.defaults?.agent) next.agent = normalized.defaults.agent;
      if (!next.model && normalized?.defaults?.model) next.model = normalized.defaults.model;
      if (!next.embedder && normalized?.defaults?.embedder) next.embedder = normalized.defaults.embedder;
    }
    convDS.setFormData?.({ values: next });
  }
  renderMergedRowsForContext(context);
  return [normalized];
}

export async function onFetchMessages({ context, data, result, payload, collection }) {
  const turns = Array.isArray(collection)
    ? collection
    : (Array.isArray(data)
      ? data
      : (Array.isArray(result)
        ? result
        : (Array.isArray(payload) ? payload : [])));
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

export function onFetchQueuedTurns({ context, data, payload, collection, result }) {
  const turns = Array.isArray(collection)
    ? collection
    : (Array.isArray(data)
      ? data
      : (Array.isArray(result)
        ? result
        : (Array.isArray(payload) ? payload : [])));
  const queuedTurns = mapTranscriptToRows(turns).queuedTurns;
  const queueDS = context?.Context?.('queueTurns')?.handlers?.dataSource;
  queueDS?.setCollection?.(queuedTurns);
  return queuedTurns;
}

export async function submitMessage({ context, message, model, agent }) {
  const rawQuery = typeof message === 'string'
    ? message.trim()
    : String(message?.content || message?.text || message?.value || '').trim();
  const selectedAgent = sanitizeAutoSelection(agent || '');
  const metaForm = context?.Context?.('meta')?.handlers?.dataSource?.peekFormData?.() || {};
  const convForm = context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.() || {};
  const persistedAgent = sanitizeAutoSelection(getPersistedSelectedAgent());
  const effectiveAgentForLookup = resolveSubmitAgent({ selectedAgent, persistedAgent, metaForm, convForm }) || 'default';
  let query = rawQuery;
  if (rawQuery && rawQuery.includes('@{')) {
    try {
      const registry = await listLookupRegistry('chat-composer', effectiveAgentForLookup);
      query = flattenStored(rawQuery, registry, { allowUnresolvedRequired: true }).trim();
    } catch (err) {
      showToast(String(err?.message || err || 'Resolve required lookups before sending.'), { intent: 'warning' });
      return;
    }
  }
  if (!query) return;
  setStage({ phase: 'thinking', text: 'Assistant thinking…' });
  const convDS = context?.Context?.('conversations')?.handlers?.dataSource;
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
  const conversationID = await ensureConversation(context, {
    agent: selectedAgent,
    model: effectiveModel || selectedModel,
    immediateSubmit: true
  });
  const clientRequestId = `msg_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`;
  rememberSeedTitle(conversationID, query);
  convDS?.setFormData?.({
    values: {
      ...convForm,
      id: conversationID
    }
  });
  if (String(context?.identity?.windowId || '').trim() === MAIN_CHAT_WINDOW_ID) {
    markPendingConversationBootstrap(conversationID);
    openConversationInMainWindow(conversationID);
  } else {
    markPendingConversationBootstrap(conversationID);
    publishActiveConversation(conversationID, context);
  }
  try {
    await publishUIBridgeSnapshotNow();
  } catch (_) {}

  const messageAttachments = normalizeUploadItems(message?.attachments || message?.files || []);
  const mergedAttachments = mergeAttachments(messageAttachments, pendingUploads);
  const payload = {
    conversationId: conversationID,
    messageId: clientRequestId,
    query,
    agentId: resolveSubmitAgent({ selectedAgent, persistedAgent, metaForm, convForm }),
    model: effectiveModel || sanitizeAutoSelection(convForm?.model || ''),
    tools: Array.isArray(metaForm?.tool) ? metaForm.tool : undefined,
    reasoningEffort: metaForm?.reasoningEffort || undefined,
    context: buildWebQueryContext(),
    attachments: mergedAttachments.length > 0 ? mergedAttachments : undefined
  };
  const resolvedUserID = resolveUserID(context);
  if (resolvedUserID) {
    payload.userId = resolvedUserID;
  }

  const chatState = ensureContextResources(context);
  clearSettledConversationBootstrapSnapshot(conversationID);
  const activeTurnID = String(chatState?.runningTurnId || chatState?.activeStreamTurnId || '').trim();
  const steeringDuringActiveTurn = !!activeTurnID;
  const queueingDuringActiveTurn = !steeringDuringActiveTurn && !!chatState?.lastHasRunning;
  const submittedAt = Date.now();
  chatState.activeConversationID = conversationID;
  chatState.liveOwnedConversationID = conversationID;
  logStreamDebug(chatState, 'submit-start', {
    conversationId: conversationID,
    clientRequestId,
    agentId: String(payload.agentId || '').trim(),
    model: String(payload.model || '').trim(),
    queueingDuringActiveTurn,
    steeringDuringActiveTurn,
    attachmentCount: Array.isArray(payload.attachments) ? payload.attachments.length : 0,
    queryChars: query.length
  });
  if (steeringDuringActiveTurn) {
    steerToChatStore({
      conversationId: conversationID,
      clientRequestId,
      content: query,
      createdAt: new Date(submittedAt).toISOString(),
      attachments: mergedAttachments.length > 0 ? mergedAttachments : undefined,
    });
    setStage({ phase: 'executing', text: 'Steering…', startedAt: submittedAt, completedAt: 0 });
    await client.steerTurn(conversationID, activeTurnID, { content: query, role: 'user' });
  } else if (!queueingDuringActiveTurn) {
    submitToChatStore({
      conversationId: conversationID,
      clientRequestId,
      content: query,
      createdAt: new Date(submittedAt).toISOString(),
      attachments: mergedAttachments.length > 0 ? mergedAttachments : undefined,
    });
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
    disconnectStream(context);
    setStage({ phase: 'executing', text: 'Executing…', startedAt: submittedAt, completedAt: 0 });
  } else {
    setStage({ phase: 'waiting', text: 'Queued follow-up…' });
  }
  if (!queueingDuringActiveTurn && !steeringDuringActiveTurn) {
    syncConversationTransport(context, conversationID);
    logStreamDebug(chatState, 'submit-stream-sync', {
      conversationId: conversationID,
      strategy: 'immediate-after-query-start'
    });
  }
  let queryResult = {};
  try {
    queryResult = steeringDuringActiveTurn ? {} : await client.query(payload);
  } finally {
    if (String(chatState.pendingInitialSubmitConversationID || '').trim() === conversationID) {
      chatState.pendingInitialSubmitConversationID = '';
    }
  }
  logStreamDebug(chatState, 'submit-query-response', {
    conversationId: conversationID,
    hasInlineContent: String(queryResult?.content || '').trim() !== '',
    resultKeys: queryResult && typeof queryResult === 'object' ? Object.keys(queryResult).length : 0
  });
  pendingUploads.length = 0;
  const fastCompletedInlineResult = !queueingDuringActiveTurn
    && !steeringDuringActiveTurn
    && String(queryResult?.content || '').trim() !== '';
  let fetchedConversation = null;
  let prefetchedTranscriptTurns = [];
  let prefetchedPendingElicitations = [];
  if (fastCompletedInlineResult) {
    chatState.pendingTerminalHydrationConversationID = conversationID;
    const terminalTurnIdFromQueryResult = String(
      queryResult?.turnId
      || queryResult?.messageId
      || ''
    ).trim();
    if (terminalTurnIdFromQueryResult) {
      chatState.prefetchedTerminalConversationID = conversationID;
      chatState.prefetchedTerminalTurnID = terminalTurnIdFromQueryResult;
      chatState.pendingTerminalRefreshSuppressionConversationID = conversationID;
      chatState.pendingTerminalRefreshSuppressionTurnID = terminalTurnIdFromQueryResult;
    }
    resetChatStoreConversation(conversationID);
    resetLiveStreamState(chatState);
    chatState.runningTurnId = '';
    chatState.lastHasRunning = false;
    disconnectStream(context);
    fetchedConversation = await fetchConversation(conversationID);
    try {
      const transcriptPayload = await client.getTranscript({
        conversationId: conversationID,
        includeModelCalls: true,
        includeToolCalls: true,
        includeFeeds: true,
      });
      const canonicalConversation = transcriptPayload?.conversation && typeof transcriptPayload.conversation === 'object'
        ? transcriptPayload.conversation
        : null;
      if (canonicalConversation?.conversationId) {
        applyTranscriptToChatStore(canonicalConversation.conversationId, canonicalConversation);
        prefetchedTranscriptTurns = Array.isArray(canonicalConversation?.turns) ? canonicalConversation.turns : [];
        const latestPrefetchedTurn = prefetchedTranscriptTurns[prefetchedTranscriptTurns.length - 1] || null;
        const latestPrefetchedTurnID = String(
          latestPrefetchedTurn?.turnId
          || latestPrefetchedTurn?.id
          || ''
        ).trim();
        if (latestPrefetchedTurnID) {
          chatState.prefetchedTerminalConversationID = conversationID;
          chatState.prefetchedTerminalTurnID = latestPrefetchedTurnID;
        }
      }
    } catch (_) {
      // dsTick below still performs the canonical fetch/render path.
    }
    try {
      prefetchedPendingElicitations = await fetchPendingElicitations(conversationID);
    } catch (_) {
      prefetchedPendingElicitations = [];
    }
    if (fetchedConversation) {
      const nextConvForm = convDS?.peekFormData?.() || {};
      const settledTitle = fetchedConversation?.title || fetchedConversation?.Title || nextConvForm?.title || '';
      const settledStage = String(fetchedConversation?.stage || fetchedConversation?.Stage || '').trim() || 'done';
      const settledStatus = String(fetchedConversation?.status || fetchedConversation?.Status || '').trim() || 'succeeded';
      convDS?.setFormData?.({
        values: {
          ...nextConvForm,
          id: conversationID,
          title: settledTitle,
          stage: settledStage,
          status: settledStatus,
          running: false,
        }
      });
      publishConversationMetaUpdated(conversationID, {
        title: settledTitle,
        stage: settledStage,
        status: settledStatus,
        running: false,
      });
    }
    cacheSettledConversationBootstrapSnapshot(conversationID, {
      conversation: fetchedConversation,
      turns: prefetchedTranscriptTurns,
      pendingElicitations: prefetchedPendingElicitations,
      generatedFiles: []
    });
  }
  if (queueingDuringActiveTurn || steeringDuringActiveTurn || fastCompletedInlineResult) {
    await new Promise((resolve) => setTimeout(resolve, 140));
    try {
      await dsTick(context, {
        conversationID,
        prefetchedTranscriptTurns: fastCompletedInlineResult ? prefetchedTranscriptTurns : undefined,
        prefetchedPendingElicitations: fastCompletedInlineResult ? prefetchedPendingElicitations : undefined,
      });
    } finally {
      if (fastCompletedInlineResult) {
        clearPendingConversationBootstrap(conversationID);
      }
      if (String(chatState.pendingTerminalHydrationConversationID || '').trim() === conversationID) {
        chatState.pendingTerminalHydrationConversationID = '';
      }
    }
  }
  logStreamDebug(chatState, 'submit-post-dstick', {
    conversationId: conversationID,
    queueingDuringActiveTurn,
    steeringDuringActiveTurn,
    fastCompletedInlineResult
  });
}

function resolveFeedConversationId(context, conversationId = '') {
  const explicit = String(conversationId || '').trim();
  if (explicit) return explicit;
  const fromForm = String(
    context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.()?.id || ''
  ).trim();
  if (fromForm) return fromForm;
  const fromChatState = String(context?.resources?.chat?.activeConversationID || '').trim();
  if (fromChatState) return fromChatState;
  if (typeof window !== 'undefined') {
    const fromPath = conversationIDFromPath(window.location.pathname);
    if (fromPath) return fromPath;
  }
  const windowId = String(context?.identity?.windowId || '').trim() || MAIN_CHAT_WINDOW_ID;
  return String(getScopedConversationSelection(windowId) || '').trim();
}

export function resolveComposerProps({ context, container, metaCtx: providedMetaCtx, conversationsCtx: providedConversationsCtx } = {}) {
  const chatCfg = container?.chat || {};
  if (!chatCfg?.commandCenter) return {};

  const commandCenterCfg = chatCfg.commandCenter;
  const metaRef = (typeof commandCenterCfg === 'object' && commandCenterCfg.dataSourceRef)
    ? commandCenterCfg.dataSourceRef
    : 'meta';
  const metaCtx = providedMetaCtx || context?.Context?.(metaRef);
  const metaDS = metaCtx?.handlers?.dataSource;
  const metaForm = metaDS?.peekFormData?.() || {};
  const conversationsCtx = providedConversationsCtx || context?.Context?.('conversations');
  const convForm = conversationsCtx?.handlers?.dataSource?.peekFormData?.() || {};
  const defaults = metaForm?.defaults || {};
  const currentAgent = normalizeString(metaForm?.agent);
  const persistedAgent = sanitizeAutoSelection(getPersistedSelectedAgent());
  const effectiveLookupAgent = resolveSubmitAgent({
    selectedAgent: currentAgent,
    persistedAgent,
    metaForm,
    convForm,
  }) || 'default';
  const currentModel = resolveCurrentModel(metaForm);
  const defaultModel = normalizeString(defaults?.model);
  const ensureOption = (options = [], value = '', label = '') => {
    const normalizedValue = normalizeString(value);
    if (!normalizedValue) return Array.isArray(options) ? options : [];
    const list = Array.isArray(options) ? [...options] : [];
    if (list.some((entry) => normalizeString(entry?.value ?? entry?.id) === normalizedValue)) {
      return list;
    }
    return [
      ...list,
      {
        value: normalizedValue,
        label: String(label || normalizedValue).trim() || normalizedValue
      }
    ];
  };
  const currentAgentInfo = metaForm?.agentInfo?.[currentAgent] || null;
  const currentModelInfo = metaForm?.modelInfo?.[currentModel] || null;
  const agentOptions = ensureOption(
    Array.isArray(metaForm?.agentOptions) ? metaForm.agentOptions : [],
    currentAgent,
    String(currentAgentInfo?.name || currentAgentInfo?.label || currentAgent).trim()
  );
  const modelOptions = ensureOption(
    Array.isArray(metaForm?.modelOptions) ? metaForm.modelOptions : [],
    currentModel,
    String(currentModelInfo?.name || currentModelInfo?.title || currentModelInfo?.label || currentModel).trim()
  );

  const handleChipClear = (chip) => {
    const id = normalizeString(chip?.id);
    if (!id) return false;
    if (id === 'agent') {
      applyAgentSelection({
        agentID: normalizeString(defaults?.agent) || currentAgent,
        metaDS,
        metaSnapshot: metaForm,
        context,
      });
      return true;
    }
    if (id === 'model') {
      applyModelSelection({
        modelID: defaultAgentModel(metaForm, currentAgent) || defaultModel || currentModel,
        metaDS,
        context,
      });
      return true;
    }
    if (id === 'tools') {
      applyToolsSelection({
        toolNames: defaultAgentTools(metaForm, currentAgent),
        metaDS,
      });
      return true;
    }
    if (id === 'reasoningEffort') {
      applyReasoningSelection({ effort: '', metaDS });
      return true;
    }
    return false;
  };

  return {
    commandCenter: true,
    starterTasks: Array.isArray(metaForm?.starterTasks) ? metaForm.starterTasks : [],
    inputComponent: NamedLookupInput,
    inputProps: {
      context,
      contextKind: 'chat-composer',
      contextID: effectiveLookupAgent,
    },
    agentOptions,
    agentValue: currentAgent,
    onAgentChange: (agentID) => applyAgentSelection({ agentID, metaDS, metaSnapshot: metaForm, context }),
    modelOptions,
    modelInfo: metaForm?.modelInfo || {},
    modelValue: currentModel,
    defaultModel,
    onModelChange: (modelID) => applyModelSelection({ modelID, metaDS, context }),
    reasoningOptions: DEFAULT_REASONING_OPTIONS,
    reasoningValue: normalizeString(metaForm?.reasoningEffort),
    onReasoningChange: (effort) => applyReasoningSelection({ effort, metaDS }),
    selectedTools: ensureStringArray(metaForm?.tool),
    onToolsChange: (toolNames) => applyToolsSelection({ toolNames, metaDS }),
    autoSelectTools: (metaForm?.autoSelectTools !== undefined)
      ? normalizeBool(metaForm?.autoSelectTools)
      : normalizeBool(defaults?.autoSelectTools),
    onAutoSelectToolsChange: (enabled) => applyAutoSelectToolsSelection({ enabled, metaDS, context }),
    activeChips: [],
    onChipClear: handleChipClear,
  };
}

export function renderFeed({ conversationId, context }) {
  const resolvedConversationId = resolveFeedConversationId(context, conversationId);
  return React.createElement(ChatFeedFromChatStore, {
    conversationId: resolvedConversationId,
    context,
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
  if (typeof window !== 'undefined') {
    try {
      console.info('[agently-steer]', {
        event: 'steerTurn:start',
        conversationID,
        turnID,
        contentPreview: String(content || '').slice(0, 160)
      });
    } catch (_) {}
  }
  const result = await client.steerTurn(conversationID, turnID, { content, role: 'user' });
  if (typeof window !== 'undefined') {
    try {
      console.info('[agently-steer]', {
        event: 'steerTurn:accepted',
        conversationID,
        turnID,
        result
      });
    } catch (_) {}
  }
  await dsTick(context);
  return result;
}

export async function forceSteerQueuedTurn({ context, conversationID, turnID }) {
  if (!conversationID || !turnID) return;
  if (typeof window !== 'undefined') {
    try {
      console.info('[agently-steer]', {
        event: 'forceSteer:start',
        conversationID,
        turnID,
      });
    } catch (_) {}
  }
  const result = await client.forceSteerQueuedTurn(conversationID, turnID);
  if (typeof window !== 'undefined') {
    try {
      console.info('[agently-steer]', {
        event: 'forceSteer:accepted',
        conversationID,
        turnID,
        result
      });
    } catch (_) {}
  }
  await dsTick(context);
  return result;
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

export async function forceSteerQueuedTurnBySelection({ context }) {
  const conversationID = getConversationID(context);
  const selected = getQueueSelection(context);
  const turnID = String(selected?.id || selected?.Id || '').trim();
  if (!conversationID || !turnID) return false;
  await forceSteerQueuedTurn({ context, conversationID, turnID });
  return true;
}

export async function saveQueuedTurnForm({ context, parameters }) {
  const content = String(
    parameters?.queueTurns?.content
    ?? parameters?.queueTurns?.preview
    ?? parameters?.content
    ?? parameters?.preview
    ?? readForm(context, 'queueTurns')?.content
    ?? readForm(context, 'queueTurns')?.preview
    ?? ''
  ).trim();
  if (!content) return false;
  await editQueuedTurnBySelection({ context, content });
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
  const row = props?.row || props?.item || props?.node || {};
  const uri = String(props?.uri || row?.uri || row?.URI || '').trim()
    || String(readSelection(props?.context, 'results')?.uri || readSelection(props?.context, 'results')?.URI || '').trim();
  const path = String(props?.path || row?.path || row?.Path || '').trim();
  const target = uri || path;
  if (!target) {
    showToast('Select a file to read.', { intent: 'warning' });
    return false;
  }
  const title = target.split('/').pop() || target;
  openFileViewDialog({ title, uri: target, loading: true, content: '' });
  try {
    const content = path
      ? String(await client.downloadWorkspaceFile(path))
      : String((normalizeToolResult(await client.executeTool('resources-read', { uri, maxBytes: 200000 }))?.content) ?? '');
    updateFileViewDialog({ title, uri: target, loading: false, content });
    return true;
  } catch (err) {
    updateFileViewDialog({ loading: false, content: String(err?.message || err || 'Failed to read file') });
    return false;
  }
}

export async function openResourceFeedPath(props = {}) {
  return explorerRead(props);
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
  const compactFolderChain = (node) => {
    if (!node || !node.isFolder) return node;
    let current = node;
    while (
      current?.isFolder
      && Array.isArray(current.childNodes)
      && current.childNodes.length === 1
      && current.childNodes[0]?.isFolder
    ) {
      const child = current.childNodes[0];
      current.name = `${String(current.name || '').trim()}/${String(child.name || '').trim()}`.replace(/^\/+/, '');
      current.uri = child.uri;
      current.childNodes = Array.isArray(child.childNodes) ? child.childNodes : [];
    }
    current.childNodes = (Array.isArray(current.childNodes) ? current.childNodes : []).map((child) => compactFolderChain(child));
    return current;
  };
  for (const node of nodesByPath.values()) {
    if (Array.isArray(node.childNodes) && node.childNodes.length) sortChildren(node.childNodes);
  }
  const roots = Array.from(nodesByPath.values()).filter((node) => !node.parentPath).map((node) => compactFolderChain(node));
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
  resolveComposerProps,
  renderFeed,
  renderers: {
    bubble: BubbleMessage,
    form: BubbleMessage,
    elicition: BubbleMessage,
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
  saveQueuedTurnForm,
  steerTurn,
  forceSteerQueuedTurn,
  forceSteerQueuedTurnBySelection,
  selectFolder,
  taskStatusIcon,
  onChangedFileSelect,
  prepareChangeFiles,
  runPatchCommit,
  runPatchRollback,
  explorerOpenIcon,
  explorerOpen,
  explorerRead,
  openResourceFeedPath,
  explorerSearch,
  explorerSearchInputChanged
};
