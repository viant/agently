import React, { useState, useEffect } from 'react';
import { getActiveFeeds, onFeedChange, fetchFeedDataNow, getFeedData } from '../services/toolFeedBus';
import { getScopedConversationSelection, getSelectedWindow } from '../services/conversationWindow';

const FEED_ICONS = {
  plan: '📋',
  changes: '📁',
  terminal: '🖥',
  explorer: '🔍',
};

function feedIcon(feedId) {
  return FEED_ICONS[feedId] || '📊';
}

/**
 * Tracks which feeds are expanded (shown below execution details).
 */
let expandedFeeds = new Set();
const expandListeners = new Set();
function notifyExpand() {
  for (const fn of expandListeners) fn();
}
export function isFeedExpanded(feedId) {
  return expandedFeeds.has(feedId);
}
function toggleFeedExpanded(feedId, conversationId) {
  if (expandedFeeds.has(feedId)) {
    expandedFeeds.delete(feedId);
  } else {
    expandedFeeds.add(feedId);
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
      }
    });
  }, []);

  const visibleFeeds = (Array.isArray(feeds) ? feeds : []).filter((feed) => {
    const conversationId = String(feed?.conversationId || '').trim();
    if (!conversationId) return true;
    return conversationId === currentConversationId;
  });

  if (visibleFeeds.length === 0) return null;

  return (
    <div className="app-tool-feed-bar">
      {visibleFeeds.map((feed) => {
        const isExpanded = expanded.has(feed.feedId);
        return (
          <div
            className="app-tool-feed-bar-item"
            key={feed.feedId}
            onClick={() => toggleFeedExpanded(feed.feedId, feed.conversationId)}
            role="button"
            tabIndex={0}
          >
            <span className="app-tool-feed-bar-icon">{feedIcon(feed.feedId)}</span>
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
