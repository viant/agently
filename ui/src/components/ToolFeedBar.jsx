import React, { useState, useEffect } from 'react';
import { getActiveFeeds, onFeedChange, fetchFeedDataNow, makeFeedKey, splitFeedKey } from '../services/toolFeedBus';
import { usePlanFeed } from '../services/planFeedBus';
import { getQueueSyncSnapshot } from '../services/queueSyncBus';
import { getScopedConversationSelection, getSelectedWindow } from '../services/conversationWindow';

const FEED_ICONS = {
  plan: '📋',
  changes: '📁',
  terminal: '🖥',
  explorer: '🔍',
  queue: '↳',
};

function feedIcon(feedId) {
  const rawFeedId = splitFeedKey(feedId).feedId || String(feedId || '').trim();
  return FEED_ICONS[rawFeedId] || '📊';
}

function dedupeFeeds(feeds = []) {
  const seen = new Map();
  for (const feed of Array.isArray(feeds) ? feeds : []) {
    const id = String(feed?.feedId || '').trim();
    if (!id) continue;
    seen.set(id, { ...(seen.get(id) || {}), ...(feed || {}) });
  }
  return Array.from(seen.values());
}

/**
 * Tracks which feeds are expanded (shown below execution details).
 */
let expandedFeeds = new Set();
let selectedFeedId = '';
const expandListeners = new Set();
function notifyExpand() {
  for (const fn of expandListeners) fn();
}
export function isFeedExpanded(feedId) {
  return expandedFeeds.has(feedId);
}
export function getSelectedFeedId() {
  return selectedFeedId;
}
const selectedFeedListeners = new Set();
function notifySelectedFeed() {
  for (const fn of selectedFeedListeners) fn(selectedFeedId);
}
export function onSelectedFeedChange(fn) {
  selectedFeedListeners.add(fn);
  return () => selectedFeedListeners.delete(fn);
}
function toggleFeedExpanded(feedId, conversationId) {
  if (expandedFeeds.has(feedId)) {
    expandedFeeds.delete(feedId);
    if (selectedFeedId === feedId) {
      selectedFeedId = Array.from(expandedFeeds)[0] || '';
      notifySelectedFeed();
    }
  } else {
    expandedFeeds.add(feedId);
    selectedFeedId = feedId;
    notifySelectedFeed();
    // Always fetch fresh merged data from backend on toggle.
    if (conversationId) {
      fetchFeedDataNow(feedId, conversationId);
    }
  }
  notifyExpand();
}
function useExpandedFeeds() {
  const [, forceUpdate] = useState(0);
  useEffect(() => {
    const listener = () => forceUpdate((n) => n + 1);
    expandListeners.add(listener);
    return () => expandListeners.delete(listener);
  }, []);
  return expandedFeeds;
}

export default function ToolFeedBar() {
  const [feeds, setFeeds] = useState(getActiveFeeds);
  const planFeed = usePlanFeed();
  const [queueVersion, setQueueVersion] = useState(0);
  const expanded = useExpandedFeeds();
  const selectedWindow = getSelectedWindow();
  const currentConversationId = String(
    getScopedConversationSelection(String(selectedWindow?.windowId || '').trim())
    || ''
  ).trim();

  useEffect(() => {
    return onFeedChange((next) => {
      setFeeds(next);
      // Clear expand state when feeds are cleared (conversation switch).
      if (!next || next.length === 0) {
        expandedFeeds.clear();
        selectedFeedId = '';
      }
    });
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setQueueVersion((n) => n + 1), 250);
    return () => clearInterval(interval);
  }, []);

  const visibleFeeds = (Array.isArray(feeds) ? feeds : []).filter((feed) => {
    const conversationId = String(feed?.conversationId || '').trim();
    if (!conversationId) return true;
    return conversationId === currentConversationId;
  });
  const planConversationId = String(planFeed?.conversationId || '').trim();
  const queueSnapshot = getQueueSyncSnapshot(currentConversationId);
  const queueTurns = Array.isArray(queueSnapshot?.queuedTurns) ? queueSnapshot.queuedTurns : [];
  const hasPlanData = !!String(planFeed?.explanation || '').trim()
    || (Array.isArray(planFeed?.steps) && planFeed.steps.length > 0);
  const mergedFeeds = dedupeFeeds([
    ...(queueTurns.length > 0 ? [{
      feedId: makeFeedKey('queue', currentConversationId),
      title: 'Queue',
      itemCount: queueTurns.length,
      conversationId: currentConversationId,
      rawFeedId: 'queue',
      _queueVersion: queueVersion,
    }] : []),
    ...(hasPlanData && (!planConversationId || planConversationId === currentConversationId)
      ? [{
          feedId: makeFeedKey('plan', planConversationId),
          title: 'Plan',
          itemCount: Array.isArray(planFeed?.steps) ? planFeed.steps.length : 0,
          conversationId: planConversationId,
          rawFeedId: 'plan',
        }]
      : []),
    ...visibleFeeds,
  ]);

  if (mergedFeeds.length === 0) return null;

  return (
    <div className="app-tool-feed-bar">
      {mergedFeeds.map((feed) => {
        const isExpanded = expanded.has(feed.feedId);
        return (
          <div
            className="app-tool-feed-bar-item"
            key={feed.feedId}
            onClick={() => toggleFeedExpanded(feed.feedId, feed.conversationId)}
            role="button"
            tabIndex={0}
          >
            <span className="app-tool-feed-bar-icon">{feedIcon(feed.rawFeedId || feed.feedId)}</span>
            <span className="app-tool-feed-bar-title">{feed.title || feed.feedId}</span>
            {feed.itemCount > 0 ? (
              <span className="app-tool-feed-bar-badge">{feed.itemCount}</span>
            ) : null}
            <span className={`app-tool-feed-bar-toggle${isExpanded ? ' is-on' : ''}`} />
          </div>
        );
      })}
    </div>
  );
}
