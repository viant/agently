/**
 * Singleton FeedTracker from the SDK.
 * chatRuntime stores active feeds here via SSE events.
 * ToolFeedBar subscribes and renders the indicator above the composer.
 *
 * When tool_feed_active arrives, we also fetch fresh feed data from the
 * backend so the detail panel renders up-to-date content.
 */
import { FeedTracker } from 'agently-core-ui-sdk';
import { client } from './agentlyClient';
import { cleanupFeedSignals } from './feedForgeWiring';

export const feedTracker = new FeedTracker();

// Cached feed data keyed by feedId. Refreshed on every tool_feed_active.
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

/** Get cached feed data (fetched on last tool_feed_active). */
export function getFeedData(feedId) {
  return feedDataCache[feedId] || null;
}

/** Subscribe to feed data changes. Returns unsubscribe function. */
export function onFeedDataChange(fn) {
  dataListeners.add(fn);
  return () => dataListeners.delete(fn);
}

/**
 * Called when tool_feed_active SSE arrives. Updates the tracker AND
 * fetches fresh feed data from the backend before notifying listeners.
 */
export function applyFeedEvent(payload) {
  feedTracker.applyEvent(payload);

  const feedId = payload?.feedId;
  const conversationId = payload?.conversationId || payload?.streamId || '';
  if (feedId && conversationId && payload?.type === 'tool_feed_active') {
    // Fetch fresh data — don't await, update cache async.
    client.getFeedData(feedId, conversationId).then((data) => {
      feedDataCache[feedId] = data;
      notifyDataChange();
    }).catch(() => {});
  }
  if (payload?.type === 'tool_feed_inactive' && feedId) {
    // Clean up Forge signals for this feed.
    const cached = feedDataCache[feedId];
    if (cached?.dataSources) {
      cleanupFeedSignals(feedId, Object.keys(cached.dataSources));
    }
    delete feedDataCache[feedId];
    notifyDataChange();
  }
}
