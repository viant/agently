import React, { useEffect, useMemo, useState } from 'react';
import { Tabs, Tab } from '@blueprintjs/core';
import { Container as ForgeContainer } from 'forge/components';
import { getFeedData, onFeedDataChange, getActiveFeeds, onFeedChange, makeFeedKey, splitFeedKey } from '../services/toolFeedBus';
import { getSelectedFeedId, isFeedExpanded, onSelectedFeedChange } from './ToolFeedBar';
import { usePlanFeed } from '../services/planFeedBus';
import {
  wireFeedSignals,
  normalizeDataSources,
  computeDataMap,
  applyAutoTableColumns,
} from '../services/feedForgeWiring';
import { createFeedContext } from '../services/feedForgeContext';

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
 * Tabbed feed detail panel rendered below execution details in IterationBlock.
 * Uses Forge Container to render feed UI specs from YAML.
 * Falls back to generic InlineRenderer when no UI spec is present.
 * Single feed = no tab bar, just content.
 */
export default function ToolFeedDetail({ context }) {
  const [feeds, setFeeds] = useState(getActiveFeeds);
  const [dataVersion, setDataVersion] = useState(0);
  const [, setExpandVersion] = useState(0);
  const [selectedFeedId, setSelectedFeedId] = useState(getSelectedFeedId);
  const planFeed = usePlanFeed();

  useEffect(() => {
    const u1 = onFeedChange((next) => setFeeds(next));
    const u2 = onFeedDataChange(() => setDataVersion((n) => n + 1));
    const u3 = onSelectedFeedChange((next) => setSelectedFeedId(next || ''));
    // Poll expand state changes (shared global, no subscription).
    const interval = setInterval(() => setExpandVersion((n) => n + 1), 300);
    return () => { u1(); u2(); u3(); clearInterval(interval); };
  }, []);

  // Collect expanded feeds that have data.
  const planFeedKey = makeFeedKey('plan', String(planFeed?.conversationId || '').trim());
  const planData = getFeedData(planFeedKey, String(planFeed?.conversationId || '').trim());
  const hasPlanBusData = !!String(planFeed?.explanation || '').trim()
    || (Array.isArray(planFeed?.steps) && planFeed.steps.length > 0);
  const hasPlanPayloadData = !!String(planData?.data?.output?.explanation || planData?.data?.input?.explanation || '').trim()
    || Array.isArray(planData?.data?.output?.plan)
    || Array.isArray(planData?.data?.input?.plan);
  const hasPlanData = hasPlanBusData || hasPlanPayloadData;
  const visibleFeeds = dedupeFeeds([
    ...(hasPlanData && isFeedExpanded(planFeedKey) ? [{
      feedId: planFeedKey,
      title: 'Plan',
      rawFeedId: 'plan',
      conversationId: String(planFeed?.conversationId || '').trim(),
    }] : []),
    ...((feeds || []).filter((f) => isFeedExpanded(f.feedId) && getFeedData(f.feedId, f.conversationId))),
  ]);

  if (visibleFeeds.length === 0) return null;

  // Single feed: render directly, no tab bar.
  if (visibleFeeds.length === 1) {
    return (
      <div className="app-tool-feed-detail">
        <FeedPanel feedId={visibleFeeds[0].feedId} planFeed={planFeed} />
      </div>
    );
  }

  // Multiple feeds: tabbed.
  return (
    <div className="app-tool-feed-detail">
      <Tabs
        id="tool-feed-tabs"
        renderActiveTabPanelOnly
        selectedTabId={visibleFeeds.some((feed) => feed.feedId === selectedFeedId) ? selectedFeedId : visibleFeeds[0].feedId}
      >
        {visibleFeeds.map((feed) => (
          <Tab
            key={feed.feedId}
            id={feed.feedId}
            title={feed.title || feed.feedId}
            panel={<FeedPanel feedId={feed.feedId} rawFeedId={feed.rawFeedId || splitFeedKey(feed.feedId).feedId} planFeed={planFeed} />}
          />
        ))}
      </Tabs>
    </div>
  );
}

export function resolveRootFeedDataSourceName(dataSources = {}) {
  const dsNames = Object.keys(dataSources || {});
  return dsNames.find((name) => {
    const ds = dataSources[name] || {};
    const source = String(ds?.source || '').trim().toLowerCase();
    return !String(ds?.dataSourceRef || '').trim() && (source === 'output' || source === 'input');
  }) || dsNames.find((name) => !String(dataSources[name]?.dataSourceRef || '').trim()) || dsNames[0] || '';
}

/**
 * Renders a single feed panel. Uses Forge Container when the feed spec
 * includes a `ui` definition; falls back to InlineRenderer otherwise.
 */
function FeedPanel({ feedId, rawFeedId, planFeed }) {
  const scopedConversationId = String(splitFeedKey(feedId).conversationId || '').trim();
  const data = getFeedData(feedId, scopedConversationId);
  if ((rawFeedId || splitFeedKey(feedId).feedId) === 'plan') {
    return <PlanFeedPanel planFeed={planFeed} feedData={data} />;
  }

  // Build the execution shape that wireFeedSignals expects.
  const exe = useMemo(() => {
    if (!data) return null;
    const rawDS = data.ui && typeof data.ui === 'object'
      ? (data.ui.dataSources || data.dataSources || {})
      : (data.dataSources || {});
    const dsNormalized = normalizeDataSources(rawDS);

    // Determine the root data source name and its data.
    const rootName = resolveRootFeedDataSourceName(dsNormalized);

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

  // Build Forge context for this feed.
  const conversationId = data?._conversationId || scopedConversationId || '';
  const forgeContext = useMemo(
    () => createFeedContext(feedId, exe?.dataSources || {}, conversationId),
    [conversationId, exe?.dataSources, feedId]
  );

  // Wire Forge signals whenever data changes.
  useEffect(() => {
    if (!exe || !uiContainer) { return; }
    const timer = setTimeout(() => {
      const windowId = conversationId ? `feed-${feedId}-${conversationId}` : `feed-${feedId}`;
      wireFeedSignals(exe, windowId);
      const dataMap = computeDataMap(exe);
      for (const [dsRef, rows] of Object.entries(dataMap || {})) {
        try {
          forgeContext.Context(dsRef)?.handlers?.dataSource?.setCollection?.(Array.isArray(rows) ? rows : []);
        } catch (_) {}
      }
    }, 0);
    return () => clearTimeout(timer);
  }, [exe, uiContainer, feedId, data, forgeContext]);

  if (!data) return null;

  const hasFullFeedSpec = !!(data?.ui && data?.dataSources);

  if (!hasFullFeedSpec) {
    return <div style={{ padding: 8, color: 'var(--gray2)' }}>Loading feed…</div>;
  }

  // No UI spec → fall back to generic InlineRenderer.
  if (!uiContainer) {
    return <InlineRenderer data={data} />;
  }

  return (
    <div className="app-tool-feed-detail-list" style={{ maxHeight: 'min(42vh, 360px)', overflowY: 'auto', paddingRight: 4 }}>
      <ForgeContainer container={uiContainer} context={forgeContext} />
    </div>
  );
}

function normalizePlanFeedData(planFeed = {}, feedData = null) {
  const currentExplanation = String(planFeed?.explanation || '').trim();
  const currentSteps = Array.isArray(planFeed?.steps) ? planFeed.steps : [];
  if (currentExplanation || currentSteps.length > 0) {
    return { explanation: currentExplanation, steps: currentSteps };
  }
  const payload = feedData?.data || {};
  const source = payload?.output || payload?.input || payload || {};
  const explanation = String(source?.explanation || source?.Explanation || '').trim();
  const rawSteps = Array.isArray(source?.plan || source?.Plan) ? (source.plan || source.Plan) : [];
  const steps = rawSteps
    .map((step, index) => {
      const title = String(step?.step || step?.title || step?.name || '').trim();
      if (!title) return null;
      return {
        id: String(step?.id || `${index}:${title}`),
        step: title,
        status: String(step?.status || '').trim().toLowerCase() || 'pending'
      };
    })
    .filter(Boolean);
  return { explanation, steps };
}

function PlanFeedPanel({ planFeed, feedData }) {
  const normalized = normalizePlanFeedData(planFeed, feedData);
  const explanation = String(normalized?.explanation || '').trim();
  const steps = Array.isArray(normalized?.steps) ? normalized.steps : [];
  if (!explanation && steps.length === 0) return null;
  return (
    <div className="app-tool-feed-detail-list" style={{ maxHeight: 'min(42vh, 360px)', overflowY: 'auto', paddingRight: 4 }}>
      {explanation ? (
        <div className="app-tool-feed-detail-explanation" style={{ marginBottom: 8 }}>
          {explanation}
        </div>
      ) : null}
      {steps.map((step, index) => {
        const status = String(step?.status || '').trim().toLowerCase();
        const icon = status === 'completed' || status === 'done' ? '✓'
          : status === 'in_progress' || status === 'running' ? '▶'
          : '○';
        return (
          <div className="app-tool-feed-detail-step" key={step?.id || `${index}:${step?.step || 'plan'}`}>
            <span className={`app-tool-feed-detail-status status-${status || 'pending'}`}>{icon}</span>
            <span style={{ flex: 1 }}>{String(step?.step || '').trim()}</span>
          </div>
        );
      })}
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
