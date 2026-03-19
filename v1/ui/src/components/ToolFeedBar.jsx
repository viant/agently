import React, { useState, useEffect } from 'react';
import { getActiveFeeds, onFeedChange } from '../services/toolFeedBus';

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
function toggleFeedExpanded(feedId) {
  if (expandedFeeds.has(feedId)) {
    expandedFeeds.delete(feedId);
  } else {
    expandedFeeds.add(feedId);
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

  useEffect(() => {
    return onFeedChange((next) => setFeeds(next));
  }, []);

  if (!feeds || feeds.length === 0) return null;

  return (
    <div className="app-tool-feed-bar">
      {feeds.map((feed) => {
        const isExpanded = expanded.has(feed.feedId);
        return (
          <div
            className="app-tool-feed-bar-item"
            key={feed.feedId}
            onClick={() => toggleFeedExpanded(feed.feedId)}
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
