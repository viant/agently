/**
 * Singleton FeedTracker from the SDK.
 * chatRuntime stores active feeds here via SSE events.
 * ToolFeedBar subscribes and renders the indicator above the composer.
 *
 * When tool_feed_active arrives, feed data is cached from the SSE event
 * payload so the detail panel can render immediately.
 */
import { FeedTracker, applyFeedPatchOperation } from 'agently-core-ui-sdk';
import { getCollectionSignal, getFormSignal, getFormStatusSignal } from 'forge/core';
import { client } from './agentlyClient';
import { cleanupFeedSignals, normalizeDataSources, wireFeedSignals } from './feedForgeWiring';
import { clearFeedSelection, clearFeedSelectionForConversation, registerFeedDataLoader } from './toolFeedSelection';
import { clearFeedEntityAliases, registerFeedEntityAlias } from './feedEntityAliases';

export const feedTracker = new FeedTracker();

// Cached feed data keyed by feedId. Cleared on conversation switch.
let feedDataCache = {};
let inactiveFeedKeys = new Set();
let previewTurnByFeedKey = new Map();
const dataListeners = new Set();

function decodePointer(path = '') {
  return String(path || '').split('/').slice(1).map((part) => part.replace(/~1/g, '/').replace(/~0/g, '~'));
}

function encodePointer(parts = []) {
  return `/${parts.map((part) => String(part).replace(/~/g, '~0').replace(/\//g, '~1')).join('/')}`;
}

function dataSourceRelativeParts(path = '') {
  const parts = decodePointer(path);
  if (parts[0] === 'collection' || parts[0] === 'form') return parts.slice(1);
  if (parts[0] === 'selection' && parts[1] === 'selection') return parts.slice(2);
  return parts;
}

function dataSourceRootParts(payload = {}, dataSourceRef = '', seen = new Set()) {
  const ref = String(dataSourceRef || '').trim();
  if (!ref || seen.has(ref)) return [];
  seen.add(ref);
  const config = payload?.dataSources?.[ref] || payload?.ui?.dataSources?.[ref] || {};
  const source = String(config?.source || '').trim();
  if (source) return source.split('.').filter(Boolean);
  const parent = String(config?.dataSourceRef || '').trim();
  if (!parent) return [];
  const base = dataSourceRootParts(payload, parent, seen);
  const selector = String(config?.selectors?.data || '').trim();
  if (!selector || selector === 'output' || selector === 'input') return base;
  return [...base, ...selector.split('.').filter(Boolean)];
}

export function applyActiveFeedUpdate(detail = {}) {
  const feedId = String(detail?.feedId || '').trim();
  const conversationId = String(detail?.conversationId || '').trim();
  const operations = Array.isArray(detail?.operations) ? detail.operations : [];
  if (!feedId || !conversationId || operations.length === 0) return false;
  const windowId = `feed-${feedId}-${conversationId}`;
  const scopedKey = makeFeedKey(feedId, conversationId);
  const cached = feedDataCache[scopedKey];
  const effectiveOperations = operations;
  const patchedCachedData = !!cached?.data;
  if (patchedCachedData) {
    let patchedData = cached.data;
    const dirtyDataSourceRefs = new Set(Array.isArray(cached?._dirtyDataSourceRefs) ? cached._dirtyDataSourceRefs : []);
    const canonicalOperations = new Map();
    for (const operation of effectiveOperations) {
      const base = dataSourceRootParts(cached, operation?.dataSourceRef);
      if (base.length === 0) continue;
      if (String(operation?.dataSourceRef || '').trim()) dirtyDataSourceRefs.add(String(operation.dataSourceRef).trim());
      const relative = dataSourceRelativeParts(operation?.path);
      const canonicalOperation = {
        ...operation,
        path: encodePointer([...base, ...relative]),
      };
      const fingerprint = JSON.stringify({
        op: canonicalOperation.op,
        path: canonicalOperation.path,
        value: canonicalOperation.value,
      });
      const priorRef = canonicalOperations.get(fingerprint);
      if (priorRef && priorRef !== operation?.dataSourceRef) continue;
      canonicalOperations.set(fingerprint, operation?.dataSourceRef);
      patchedData = applyFeedPatchOperation(patchedData, canonicalOperation);
    }
    feedDataCache[scopedKey] = { ...cached, data: patchedData, _dirtyDataSourceRefs: [...dirtyDataSourceRefs] };
    notifyDataChange();
  }
  const current = feedTracker.get(scopedKey);
  if (current) {
    const previewTurnId = String(detail?.turnId || current?.turnId || '').trim();
    if (previewTurnId) previewTurnByFeedKey.set(scopedKey, previewTurnId);
    feedTracker.setActive({
      ...current,
      feedId: scopedKey,
      conversationId,
      turnId: previewTurnId,
    });
  }
  const applyOperations = () => {
    // Cached feed data is the canonical draft. The data-change notification
    // makes the feed renderer re-wire Forge signals from that patched cache.
    // Applying the same array operation directly to the signals afterwards
    // would run remove/add twice (for example removing both the requested row
    // and the row that shifted into its index).
    if (patchedCachedData) {
      const latest = feedDataCache[scopedKey] || cached;
      const dataSources = normalizeDataSources(latest?.ui?.dataSources || latest?.dataSources || {});
      const rootName = Object.entries(dataSources).find(([, definition]) => (
        String(definition?.source || '').trim() && !String(definition?.dataSourceRef || '').trim()
      ))?.[0] || '';
      wireFeedSignals({ dataSources, dataFeed: { name: rootName, data: latest?.data } }, windowId);
      for (const operation of effectiveOperations) {
        const dataSourceRef = String(operation?.dataSourceRef || '').trim();
        if (!dataSourceRef) continue;
        const status = getFormStatusSignal(`${windowId}DS${dataSourceRef}`);
        status.value = { ...(status.peek?.() || status.value || {}), dirty: true };
      }
      return;
    }
    for (const operation of effectiveOperations) {
      const dataSourceRef = String(operation?.dataSourceRef || '').trim();
      if (!dataSourceRef) continue;
      const dataSourceId = `${windowId}DS${dataSourceRef}`;
      const form = getFormSignal(dataSourceId);
      form.value = applyFeedPatchOperation(form.peek?.() || form.value || {}, operation);
      const collection = getCollectionSignal(dataSourceId);
      const rows = collection.peek?.() || collection.value || [];
      if (Array.isArray(rows)) {
        const firstToken = decodePointer(operation.path)[0] || '';
        collection.value = rows.length === 1 && !/^\d+$/.test(firstToken) && firstToken !== '-'
          ? [applyFeedPatchOperation(rows[0] || {}, operation)]
          : applyFeedPatchOperation(rows, operation);
      }
    }
    if (typeof window !== 'undefined') window.lastToolFeedUpdate = detail;
  };
  if (typeof window !== 'undefined') window.setTimeout(applyOperations, 25);
  else applyOperations();
  return true;
}

function looksLikeResolvedFeedPayload(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  if (value.ui && typeof value.ui === 'object') return true;
  if (value.dataSources && typeof value.dataSources === 'object') return true;
  if (value.dataFeed && typeof value.dataFeed === 'object') return true;
  if (Array.isArray(value.containers)) return true;
  if (String(value.renderMode || '').trim()) return true;
  return false;
}

export function normalizeFeedPayload(payload) {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return payload;
  const base = { ...payload };
  const nested = base.data;
  if (
    !looksLikeResolvedFeedPayload(base)
    && nested
    && typeof nested === 'object'
    && !Array.isArray(nested)
    && looksLikeResolvedFeedPayload(nested)
  ) {
    return {
      ...base,
      ...nested,
      data: Object.prototype.hasOwnProperty.call(nested, 'data') ? nested.data : nested,
    };
  }
  return base;
}

export function makeFeedKey(feedId = '', conversationId = '') {
  const rawFeedId = String(feedId || '').trim();
  const rawConversationId = String(conversationId || '').trim();
  if (!rawFeedId) return '';
  return rawConversationId ? `${rawConversationId}::${rawFeedId}` : rawFeedId;
}

export function splitFeedKey(feedKey = '') {
  const raw = String(feedKey || '').trim();
  if (!raw) return { feedId: '', conversationId: '' };
  const idx = raw.indexOf('::');
  if (idx === -1) return { feedId: raw, conversationId: '' };
  return {
    conversationId: raw.slice(0, idx).trim(),
    feedId: raw.slice(idx + 2).trim()
  };
}

function normalizeScopedFeedIdentity(feedId = '', conversationId = '') {
  const directFeedId = String(feedId || '').trim();
  const directConversationId = String(conversationId || '').trim();
  const split = splitFeedKey(directFeedId);
  const normalizedFeedId = String(split.feedId || directFeedId).trim();
  const normalizedConversationId = String(split.conversationId || directConversationId).trim();
  return {
    feedId: normalizedFeedId,
    conversationId: normalizedConversationId,
    scopedKey: makeFeedKey(normalizedFeedId, normalizedConversationId),
  };
}

function notifyDataChange() {
  for (const fn of dataListeners) fn();
}

function syncFeedPresentation(feedKey, presentation) {
  if (!presentation || typeof presentation !== 'object') return;
  const current = feedTracker.get(feedKey);
  if (!current) return;
  feedTracker.setActive({ ...current, presentation });
}

export function getActiveFeeds() {
  return feedTracker.feeds;
}

export function onFeedChange(fn) {
  return feedTracker.onChange(fn);
}

/** Get cached feed data. */
export function getFeedData(feedId, conversationId = '') {
  const { feedId: normalizedFeedId, scopedKey } = normalizeScopedFeedIdentity(feedId, conversationId);
  if (scopedKey && feedDataCache[scopedKey]) return normalizeFeedPayload(feedDataCache[scopedKey]) || null;
  return normalizedFeedId ? (normalizeFeedPayload(feedDataCache[normalizedFeedId]) || null) : null;
}

export function isFeedInactive(feedId, conversationId = '') {
  const { scopedKey } = normalizeScopedFeedIdentity(feedId, conversationId);
  return scopedKey ? inactiveFeedKeys.has(scopedKey) : false;
}

export function updateFeedData(feedId, patch = {}, conversationId = '') {
  const { feedId: normalizedFeedId, conversationId: normalizedConversationId, scopedKey } = normalizeScopedFeedIdentity(
    feedId,
    conversationId || patch?._conversationId || ''
  );
  if (!scopedKey) return;
  const current = feedDataCache[scopedKey] || {
    feedKey: scopedKey,
    feedId: normalizedFeedId,
    _conversationId: normalizedConversationId
  };
  feedDataCache[scopedKey] = normalizeFeedPayload({
    ...current,
    ...(patch || {}),
    ...(Object.prototype.hasOwnProperty.call(patch || {}, 'data') ? { _dirtyDataSourceRefs: [] } : {}),
    feedKey: scopedKey,
    feedId: normalizedFeedId,
    _conversationId: normalizedConversationId
  });
  registerFeedEntityAlias(feedDataCache[scopedKey], normalizedConversationId);
  notifyDataChange();
}

/** Subscribe to feed data changes. Returns unsubscribe function. */
export function onFeedDataChange(fn) {
  dataListeners.add(fn);
  return () => dataListeners.delete(fn);
}

/** Fetch fresh feed data from backend (always makes a call, no cache check). */
export function fetchFeedDataNow(feedId, conversationId) {
  const { feedId: normalizedFeedId, conversationId: normalizedConversationId, scopedKey } = normalizeScopedFeedIdentity(feedId, conversationId);
  if (!scopedKey || !normalizedConversationId) return;
  const existing = feedDataCache[scopedKey] || null;
  // Clear stale cache entry before fetch unless we already have inline/local data.
  if (!existing?.data) {
    delete feedDataCache[scopedKey];
  }
  client.getFeedData(normalizedFeedId, normalizedConversationId).then((data) => {
    if (data) {
      syncFeedPresentation(scopedKey, data.presentation);
      const latest = feedDataCache[scopedKey] || existing || {};
      const keepDirtyData = Array.isArray(latest?._dirtyDataSourceRefs) && latest._dirtyDataSourceRefs.length > 0;
      feedDataCache[scopedKey] = normalizeFeedPayload({
        ...latest,
        ...data,
        data: keepDirtyData ? latest?.data : (data?.data != null ? data.data : (latest?.data ?? null)),
        ...(keepDirtyData ? { _dirtyDataSourceRefs: latest._dirtyDataSourceRefs } : {}),
        feedKey: scopedKey,
        feedId: normalizedFeedId,
        _conversationId: normalizedConversationId
      });
      registerFeedEntityAlias(feedDataCache[scopedKey], normalizedConversationId);
    }
    notifyDataChange();
  }).catch(() => {
    notifyDataChange();
  });
}

registerFeedDataLoader(fetchFeedDataNow);

/**
 * Clear all feed state (cache + tracker). Call on conversation switch.
 */
export function clearFeedState() {
  for (const [id, cached] of Object.entries(feedDataCache)) {
    if (cached?.dataSources) {
      cleanupFeedSignals(id, Object.keys(cached.dataSources), cached?._conversationId || '');
    }
  }
  feedDataCache = {};
  inactiveFeedKeys = new Set();
  previewTurnByFeedKey = new Map();
  clearFeedSelection();
  clearFeedEntityAliases();
  feedTracker.clear();
  notifyDataChange();
}

export function clearFeedStateForConversation(conversationId = '') {
  const normalizedConversationId = String(conversationId || '').trim();
  if (!normalizedConversationId) return;
  for (const [id, cached] of Object.entries(feedDataCache)) {
    if (String(cached?._conversationId || '').trim() !== normalizedConversationId) continue;
    if (cached?.dataSources) {
      cleanupFeedSignals(id, Object.keys(cached.dataSources), normalizedConversationId);
    }
    delete feedDataCache[id];
  }
  inactiveFeedKeys = new Set(
    Array.from(inactiveFeedKeys).filter((feedKey) => {
      const { conversationId: scopedConversationId } = splitFeedKey(feedKey);
      return scopedConversationId !== normalizedConversationId;
    })
  );
  previewTurnByFeedKey = new Map(
    Array.from(previewTurnByFeedKey.entries()).filter(([feedKey]) => splitFeedKey(feedKey).conversationId !== normalizedConversationId)
  );
  clearFeedSelectionForConversation(normalizedConversationId);
  clearFeedEntityAliases(normalizedConversationId);
  for (const feed of feedTracker.feeds) {
    if (String(feed?.conversationId || '').trim() !== normalizedConversationId) continue;
    feedTracker.setInactive(String(feed?.feedId || '').trim());
  }
  notifyDataChange();
}

/**
 * Called when tool_feed_active/inactive SSE arrives.
 */
export function applyFeedEvent(payload) {
  const feedId = String(payload?.feedId || '').trim();
  const conversationId = payload?.conversationId || payload?.streamId || '';
  const scopedKey = makeFeedKey(feedId, conversationId);
  if (!scopedKey) return;
  const trackerEvent = {
    ...payload,
    feedId: scopedKey,
    rawFeedId: feedId,
    conversationId: String(conversationId || '').trim(),
    feedTitle: payload?.feedTitle || feedId,
  };
  const previewTurnId = previewTurnByFeedKey.get(scopedKey) || '';
  if (payload?.type === 'tool_feed_active' && previewTurnId) {
    const incomingTurnId = String(payload?.turnId || '').trim();
    if (payload?.createdAt && incomingTurnId && incomingTurnId !== previewTurnId) {
      previewTurnByFeedKey.delete(scopedKey);
    } else {
      trackerEvent.turnId = previewTurnId;
    }
  }
  feedTracker.applyEvent(trackerEvent);

  if (payload?.type === 'tool_feed_active') {
    inactiveFeedKeys.delete(scopedKey);
    const existing = feedDataCache[scopedKey] || null;
    const incomingTurnId = String(payload?.turnId || '').trim();
    const hasDirtyPreview = Array.isArray(existing?._dirtyDataSourceRefs) && existing._dirtyDataSourceRefs.length > 0;
    const preservePreviewData = hasDirtyPreview && previewTurnId
      && (!incomingTurnId || incomingTurnId === previewTurnId);
    // Set inline data immediately for fast rendering.
    if (payload.feedData && !preservePreviewData) {
      feedDataCache[scopedKey] = normalizeFeedPayload({
        ...(existing || {}),
        data: payload.feedData,
        _dirtyDataSourceRefs: [],
        feedKey: scopedKey,
        feedId,
        title: payload.feedTitle || feedId,
        _conversationId: conversationId
      });
      registerFeedEntityAlias(feedDataCache[scopedKey], String(conversationId || '').trim());
      notifyDataChange();
    }
    // Fetch from API only when the scoped feed does not already have the
    // feed spec (dataSources + ui). This avoids repeated spec fetches for
    // queue/local feeds and transcript/live replays of the same feed.
    const needsSpecFetch = !existing?.ui || !existing?.dataSources;
    if (conversationId && needsSpecFetch) {
      client.getFeedData(feedId, conversationId).then((data) => {
        if (data) {
          syncFeedPresentation(scopedKey, data.presentation);
          const latest = feedDataCache[scopedKey] || existing || {};
          const keepDirtyData = Array.isArray(latest?._dirtyDataSourceRefs) && latest._dirtyDataSourceRefs.length > 0;
          feedDataCache[scopedKey] = normalizeFeedPayload({
            ...latest,
            ...data,
            data: keepDirtyData ? latest?.data : (data?.data != null ? data.data : (latest?.data ?? null)),
            ...(keepDirtyData ? { _dirtyDataSourceRefs: latest._dirtyDataSourceRefs } : {}),
            feedKey: scopedKey,
            feedId,
            _conversationId: conversationId
          });
          registerFeedEntityAlias(feedDataCache[scopedKey], String(conversationId || '').trim());
          notifyDataChange();
        }
      }).catch((error) => {
        console.error('Tool Feed metadata fetch failed', {
          feedId,
          conversationId,
          message: String(error?.message || error || 'unknown error'),
        });
      });
    }
  }

  if (payload?.type === 'tool_feed_inactive') {
    inactiveFeedKeys.add(scopedKey);
    const cached = feedDataCache[scopedKey];
    if (cached?.dataSources) {
      cleanupFeedSignals(scopedKey, Object.keys(cached.dataSources), conversationId);
    }
    delete feedDataCache[scopedKey];
    notifyDataChange();
  }
}

if (typeof window !== 'undefined' && !window.__agentlyFeedUpdateListenerInstalled) {
  window.__agentlyFeedUpdateListenerInstalled = true;
  window.addEventListener('forge:feed-update', (event) => {
    applyActiveFeedUpdate(event?.detail || {});
  });
}
