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

export const feedTracker = new FeedTracker();

// Cached feed data keyed by feedId. Cleared on conversation switch.
let feedDataCache = {};
const dataListeners = new Set();

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
export function getFeedData(feedId) {
  return feedDataCache[feedId] || null;
}

/** Subscribe to feed data changes. Returns unsubscribe function. */
export function onFeedDataChange(fn) {
  dataListeners.add(fn);
  return () => dataListeners.delete(fn);
}

/** Fetch fresh feed data from backend (always makes a call, no cache check). */
export function fetchFeedDataNow(feedId, conversationId) {
  if (!feedId || !conversationId) return;
  // Clear stale cache entry before fetch.
  delete feedDataCache[feedId];
  client.getFeedData(feedId, conversationId).then((data) => {
    if (data) {
      feedDataCache[feedId] = { ...data, _conversationId: conversationId };
    }
    notifyDataChange();
  }).catch(() => {
    notifyDataChange();
  });
}

/**
 * Clear all feed state (cache + tracker). Call on conversation switch.
 */
export function clearFeedState() {
  for (const [id, cached] of Object.entries(feedDataCache)) {
    if (cached?.dataSources) {
      cleanupFeedSignals(id, Object.keys(cached.dataSources));
    }
  }
  feedDataCache = {};
  feedTracker.clear();
  notifyDataChange();
}

/**
 * Called when tool_feed_active/inactive SSE arrives.
 */
export function applyFeedEvent(payload) {
  feedTracker.applyEvent(payload);

  const feedId = payload?.feedId;
  const conversationId = payload?.conversationId || payload?.streamId || '';

  if (feedId && payload?.type === 'tool_feed_active') {
    // Set inline data immediately for fast rendering.
    if (payload.feedData) {
      feedDataCache[feedId] = { data: payload.feedData, feedId, title: payload.feedTitle || feedId, _conversationId: conversationId };
      notifyDataChange();
    }
    // Always fetch from API to get the full spec (dataSources + ui from YAML).
    if (conversationId) {
      client.getFeedData(feedId, conversationId).then((data) => {
        if (data) {
          feedDataCache[feedId] = { ...data, _conversationId: conversationId };
          notifyDataChange();
        }
      }).catch(() => {});
    }
  }

  if (payload?.type === 'tool_feed_inactive' && feedId) {
    const cached = feedDataCache[feedId];
    if (cached?.dataSources) {
      cleanupFeedSignals(feedId, Object.keys(cached.dataSources));
    }
    delete feedDataCache[feedId];
    notifyDataChange();
  }
}
