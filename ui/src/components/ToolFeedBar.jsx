import React, { useState, useEffect } from 'react';
import { getActiveFeeds, onFeedChange, fetchFeedDataNow, splitFeedKey } from '../services/toolFeedBus';
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
let autoExpandedFeedSignature = '';
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
export function __resetToolFeedBarStateForTest() {
  expandedFeeds = new Set();
  selectedFeedId = '';
  autoExpandedFeedSignature = '';
}
function feedSignature(feeds = []) {
  return (Array.isArray(feeds) ? feeds : [])
    .map((feed) => `${String(feed?.conversationId || '').trim()}::${String(feed?.feedId || '').trim()}`)
    .join('|');
}
function fetchFeedIfNeeded(feedId, conversationId) {
  if (conversationId) {
    fetchFeedDataNow(feedId, conversationId);
  }
}

export function expandFeed(feedId, conversationId) {
  if (!feedId) return;
  if (!expandedFeeds.has(feedId)) {
    expandedFeeds.add(feedId);
    notifyExpand();
  }
  selectedFeedId = feedId;
  notifySelectedFeed();
  fetchFeedIfNeeded(feedId, conversationId);
}

export function collapseFeed(feedId) {
  if (!feedId) return;
  if (!expandedFeeds.has(feedId)) return;
  expandedFeeds.delete(feedId);
  if (selectedFeedId === feedId) {
    selectedFeedId = Array.from(expandedFeeds)[0] || '';
    notifySelectedFeed();
  }
  notifyExpand();
}

export function toggleFeedExpanded(feedId, conversationId) {
  if (expandedFeeds.has(feedId)) {
    collapseFeed(feedId);
  } else {
    expandFeed(feedId, conversationId);
  }
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
        autoExpandedFeedSignature = '';
      }
    });
  }, []);

  const visibleFeeds = (Array.isArray(feeds) ? feeds : []).filter((feed) => {
    const conversationId = String(feed?.conversationId || '').trim();
    if (!conversationId) return true;
    return conversationId === currentConversationId;
  });
  const mergedFeeds = dedupeFeeds(visibleFeeds);

  useEffect(() => {
    if (mergedFeeds.length === 0) return;
    const hasExpandedVisibleFeed = mergedFeeds.some((feed) => expandedFeeds.has(feed.feedId));
    if (hasExpandedVisibleFeed) return;
    const signature = feedSignature(mergedFeeds);
    if (autoExpandedFeedSignature === signature) return;
    const first = mergedFeeds[0];
    if (!first?.feedId) return;
    autoExpandedFeedSignature = signature;
    expandedFeeds.add(first.feedId);
    selectedFeedId = first.feedId;
    notifySelectedFeed();
    notifyExpand();
    if (first.conversationId) {
      fetchFeedDataNow(first.rawFeedId || splitFeedKey(first.feedId).feedId, first.conversationId);
    }
  }, [mergedFeeds]);

  const hasAnyExpandedFeed = mergedFeeds.some((feed) => expanded.has(feed.feedId));

  if (mergedFeeds.length === 0) return null;

  return (
    <div className="app-tool-feed-bar">
      {mergedFeeds.map((feed) => {
        const isExpanded = expanded.has(feed.feedId);
        return (
          <div
            className="app-tool-feed-bar-item"
            key={feed.feedId}
            onClick={() => expandFeed(feed.feedId, feed.conversationId)}
            role="button"
            tabIndex={0}
          >
            <span className="app-tool-feed-bar-icon">{feedIcon(feed.rawFeedId || feed.feedId)}</span>
            <span className="app-tool-feed-bar-title">{feed.title || feed.feedId}</span>
            {feed.itemCount > 0 ? (
              <span className="app-tool-feed-bar-badge">{feed.itemCount}</span>
            ) : null}
            <span
              className={`app-tool-feed-bar-toggle${isExpanded ? ' is-on' : ''}`}
              role="switch"
              aria-checked={isExpanded}
              onClick={(event) => {
                event.stopPropagation();
                toggleFeedExpanded(feed.feedId, feed.conversationId);
              }}
            />
          </div>
        );
      })}
    </div>
  );
}
