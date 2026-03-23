import React, { useEffect, useMemo, useState } from 'react';
import { Tabs, Tab } from '@blueprintjs/core';
import { Container as ForgeContainer } from 'forge/components';
import { getFeedData, onFeedDataChange, getActiveFeeds, onFeedChange } from '../services/toolFeedBus';
import { isFeedExpanded } from './ToolFeedBar';
import {
  wireFeedSignals,
  normalizeDataSources,
  computeDataMap,
  applyAutoTableColumns,
} from '../services/feedForgeWiring';
import { createFeedContext } from '../services/feedForgeContext';

/**
 * Tabbed feed detail panel rendered below execution details in IterationBlock.
 * Uses Forge Container to render feed UI specs from YAML.
 * Falls back to generic InlineRenderer when no UI spec is present.
 * Single feed = no tab bar, just content.
 */
export default function ToolFeedDetail({ context }) {
  const [feeds, setFeeds] = useState(getActiveFeeds);
  const [dataVersion, setDataVersion] = useState(0);
  const [, setExpandVersion] = useState(0);

  useEffect(() => {
    const u1 = onFeedChange((next) => setFeeds(next));
    const u2 = onFeedDataChange(() => setDataVersion((n) => n + 1));
    // Poll expand state changes (shared global, no subscription).
    const interval = setInterval(() => setExpandVersion((n) => n + 1), 300);
    return () => { u1(); u2(); clearInterval(interval); };
  }, []);

  // Collect expanded feeds that have data.
  const visibleFeeds = (feeds || []).filter((f) => isFeedExpanded(f.feedId) && getFeedData(f.feedId));

  if (visibleFeeds.length === 0) return null;

  // Single feed: render directly, no tab bar.
  if (visibleFeeds.length === 1) {
    return (
      <div className="app-tool-feed-detail">
        <FeedPanel feedId={visibleFeeds[0].feedId} />
      </div>
    );
  }

  // Multiple feeds: tabbed.
  return (
    <div className="app-tool-feed-detail">
      <Tabs id="tool-feed-tabs" renderActiveTabPanelOnly>
        {visibleFeeds.map((feed) => (
          <Tab
            key={feed.feedId}
            id={feed.feedId}
            title={feed.title || feed.feedId}
            panel={<FeedPanel feedId={feed.feedId} />}
          />
        ))}
      </Tabs>
    </div>
  );
}

/**
 * Renders a single feed panel. Uses Forge Container when the feed spec
 * includes a `ui` definition; falls back to InlineRenderer otherwise.
 */
function FeedPanel({ feedId }) {
  const data = getFeedData(feedId);
  const [ready, setReady] = useState(false);

  // Build the execution shape that wireFeedSignals expects.
  const exe = useMemo(() => {
    if (!data) return null;
    const rawDS = data.ui && typeof data.ui === 'object'
      ? (data.ui.dataSources || data.dataSources || {})
      : (data.dataSources || {});
    const dsNormalized = normalizeDataSources(rawDS);

    // Determine the root data source name and its data.
    const dsNames = Object.keys(dsNormalized);
    const rootName = dsNames[0] || '';

    return {
      id: feedId,
      dataSources: dsNormalized,
      dataFeed: { name: rootName, data: data.data },
      ui: data.ui,
    };
  }, [data, feedId]);

  // Prepare UI container with auto-columns and data source defs merged.
  const uiContainer = useMemo(() => {
    if (!exe || !exe.ui || typeof exe.ui !== 'object') return null;
    const uiClone = JSON.parse(JSON.stringify(exe.ui));
    uiClone.dataSources = exe.dataSources;
    const dataMap = computeDataMap(exe);
    applyAutoTableColumns(uiClone, dataMap);
    return uiClone;
  }, [exe]);

  // Wire Forge signals whenever data changes.
  useEffect(() => {
    if (!exe || !uiContainer) { setReady(false); return; }
    const conversationId = data?._conversationId || '';
    const windowId = conversationId ? `feed-${feedId}-${conversationId}` : `feed-${feedId}`;
    wireFeedSignals(exe, windowId);
    setReady(true);
  }, [exe, uiContainer, feedId, data]);

  if (!data) return null;

  // No UI spec → fall back to generic InlineRenderer.
  if (!uiContainer) {
    return <InlineRenderer data={data} />;
  }

  // Build Forge context for this feed.
  const conversationId = data?._conversationId || '';
  const forgeContext = createFeedContext(feedId, exe?.dataSources || {}, conversationId);

  if (!ready) {
    return <div style={{ padding: 8, color: 'var(--gray2)' }}>Initializing data sources…</div>;
  }

  return (
    <div className="app-tool-feed-detail-list" style={{ maxHeight: '18vh', overflowY: 'auto' }}>
      <ForgeContainer container={uiContainer} context={forgeContext} />
    </div>
  );
}

/**
 * Generic data-driven renderer — inspects the data shape and renders
 * accordingly. Used as fallback when no Forge UI spec is defined.
 */
function InlineRenderer({ data }) {
  if (!data) return null;
  const root = data?.data || {};
  // Flatten: if root has a single "output" key, use that as the display root.
  const display = (root.output && typeof root.output === 'object' && !Array.isArray(root.output))
    ? root.output
    : root;

  return (
    <div className="app-tool-feed-detail-list" style={{ maxHeight: '18vh', overflowY: 'auto' }}>
      <AutoRender value={display} depth={0} />
    </div>
  );
}

/** Recursively renders any JSON value — auto-paginates large arrays. */
function AutoRender({ value, depth = 0, label }) {
  if (value == null) return null;

  // Array → paginated list.
  if (Array.isArray(value)) {
    if (value.length === 0) return null;
    return <PaginatedList items={value} depth={depth} label={label} />;
  }

  // Object with identifiable row shape → render as a row.
  if (typeof value === 'object') {
    const keys = Object.keys(value);
    const statusField = value.status || value.Status;
    const titleField = value.step || value.title || value.name || value.Name
      || value.path || value.Path || value.uri || value.URI;
    if (titleField) {
      const status = String(statusField || '').trim().toLowerCase();
      const icon = status === 'completed' || status === 'done' ? '✓'
        : status === 'in_progress' || status === 'running' ? '▶'
        : status ? '○' : '';
      const secondary = value.Matches || value.matches || value.hits || value.size || value.count;
      return (
        <div className="app-tool-feed-detail-step">
          {icon ? <span className={`app-tool-feed-detail-status status-${status || 'pending'}`}>{icon}</span> : null}
          <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {String(titleField)}
          </span>
          {secondary != null ? <span style={{ color: '#6b7280', fontSize: 11, flexShrink: 0 }}>{secondary}</span> : null}
        </div>
      );
    }
    // Nested arrays → render only the arrays, skip scalar metadata.
    const arrayKeys = keys.filter((k) => Array.isArray(value[k]) && value[k].length > 0);
    if (arrayKeys.length > 0 && depth < 2) {
      return (
        <>
          {arrayKeys.map((k) => (
            <AutoRender key={k} value={value[k]} depth={depth + 1} label={arrayKeys.length > 1 ? k : undefined} />
          ))}
        </>
      );
    }
    // Flat object summary.
    return (
      <div className="app-tool-feed-detail-step">
        <span style={{ color: '#6b7280', fontSize: 11 }}>
          {keys.slice(0, 8).map((k) => {
            const v = value[k];
            const d = Array.isArray(v) ? `${v.length} items` : typeof v === 'object' ? '{…}' : String(v).slice(0, 50);
            return `${k}: ${d}`;
          }).join(' · ')}
        </span>
      </div>
    );
  }

  // Scalar.
  return (
    <div className="app-tool-feed-detail-step">
      {label ? <span style={{ fontWeight: 500 }}>{label}: </span> : null}
      <span>{String(value).slice(0, 200)}</span>
    </div>
  );
}

const PAGE_SIZE = 10;

/** Renders an array with automatic pagination when > PAGE_SIZE items. */
function PaginatedList({ items, depth, label }) {
  const [page, setPage] = useState(0);
  const total = items.length;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));
  const needsPagination = total > PAGE_SIZE;
  const start = page * PAGE_SIZE;
  const slice = needsPagination ? items.slice(start, start + PAGE_SIZE) : items;

  return (
    <div>
      {label ? (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '2px 0', marginBottom: 2 }}>
          <span style={{ fontWeight: 600, fontSize: 12, color: '#4a5568' }}>{label} ({total})</span>
          {needsPagination ? (
            <span style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 11, color: '#6b7280' }}>
              <button className="app-tool-feed-page-btn" disabled={page <= 0} onClick={() => setPage(page - 1)}>‹</button>
              {page + 1}/{totalPages}
              <button className="app-tool-feed-page-btn" disabled={page >= totalPages - 1} onClick={() => setPage(page + 1)}>›</button>
            </span>
          ) : null}
        </div>
      ) : needsPagination ? (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', padding: '2px 0', marginBottom: 2 }}>
          <span style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 11, color: '#6b7280' }}>
            {total} items
            <button className="app-tool-feed-page-btn" disabled={page <= 0} onClick={() => setPage(page - 1)}>‹</button>
            {page + 1}/{totalPages}
            <button className="app-tool-feed-page-btn" disabled={page >= totalPages - 1} onClick={() => setPage(page + 1)}>›</button>
          </span>
        </div>
      ) : null}
      {slice.map((item, i) => (
        <AutoRender key={start + i} value={item} depth={depth + 1} />
      ))}
    </div>
  );
}
