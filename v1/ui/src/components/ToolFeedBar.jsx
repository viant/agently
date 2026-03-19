import React, { useState, useEffect, useCallback } from 'react';
import { getActiveFeeds, onFeedChange } from '../services/toolFeedBus';
import { client } from '../services/agentlyClient';

const FEED_ICONS = {
  plan: '📋',
  changes: '📁',
  terminal: '🖥',
  explorer: '🔍',
};

function feedIcon(feedId) {
  return FEED_ICONS[feedId] || '📊';
}

export default function ToolFeedBar() {
  const [feeds, setFeeds] = useState(getActiveFeeds);
  const [expanded, setExpanded] = useState({});
  const [feedData, setFeedData] = useState({});

  useEffect(() => {
    return onFeedChange((next) => {
      setFeeds(next);
      // Remove expanded state for feeds that went inactive.
      setExpanded((prev) => {
        const activeIds = new Set(next.map((f) => f.feedId));
        const cleaned = {};
        for (const [id, val] of Object.entries(prev)) {
          if (activeIds.has(id)) cleaned[id] = val;
        }
        return cleaned;
      });
    });
  }, []);

  const toggleExpand = useCallback(async (feed) => {
    const id = feed.feedId;
    setExpanded((prev) => {
      const next = { ...prev, [id]: !prev[id] };
      // Fetch data on first expand.
      if (next[id] && !feedData[id] && feed.conversationId) {
        client.getFeedData(id, feed.conversationId).then((data) => {
          setFeedData((prev) => ({ ...prev, [id]: data }));
        }).catch(() => {});
      }
      return next;
    });
  }, [feedData]);

  if (!feeds || feeds.length === 0) return null;

  return (
    <div className="app-tool-feed-bar">
      {feeds.map((feed) => {
        const isExpanded = !!expanded[feed.feedId];
        return (
          <div className="app-tool-feed-bar-entry" key={feed.feedId}>
            <div
              className="app-tool-feed-bar-item"
              onClick={() => toggleExpand(feed)}
              role="button"
              tabIndex={0}
            >
              <span className="app-tool-feed-bar-icon">{feedIcon(feed.feedId)}</span>
              <span className="app-tool-feed-bar-title">{feed.title || feed.feedId}</span>
              {feed.itemCount > 0 ? (
                <span className="app-tool-feed-bar-badge">{feed.itemCount}</span>
              ) : null}
              <span className="app-tool-feed-bar-expand">{isExpanded ? '▴' : '▾'}</span>
            </div>
            {isExpanded && feedData[feed.feedId] ? (
              <div className="app-tool-feed-bar-detail">
                <FeedDetail data={feedData[feed.feedId]} />
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

/** Renders feed detail content based on feed type. */
function FeedDetail({ data }) {
  if (!data) return null;
  const output = data?.data?.output || data?.data || {};

  // Plan feed: show task list.
  const plan = Array.isArray(output?.plan) ? output.plan : [];
  if (plan.length > 0) {
    return (
      <div className="app-tool-feed-detail-list">
        {plan.map((step, i) => (
          <div className="app-tool-feed-detail-step" key={step?.id || i}>
            <span className="app-tool-feed-detail-status">
              {String(step?.status || '').toLowerCase() === 'completed' ? '✓' : '○'}
            </span>
            <span>{step?.step || step?.title || step?.name || `Step ${i + 1}`}</span>
          </div>
        ))}
      </div>
    );
  }

  // Terminal feed: show commands.
  const commands = Array.isArray(output?.commands) ? output.commands : [];
  if (commands.length > 0) {
    return (
      <div className="app-tool-feed-detail-terminal">
        {commands.map((cmd, i) => (
          <div key={i} className="app-tool-feed-detail-cmd">
            <code>{cmd?.input || cmd?.command || JSON.stringify(cmd)}</code>
          </div>
        ))}
      </div>
    );
  }

  // Changes feed: show file list.
  const changes = Array.isArray(output?.changes) ? output.changes : [];
  if (changes.length > 0) {
    return (
      <div className="app-tool-feed-detail-list">
        {changes.map((change, i) => (
          <div key={i} className="app-tool-feed-detail-step">
            <span>{change?.path || change?.file || JSON.stringify(change)}</span>
          </div>
        ))}
      </div>
    );
  }

  // Generic: show JSON summary.
  const keys = Object.keys(output);
  if (keys.length > 0) {
    return (
      <div className="app-tool-feed-detail-list">
        {keys.slice(0, 10).map((key) => (
          <div key={key} className="app-tool-feed-detail-step">
            <span style={{ fontWeight: 500 }}>{key}:</span>{' '}
            <span>{typeof output[key] === 'object' ? JSON.stringify(output[key]).slice(0, 80) : String(output[key]).slice(0, 80)}</span>
          </div>
        ))}
      </div>
    );
  }

  return <div className="app-tool-feed-detail-empty">No data</div>;
}
