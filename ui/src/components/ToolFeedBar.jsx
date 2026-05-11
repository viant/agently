import React, { useState, useEffect } from 'react';
import { getActiveFeeds, onFeedChange, splitFeedKey } from '../services/toolFeedBus';
import { getScopedConversationSelection, getSelectedWindow } from '../services/conversationWindow';
import {
  __resetToolFeedSelectionForTest,
  expandFeed,
  getExpandedFeedIds,
  getSelectedFeedId,
  isFeedExpanded,
  onFeedExpansionChange,
  onSelectedFeedChange,
  reconcileFeedSelection,
  toggleFeedExpanded,
} from '../services/toolFeedSelection';

export {
  expandFeed,
  getSelectedFeedId,
  isFeedExpanded,
  onSelectedFeedChange,
  toggleFeedExpanded,
} from '../services/toolFeedSelection';

export function feedIcon(feedId, title = '') {
  const rawTitle = String(title || '').trim();
  const rawFeedId = splitFeedKey(feedId).feedId || String(feedId || '').trim();
  const source = rawTitle || rawFeedId;
  const match = source.match(/[A-Za-z0-9]/);
  return match ? match[0].toUpperCase() : '•';
}

export function dedupeFeeds(feeds = []) {
  const seen = new Map();
  for (const feed of Array.isArray(feeds) ? feeds : []) {
    const id = String(feed?.feedId || '').trim();
    if (!id) continue;
    seen.set(id, { ...(seen.get(id) || {}), ...(feed || {}) });
  }
  return Array.from(seen.values());
}

let autoExpandedFeedSignature = '';
export function __resetToolFeedBarStateForTest() {
  __resetToolFeedSelectionForTest();
  autoExpandedFeedSignature = '';
}
function feedSignature(feeds = []) {
  return (Array.isArray(feeds) ? feeds : [])
    .map((feed) => `${String(feed?.conversationId || '').trim()}::${String(feed?.feedId || '').trim()}`)
    .join('|');
}
function useExpandedFeeds() {
  const [expanded, setExpanded] = useState(() => getExpandedFeedIds());
  useEffect(() => {
    return onFeedExpansionChange((next) => setExpanded(new Set(next)));
  }, []);
  return expanded;
}

export default function ToolFeedBar() {
  const [feeds, setFeeds] = useState(getActiveFeeds);
  const expanded = useExpandedFeeds();
  const [selectedFeedId, setSelectedFeedId] = useState('');
  const selectedWindow = getSelectedWindow();
  const currentConversationId = String(
    getScopedConversationSelection(String(selectedWindow?.windowId || '').trim())
    || ''
  ).trim();

  useEffect(() => {
    setSelectedFeedId(getSelectedFeedId(currentConversationId));
  }, [currentConversationId]);

  useEffect(() => onSelectedFeedChange(() => {
    setSelectedFeedId(getSelectedFeedId(currentConversationId));
  }), [currentConversationId]);

  useEffect(() => {
    return onFeedChange((next) => {
      setFeeds(next);
      reconcileFeedSelection(next);
      // Clear expand state when feeds are cleared (conversation switch).
      if (!next || next.length === 0) {
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
    const hasExpandedVisibleFeed = mergedFeeds.some((feed) => expanded.has(feed.feedId));
    if (hasExpandedVisibleFeed) return;
    const signature = feedSignature(mergedFeeds);
    if (autoExpandedFeedSignature === signature) return;
    const first = mergedFeeds[0];
    if (!first?.feedId) return;
    autoExpandedFeedSignature = signature;
    expandFeed(first.feedId, first.conversationId);
  }, [currentConversationId, expanded, mergedFeeds]);

  if (mergedFeeds.length === 0) return null;

  return (
    <div className="app-tool-feed-bar">
      {mergedFeeds.map((feed) => {
        const isExpanded = expanded.has(feed.feedId);
        const isSelected = feed.feedId === selectedFeedId;
        return (
          <div
            className={`app-tool-feed-bar-item${isSelected ? ' is-selected' : ''}`}
            key={feed.feedId}
            onClick={() => expandFeed(feed.feedId, feed.conversationId)}
            role="button"
            tabIndex={0}
          >
            <span className="app-tool-feed-bar-icon">{feedIcon(feed.rawFeedId || feed.feedId, feed.title || feed.feedId)}</span>
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
