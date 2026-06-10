/**
 * Singleton FeedTracker from the SDK.
 * chatRuntime stores active feeds here via SSE events.
 * ToolFeedBar subscribes and renders the indicator above the composer.
 *
 * When tool_feed_active arrives, feed data is cached from the SSE event
 * payload so the detail panel can render immediately.
 */
import { FeedTracker } from 'agently-core-ui-sdk';
import { client } from './agentlyClient';
import { cleanupFeedSignals } from './feedForgeWiring';
import { clearFeedSelection, clearFeedSelectionForConversation, registerFeedDataLoader } from './toolFeedSelection';

export const feedTracker = new FeedTracker();

// Cached feed data keyed by feedId. Cleared on conversation switch.
let feedDataCache = {};
let inactiveFeedKeys = new Set();
const dataListeners = new Set();

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
    feedKey: scopedKey,
    feedId: normalizedFeedId,
    _conversationId: normalizedConversationId
  });
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
      feedDataCache[scopedKey] = normalizeFeedPayload({
        ...(existing || {}),
        ...data,
        data: data?.data != null ? data.data : (existing?.data ?? null),
        feedKey: scopedKey,
        feedId: normalizedFeedId,
        _conversationId: normalizedConversationId
      });
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
  clearFeedSelection();
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
  clearFeedSelectionForConversation(normalizedConversationId);
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
  feedTracker.applyEvent(trackerEvent);

  if (payload?.type === 'tool_feed_active') {
    inactiveFeedKeys.delete(scopedKey);
    const existing = feedDataCache[scopedKey] || null;
    // Set inline data immediately for fast rendering.
    if (payload.feedData) {
      feedDataCache[scopedKey] = normalizeFeedPayload({
        data: payload.feedData,
        feedKey: scopedKey,
        feedId,
        title: payload.feedTitle || feedId,
        _conversationId: conversationId
      });
      notifyDataChange();
    }
    // Fetch from API only when the scoped feed does not already have the
    // feed spec (dataSources + ui). This avoids repeated spec fetches for
    // queue/local feeds and transcript/live replays of the same feed.
    const needsSpecFetch = !existing?.ui || !existing?.dataSources;
    if (conversationId && needsSpecFetch) {
      client.getFeedData(feedId, conversationId).then((data) => {
        if (data) {
          const latest = feedDataCache[scopedKey] || existing || {};
          feedDataCache[scopedKey] = normalizeFeedPayload({
            ...latest,
            ...data,
            data: data?.data != null ? data.data : (latest?.data ?? null),
            feedKey: scopedKey,
            feedId,
            _conversationId: conversationId
          });
          notifyDataChange();
        }
      }).catch(() => {});
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
