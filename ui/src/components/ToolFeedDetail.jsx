import React, { useEffect, useRef, useState } from 'react';
import { CompactFeedList, Container, Terminal } from 'forge/components';
import { getFeedData, onFeedDataChange, getActiveFeeds, onFeedChange, splitFeedKey } from '../services/toolFeedBus';
import { openResourceFeedPath } from '../services/chatService';
import {
  getExpandedFeedIds,
  getSelectedFeedId,
  onFeedExpansionChange,
  onSelectedFeedChange
} from '../services/toolFeedSelection';
import {
  applyAutoTableColumns,
  asArray,
  computeDataMap,
  normalizeDataSources,
  selectPath,
  wireFeedSignals
} from '../services/feedForgeWiring';
import { createFeedContext } from '../services/feedForgeContext';
import { normalizeFeedPayload } from '../services/toolFeedBus';

function dedupeFeeds(feeds = []) {
  const seen = new Map();
  for (const feed of Array.isArray(feeds) ? feeds : []) {
    const id = String(feed?.feedId || '').trim();
    if (!id) continue;
    seen.set(id, { ...(seen.get(id) || {}), ...(feed || {}) });
  }
  return Array.from(seen.values());
}

function hasRenderableFeedData(data = null) {
  if (!data || typeof data !== 'object') return false;
  const root = data?.data;
  if (root == null) return false;
  if (Array.isArray(root)) return root.length > 0;
  if (typeof root !== 'object') return String(root).trim() !== '';
  const output = root?.output;
  if (Array.isArray(output)) return output.length > 0;
  if (output && typeof output === 'object') return Object.keys(output).length > 0;
  return Object.keys(root).length > 0;
}

function resolveFeedDetailConversationId(explicitConversationId = '', context = null) {
  const provided = String(explicitConversationId || '').trim();
  if (provided) return provided;
  const fromConversationDS = String(
    context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.()?.id || ''
  ).trim();
  if (fromConversationDS) return fromConversationDS;
  return '';
}

/**
 * Expanded feed detail panel rendered below execution details in IterationBlock.
 * Uses Forge Container to render feed UI specs from YAML.
 * Falls back to generic InlineRenderer when no UI spec is present.
 */
export default function ToolFeedDetail({ context, variant = 'inline', conversationId = '' }) {
  const [feeds, setFeeds] = useState(getActiveFeeds);
  const [dataVersion, setDataVersion] = useState(0);
  const [expandedFeeds, setExpandedFeeds] = useState(() => getExpandedFeedIds());
  const scopedConversationId = resolveFeedDetailConversationId(conversationId, context);
  const [selectedFeedId, setSelectedFeedId] = useState(() => getSelectedFeedId(scopedConversationId));
  const [isOverflowing, setIsOverflowing] = useState(false);
  const [isExpanded, setIsExpanded] = useState(false);
  const bodyRef = useRef(null);
  const collapsedHeight = variant === 'rail' ? 280 : 180;

  useEffect(() => {
    setSelectedFeedId(getSelectedFeedId(scopedConversationId));
  }, [scopedConversationId]);

  useEffect(() => {
    const u1 = onFeedChange((next) => setFeeds(next));
    const u2 = onFeedDataChange(() => setDataVersion((n) => n + 1));
    const u3 = onSelectedFeedChange(() => setSelectedFeedId(getSelectedFeedId(scopedConversationId)));
    const u4 = onFeedExpansionChange((next) => setExpandedFeeds(new Set(next)));
    return () => { u1(); u2(); u3(); u4(); };
  }, [scopedConversationId]);

  // Collect expanded feeds that have data.
  const candidateFeeds = dedupeFeeds((feeds || []).filter((feed) => {
    const feedConversationId = String(feed?.conversationId || '').trim();
    if (scopedConversationId && feedConversationId && feedConversationId !== scopedConversationId) {
      return false;
    }
    return !!getFeedData(feed.feedId, feed.conversationId);
  }));
  const hasAnyExpandedFeed = candidateFeeds.some((feed) => expandedFeeds.has(feed.feedId));
  const visibleFeeds = hasAnyExpandedFeed
    ? candidateFeeds.filter((feed) => expandedFeeds.has(feed.feedId))
    : [];
  const renderableFeeds = visibleFeeds.filter((feed) => {
    const data = getFeedData(feed.feedId, feed.conversationId);
    if (!data) return false;
    const rawDS = data.ui && typeof data.ui === 'object'
      ? (data.ui.dataSources || data.dataSources || {})
      : (data.dataSources || {});
    if (data?.ui && rawDS && Object.keys(rawDS).length > 0) return true;
    return hasRenderableFeedData(data);
  });

  useEffect(() => {
    setIsExpanded(false);
  }, [selectedFeedId, visibleFeeds.map((feed) => feed.feedId).join('|'), dataVersion]);

  useEffect(() => {
    if (typeof window === 'undefined') return undefined;
    const element = bodyRef.current;
    if (!element) {
      setIsOverflowing(false);
      return undefined;
    }
    const measure = () => {
      const nextOverflowing = element.scrollHeight > collapsedHeight + 4 || element.scrollWidth > element.clientWidth + 4;
      setIsOverflowing(nextOverflowing);
      if (!nextOverflowing) {
        setIsExpanded(false);
      }
    };
    const frame = window.requestAnimationFrame(measure);
    let observer = null;
    if (typeof window.ResizeObserver === 'function') {
      observer = new window.ResizeObserver(measure);
      observer.observe(element);
    }
    return () => {
      window.cancelAnimationFrame(frame);
      observer?.disconnect();
    };
  }, [collapsedHeight, dataVersion, selectedFeedId, visibleFeeds.map((feed) => feed.feedId).join('|')]);

  if (renderableFeeds.length === 0) return null;

  return (
    <div className={`app-tool-feed-detail app-tool-feed-detail--${variant}${isOverflowing ? ' has-overflow' : ''}${isExpanded ? ' is-expanded' : ' is-collapsed'}`}>
      <div ref={bodyRef} className="app-tool-feed-detail-body">
        {renderableFeeds.map((feed) => (
          <section
            key={feed.feedId}
            className={`app-tool-feed-detail-section${feed.feedId === selectedFeedId ? ' is-selected' : ''}`}
            data-feed-id={feed.feedId}
          >
            {renderableFeeds.length > 1 ? (
              <div className="app-tool-feed-detail-section-header">
                <span className="app-tool-feed-detail-section-title">{feed.title || feed.feedId}</span>
                {feed.itemCount > 0 ? <span className="app-tool-feed-detail-section-badge">{feed.itemCount}</span> : null}
              </div>
            ) : null}
            <FeedPanel
              feedId={feed.feedId}
              rawFeedId={feed.rawFeedId || splitFeedKey(feed.feedId).feedId}
              context={context}
              variant={variant}
            />
          </section>
        ))}
      </div>
      {isOverflowing && variant !== 'rail' ? (
        <div className="app-tool-feed-detail-footer">
          <button
            type="button"
            className="app-tool-feed-detail-toggle"
            onClick={() => setIsExpanded((value) => !value)}
          >
            {isExpanded ? 'Collapse' : 'Expand'}
          </button>
        </div>
      ) : null}
    </div>
  );
}

function FeedPanel({ feedId, context, variant = 'inline' }) {
  const scopedConversationId = String(splitFeedKey(feedId).conversationId || '').trim();
  const rawFeedId = String(splitFeedKey(feedId).feedId || '').trim();
  const data = normalizeFeedPayload(getFeedData(feedId, scopedConversationId));
  if (!data) return null;
  if (!hasRenderableFeedData(data)) return null;
  const onPathActivate = rawFeedId === 'resources'
    ? (row) => openResourceFeedPath({ row, context })
    : null;
  if (String(data?.ui?.renderMode || data?.renderMode || '').trim().toLowerCase() === 'forge') {
    return (
      <ForgeFeedRenderer
        data={data}
        feedId={rawFeedId || feedId}
        conversationId={scopedConversationId}
        variant={variant}
      />
    );
  }
  return <InlineRenderer data={data} variant={variant} onPathActivate={onPathActivate} />;
}

function cloneFeedNode(node) {
  if (!node || typeof node !== 'object') return node;
  try {
    return JSON.parse(JSON.stringify(node));
  } catch (_) {
    return { ...node };
  }
}

function buildForgeFeedContainer(feedId = '', payload = {}, dataMap = {}) {
  const ui = (payload?.ui && typeof payload.ui === 'object') ? payload.ui : {};
  const rootContainers = Array.isArray(ui.containers) ? cloneFeedNode(ui.containers) : [];
  const rootItems = Array.isArray(ui.items) ? cloneFeedNode(ui.items) : [];
  const dsRefs = Object.keys((ui.dataSources && typeof ui.dataSources === 'object')
    ? ui.dataSources
    : ((payload?.dataSources && typeof payload.dataSources === 'object') ? payload.dataSources : {}));
  const defaultDataSourceRef = dsRefs[0] || '';

  let container = null;
  const hasTopLevelVisuals = !!(ui.toolbar || ui.table || ui.chart || ui.chat || ui.terminal || ui.fileBrowser || ui.treeBrowser || ui.editor || ui.schemaBasedForm || ui.layout || rootItems.length > 0);
  if (rootContainers.length === 1 && !hasTopLevelVisuals) {
    container = rootContainers[0];
  } else {
    container = {
      id: `${feedId || 'feed'}-root`,
      title: ui.title || payload?.title || '',
      dataSourceRef: defaultDataSourceRef,
      layout: ui.layout || { orientation: 'vertical', columns: 1 },
      items: rootItems,
      containers: rootContainers,
      toolbar: ui.toolbar,
      table: ui.table,
      chart: ui.chart,
      chat: ui.chat,
      terminal: ui.terminal,
      fileBrowser: ui.fileBrowser,
      treeBrowser: ui.treeBrowser,
      editor: ui.editor,
      schemaBasedForm: ui.schemaBasedForm,
      style: ui.style || {},
    };
  }

  if (container && !container.dataSourceRef && defaultDataSourceRef) {
    container.dataSourceRef = defaultDataSourceRef;
  }
  return applyAutoTableColumns(container, dataMap);
}

function ForgeFeedRenderer({ data, feedId = '', conversationId = '', variant = 'inline' }) {
  const normalized = normalizeFeedPayload(data);
  const dataSources = normalizeDataSources(normalized?.ui?.dataSources || normalized?.dataSources || {});
  const context = createFeedContext(feedId, dataSources, conversationId);
  const execution = {
    dataSources,
    dataFeed: {
      name: Object.keys(dataSources)[0] || feedId,
      data: normalized?.data,
    },
  };
  const dataMap = computeDataMap(execution);
  wireFeedSignals(execution, context.identity.windowId);
  const container = buildForgeFeedContainer(feedId, normalized, dataMap);
  if (!container) return null;
  const railStyle = variant === 'rail'
    ? { height: '100%', minHeight: 0, overflowY: 'auto' }
    : { maxHeight: 'min(18vh, 220px)', overflowY: 'auto' };
  return (
    <div className="app-tool-feed-detail-forge" style={railStyle}>
      <Container context={context} container={container} isActive suppressTitle={!container?.title} />
    </div>
  );
}

/**
 * Generic data-driven renderer — inspects the data shape and renders
 * accordingly. Used as fallback when no Forge UI spec is defined.
 */
function InlineRenderer({ data, variant = 'inline', onPathActivate = null }) {
  if (!data) return null;
  const railStyle = variant === 'rail'
    ? { height: '100%', minHeight: 0, overflowY: 'auto' }
    : { maxHeight: 'min(18vh, 140px)', overflowY: 'auto' };
  const terminalUI = data?.ui?.terminal && typeof data.ui.terminal === 'object'
    ? data.ui.terminal
    : null;
  const dataSources = (data?.dataSources && typeof data.dataSources === 'object')
    ? data.dataSources
    : ((data?.ui?.dataSources && typeof data.ui.dataSources === 'object') ? data.ui.dataSources : {});

  if (terminalUI && Object.keys(dataSources).length > 0) {
    const dsRef = String(terminalUI.dataSourceRef || '').trim();
    const dsConfig = dsRef ? (dataSources?.[dsRef] || {}) : {};
    const source = String(dsConfig?.source || '').trim();
    const entries = source ? asArray(selectPath(source, data?.data || {})) : [];
    return (
      <div className="app-tool-feed-detail-list" style={railStyle}>
        <Terminal
          entries={entries}
          height={terminalUI.height || '100%'}
          prompt={terminalUI.prompt || '$'}
          autoScroll={terminalUI.autoScroll !== false}
          showDividers={!!terminalUI.showDividers}
          truncateLongOutput={terminalUI.truncateLongOutput}
          truncateLength={terminalUI.truncateLength}
          className={terminalUI.className || ''}
          style={terminalUI.style || {}}
        />
      </div>
    );
  }

  return (
    <div className="app-tool-feed-detail-list" style={railStyle}>
      <CompactFeedList data={data} classNamePrefix="app-tool-feed-detail" onPathActivate={onPathActivate} />
    </div>
  );
}
