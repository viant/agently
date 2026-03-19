import React, { useEffect, useMemo, useState } from 'react';
import { Tabs, Tab } from '@blueprintjs/core';
import { getFeedData, onFeedDataChange, getActiveFeeds, onFeedChange } from '../services/toolFeedBus';
import { isFeedExpanded } from './ToolFeedBar';
import { normalizeDataSources, computeDataMap, applyAutoTableColumns, wireFeedSignals } from '../services/feedForgeWiring';
import { createFeedContext } from '../services/feedForgeContext';

// Try to import Forge Container. Falls back to null if unavailable.
let ForgeContainer = null;
try {
  ForgeContainer = require('forge/components/Container.jsx').default
    || require('forge/components/Container.jsx').Container;
} catch (_) {
  try {
    const mod = require('forge/components/Container');
    ForgeContainer = mod.default || mod.Container || null;
  } catch (__) {}
}

/**
 * Tabbed feed detail panel rendered below execution details in IterationBlock.
 * Uses Forge Container to render feed UI specs from YAML.
 * Single feed = no tab bar, just content.
 */
export default function ToolFeedDetail() {
  const [feeds, setFeeds] = useState(getActiveFeeds);
  const [, tick] = useState(0);

  useEffect(() => {
    const u1 = onFeedChange((next) => setFeeds(next));
    const u2 = onFeedDataChange(() => tick((n) => n + 1));
    return () => { u1(); u2(); };
  }, []);

  // Collect expanded feeds that have data.
  const visibleFeeds = useMemo(() => {
    return (feeds || []).filter((f) => isFeedExpanded(f.feedId) && getFeedData(f.feedId));
  }, [feeds, tick]);

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

function FeedPanel({ feedId }) {
  const [ready, setReady] = useState(false);
  const data = getFeedData(feedId);

  const { forgeContext, uiSpec } = useMemo(() => {
    if (!data) return { forgeContext: null, uiSpec: null };

    const rawDS = data.dataSources || data.ui?.dataSources || {};
    const normalized = normalizeDataSources(rawDS);
    const windowId = `feed-${feedId}`;

    // Build the execution-like object for wiring.
    const exe = {
      dataSources: normalized,
      dataFeed: { name: feedId, data: data.data },
    };
    const dataMap = computeDataMap(exe);

    // Deep clone UI spec and apply auto columns.
    let ui = null;
    if (data.ui && typeof data.ui === 'object') {
      ui = JSON.parse(JSON.stringify(data.ui));
      ui.dataSources = normalized;
      applyAutoTableColumns(ui, dataMap);
    }

    // Wire data into Forge signals.
    wireFeedSignals(exe, windowId);

    // Build minimal Forge context.
    const ctx = createFeedContext(feedId, normalized);
    return { forgeContext: ctx, uiSpec: ui };
  }, [feedId, data]);

  useEffect(() => {
    // Defer rendering until signals are wired.
    if (forgeContext) setReady(true);
    else setReady(false);
  }, [forgeContext]);

  if (!data) return null;

  if (!ForgeContainer || !uiSpec || !ready) return null;

  return (
    <div style={{ minHeight: 20 }}>
      <ForgeContainer container={uiSpec} context={forgeContext} />
    </div>
  );
}

