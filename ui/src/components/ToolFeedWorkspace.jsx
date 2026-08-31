import React, { useEffect, useMemo, useState } from 'react';
import { Drawer, Icon } from '@blueprintjs/core';
import { fetchFeedDataNow, getActiveFeeds, onFeedChange } from '../services/toolFeedBus';
import {
  activateExclusiveFeed,
  clearFeedSelectionForConversation,
  getSelectedFeedId,
  onSelectedFeedChange,
  reconcileFeedSelection,
} from '../services/toolFeedSelection';
import { dedupeFeeds, feedAccent, feedIconName } from './ToolFeedBar';
import ToolFeedDetail from './ToolFeedDetail.jsx';
import { toolFeedTargetsPlacement } from '../services/toolFeedTarget';

export function sortWorkspaceFeeds(feeds = []) {
  return [...(Array.isArray(feeds) ? feeds : [])];
}

export function filterWorkspaceFeeds(feeds = [], conversationId = '', developerMode = false) {
  const scopedConversationId = String(conversationId || '').trim();
  const visible = (Array.isArray(feeds) ? feeds : []).filter((feed) => {
    if (feed?.developerOnly === true && !developerMode) return false;
    const feedConversationId = String(feed?.conversationId || '').trim();
    const conversationMatches = !feedConversationId || !scopedConversationId || feedConversationId === scopedConversationId;
    return conversationMatches && toolFeedTargetsPlacement(feed, 'workspace', true);
  });
  return sortWorkspaceFeeds(dedupeFeeds(visible));
}

export function isStackedToolFeedViewport(width) {
  const value = Number(width);
  return Number.isFinite(value) && value > 0 && value <= 1100;
}

export default function ToolFeedWorkspace({ conversationId = '', developerMode = false, initialDismissed = false, stackedOverride = null }) {
  const [feeds, setFeeds] = useState(getActiveFeeds);
  const [selectedFeedId, setSelectedFeedId] = useState(() => getSelectedFeedId(conversationId));
  const [collapsed, setCollapsed] = useState(false);
  const [expanded, setExpanded] = useState(false);
  const [dismissed, setDismissed] = useState(() => !!initialDismissed);
  const [compactDrawerOpen, setCompactDrawerOpen] = useState(false);
  const [stackedViewport, setStackedViewport] = useState(() => (
    stackedOverride == null && typeof window !== 'undefined'
      ? isStackedToolFeedViewport(window.innerWidth)
      : stackedOverride === true
  ));

  useEffect(() => {
    if (stackedOverride != null) {
      setStackedViewport(stackedOverride === true);
      return undefined;
    }
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return undefined;
    const query = window.matchMedia('(max-width: 1100px)');
    const update = () => setStackedViewport(query.matches);
    update();
    query.addEventListener?.('change', update);
    return () => query.removeEventListener?.('change', update);
  }, [stackedOverride]);

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
    () => filterWorkspaceFeeds(feeds, conversationId, developerMode),
    [conversationId, developerMode, feeds]
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
    if (dismissed || !selectedFeedId) return;
    const active = visibleFeeds.find((feed) => feed.feedId === selectedFeedId);
    if (!active) return;
    fetchFeedDataNow(active.feedId, active.conversationId || conversationId);
  }, [conversationId, dismissed, feedSignature, selectedFeedId]);

  useEffect(() => {
    if (visibleFeeds.length > 0) return;
    setCollapsed(false);
    setExpanded(false);
    setDismissed(false);
    setCompactDrawerOpen(false);
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
            aria-label={`Reopen Tool feeds (${visibleFeeds.length} active)`}
            title={`Reopen Tool feeds · ${visibleFeeds.length} active`}
            onClick={() => setDismissed(false)}
          >
            <span className="app-tool-workspace-reopen-icon" aria-hidden="true">↗</span>
            <span className="app-tool-workspace-reopen-count" aria-hidden="true">{visibleFeeds.length}</span>
          </button>
        </div>
      </aside>
    );
  }

  const selectFeed = (feed, openDrawer = false) => {
    activateExclusiveFeed(feed.feedId, feed.conversationId);
    if (openDrawer) setCompactDrawerOpen(true);
  };

  const tabs = (openDrawer = false) => (
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
            style={{ '--feed-accent': feedAccent(feed.presentation) }}
            onClick={() => selectFeed(feed, openDrawer)}
          >
            <span className="app-tool-workspace-tab-icon" aria-hidden="true"><Icon icon={feedIconName(feed.presentation)} size={13} /></span>
            <span className="app-tool-workspace-tab-label">{feed.title || feed.feedId}</span>
            {feed.itemCount > 0 ? <span className="app-tool-workspace-tab-count">{feed.itemCount}</span> : null}
          </button>
        );
      })}
    </div>
  );

  if (stackedViewport) {
    return (
      <>
        <aside className="app-tool-workspace is-compact-launcher" aria-label="Tool feeds">
          <div className="app-tool-workspace-card">
            <div className="app-tool-workspace-header">
              <div className="app-tool-workspace-title">Tool feeds</div>
              <button type="button" className="app-tool-workspace-open-drawer" onClick={() => setCompactDrawerOpen(true)} aria-label="Open Tool feeds drawer">Open</button>
            </div>
            {tabs(true)}
          </div>
        </aside>
        <Drawer
          isOpen={compactDrawerOpen}
          onClose={() => setCompactDrawerOpen(false)}
          position="right"
          size="min(420px, 100vw)"
          className="app-tool-workspace-drawer"
          portalClassName="app-tool-workspace-drawer-portal"
        >
          <section role="dialog" aria-modal="true" aria-label="Tool feeds" className="app-tool-workspace-drawer-content">
            <header className="app-tool-workspace-drawer-header">
              <div className="app-tool-workspace-title">Tool feeds</div>
              <button type="button" onClick={() => setCompactDrawerOpen(false)} aria-label="Close Tool feeds drawer">Close</button>
            </header>
            <div className="app-tool-workspace-drawer-tabs">{tabs(false)}</div>
            <div className="app-tool-workspace-drawer-body">
              <ToolFeedDetail variant="rail" placement="workspace" conversationId={conversationId} />
            </div>
          </section>
        </Drawer>
      </>
    );
  }

  return (
    <aside className={`app-tool-workspace${collapsed ? ' is-collapsed' : ''}${expanded ? ' is-expanded' : ''}`} aria-label="Tool workspace">
      <div className="app-tool-workspace-card">
        <div className="app-tool-workspace-header">
          <div className="app-tool-workspace-title">Tool feeds</div>
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
              aria-label={collapsed ? 'Show Tool feeds' : 'Collapse Tool feeds to header'}
              title={collapsed ? 'Show Tool feeds' : 'Collapse Tool feeds to header'}
              aria-pressed={collapsed}
              onClick={() => {
                setExpanded(false);
                setCollapsed((value) => !value);
              }}
            />
            <button
              type="button"
              className="app-tool-workspace-dot app-tool-workspace-dot--expand"
              aria-label={expanded ? 'Restore Conversation and Tool feeds' : 'Maximize Tool feeds'}
              title={expanded ? 'Restore Conversation and Tool feeds' : 'Maximize Tool feeds'}
              aria-pressed={expanded}
              onClick={() => {
                setCollapsed(false);
                setExpanded((value) => !value);
              }}
            />
          </div>
        </div>
        {!collapsed ? tabs(false) : null}
        {!collapsed ? (
          <div className="app-tool-workspace-body">
            <ToolFeedDetail variant="rail" placement="workspace" conversationId={conversationId} />
          </div>
        ) : null}
      </div>
    </aside>
  );
}
