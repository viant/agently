import React, { useEffect, useMemo, useState } from 'react';
import { getActiveFeeds, onFeedChange } from '../services/toolFeedBus';
import {
  activateExclusiveFeed,
  clearFeedSelectionForConversation,
  getSelectedFeedId,
  onSelectedFeedChange,
  reconcileFeedSelection,
} from '../services/toolFeedSelection';
import { dedupeFeeds, feedIcon } from './ToolFeedBar';
import ToolFeedDetail from './ToolFeedDetail.jsx';

export function sortWorkspaceFeeds(feeds = []) {
  return [...(Array.isArray(feeds) ? feeds : [])];
}

export function filterWorkspaceFeeds(feeds = [], conversationId = '') {
  const scopedConversationId = String(conversationId || '').trim();
  const visible = (Array.isArray(feeds) ? feeds : []).filter((feed) => {
    const feedConversationId = String(feed?.conversationId || '').trim();
    if (!feedConversationId) return true;
    return !scopedConversationId || feedConversationId === scopedConversationId;
  });
  return sortWorkspaceFeeds(dedupeFeeds(visible));
}

export default function ToolFeedWorkspace({ conversationId = '', initialDismissed = false }) {
  const [feeds, setFeeds] = useState(getActiveFeeds);
  const [selectedFeedId, setSelectedFeedId] = useState(() => getSelectedFeedId(conversationId));
  const [collapsed, setCollapsed] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [dismissed, setDismissed] = useState(() => !!initialDismissed);

  useEffect(() => onFeedChange((next) => {
    setFeeds(next);
    reconcileFeedSelection(next);
  }), []);
  useEffect(() => {
    setSelectedFeedId(getSelectedFeedId(conversationId));
  }, [conversationId]);
  useEffect(() => onSelectedFeedChange(() => {
    setSelectedFeedId(getSelectedFeedId(conversationId));
  }), [conversationId]);

  const visibleFeeds = useMemo(
    () => filterWorkspaceFeeds(feeds, conversationId),
    [conversationId, feeds]
  );
  const feedSignature = useMemo(
    () => visibleFeeds.map((feed) => String(feed?.feedId || '').trim()).filter(Boolean).join('|'),
    [visibleFeeds]
  );

  useEffect(() => {
    if (dismissed) return;
    if (visibleFeeds.length === 0) return;
    const active = visibleFeeds.find((feed) => feed.feedId === selectedFeedId);
    if (active) return;
    const first = visibleFeeds[0];
    if (!first?.feedId) return;
    activateExclusiveFeed(first.feedId, first.conversationId);
  }, [dismissed, selectedFeedId, visibleFeeds]);

  useEffect(() => {
    if (visibleFeeds.length > 0) return;
    setCollapsed(false);
    setExpanded(false);
    setDismissed(false);
  }, [visibleFeeds.length]);

  useEffect(() => {
    setDismissed(false);
  }, [conversationId, feedSignature]);

  if (visibleFeeds.length === 0) {
    return null;
  }

  if (dismissed) {
    return (
      <aside className="app-tool-workspace is-dismissed" aria-label="Tool workspace">
        <div className="app-tool-workspace-card app-tool-workspace-card--dismissed">
          <button
            type="button"
            className="app-tool-workspace-reopen"
            aria-label="Reopen tool feeds"
            title="Reopen tool feeds"
            onClick={() => setDismissed(false)}
          >
            <span className="app-tool-workspace-reopen-icon" aria-hidden="true">↗</span>
            <span className="app-tool-workspace-reopen-copy">
              <span className="app-tool-workspace-reopen-label">Reopen tool feeds</span>
              <span className="app-tool-workspace-reopen-meta">
                {visibleFeeds.length} active
              </span>
            </span>
          </button>
        </div>
      </aside>
    );
  }

  return (
    <aside className={`app-tool-workspace${collapsed ? ' is-collapsed' : ''}${expanded ? ' is-expanded' : ''}`} aria-label="Tool workspace">
      <div className="app-tool-workspace-card">
        <div className="app-tool-workspace-header">
          <div>
            <div className="app-tool-workspace-eyebrow">Tool workspace</div>
            <div className="app-tool-workspace-title">Tool feeds</div>
          </div>
          <div className="app-tool-workspace-header-actions">
            <button
              type="button"
              className="app-tool-workspace-dot app-tool-workspace-dot--close"
              aria-label="Close tool feeds"
              title="Close tool feeds"
              onClick={() => {
                clearFeedSelectionForConversation(conversationId);
                setDismissed(true);
              }}
            />
            <button
              type="button"
              className="app-tool-workspace-dot app-tool-workspace-dot--collapse"
              aria-label={collapsed ? 'Show tool feed body' : 'Collapse tool feeds to tabs'}
              title={collapsed ? 'Show tool feed body' : 'Collapse tool feeds to tabs'}
              aria-pressed={collapsed}
              onClick={() => setCollapsed((value) => !value)}
            />
            <button
              type="button"
              className="app-tool-workspace-dot app-tool-workspace-dot--expand"
              aria-label={expanded ? 'Restore tool feed width' : 'Expand tool feed width'}
              title={expanded ? 'Restore tool feed width' : 'Expand tool feed width'}
              aria-pressed={expanded}
              onClick={() => setExpanded((value) => !value)}
            />
          </div>
        </div>
        <div className="app-tool-workspace-tabs" role="tablist" aria-label="Tool feeds">
          {visibleFeeds.map((feed) => {
            const active = feed.feedId === selectedFeedId;
            return (
              <button
                key={feed.feedId}
                type="button"
                role="tab"
                aria-selected={active}
                className={`app-tool-workspace-tab${active ? ' is-active' : ''}`}
                onClick={() => activateExclusiveFeed(feed.feedId, feed.conversationId)}
              >
                <span className="app-tool-workspace-tab-icon" aria-hidden="true">{feedIcon(feed.rawFeedId || feed.feedId, feed.title || feed.feedId)}</span>
                <span className="app-tool-workspace-tab-label">{feed.title || feed.feedId}</span>
                {feed.itemCount > 0 ? <span className="app-tool-workspace-tab-count">{feed.itemCount}</span> : null}
              </button>
            );
          })}
        </div>
        {!collapsed ? (
          <div className="app-tool-workspace-body">
            <ToolFeedDetail variant="rail" conversationId={conversationId} />
          </div>
        ) : null}
      </div>
    </aside>
  );
}
