import React, { useEffect, useMemo, useState } from 'react';
import { Drawer, Icon } from '@blueprintjs/core';

import { getActiveFeeds, onFeedChange } from '../services/toolFeedBus';
import { activateExclusiveFeed } from '../services/toolFeedSelection';
import { toolFeedTargetsPlacement } from '../services/toolFeedTarget';
import { feedAccent, feedIconName } from './ToolFeedBar';
import ToolFeedDetail from './ToolFeedDetail';

export function filterDetachedFeeds(feeds = [], conversationId = '', developerMode = false) {
  const scope = String(conversationId || '').trim();
  return (Array.isArray(feeds) ? feeds : []).filter((feed) => {
    if (feed?.developerOnly === true && !developerMode) return false;
    const feedConversationId = String(feed?.conversationId || '').trim();
    if (scope && feedConversationId && feedConversationId !== scope) return false;
    return toolFeedTargetsPlacement(feed, 'detached', false);
  });
}

export default function ToolFeedDetached({ conversationId = '', developerMode = false, initialOpen = false }) {
  const [feeds, setFeeds] = useState(getActiveFeeds);
  const [open, setOpen] = useState(initialOpen === true);
  const [selectedFeedId, setSelectedFeedId] = useState('');

  useEffect(() => onFeedChange((next) => setFeeds(next)), []);
  const visibleFeeds = useMemo(
    () => filterDetachedFeeds(feeds, conversationId, developerMode),
    [conversationId, developerMode, feeds],
  );
  const signature = visibleFeeds.map((feed) => String(feed?.feedId || '').trim()).filter(Boolean).join('|');

  useEffect(() => {
    if (visibleFeeds.length === 0) {
      setOpen(false);
      setSelectedFeedId('');
      return;
    }
    const selected = visibleFeeds.find((feed) => feed.feedId === selectedFeedId) || visibleFeeds[0];
    if (selected?.feedId !== selectedFeedId) setSelectedFeedId(selected?.feedId || '');
    if (selected?.feedId) activateExclusiveFeed(selected.feedId, selected.conversationId);
    setOpen(true);
  }, [signature]);

  if (visibleFeeds.length === 0) return null;
  const selected = visibleFeeds.find((feed) => feed.feedId === selectedFeedId) || visibleFeeds[0];
  const accent = feedAccent(selected?.presentation);

  return (
    <>
      {!open ? (
        <button
          type="button"
          className="app-tool-feed-detached-launcher"
          onClick={() => setOpen(true)}
          style={{ '--feed-accent': accent }}
          aria-label={`Open ${selected?.title || 'detached tool feed'}`}
        >
          <Icon icon={feedIconName(selected?.presentation)} size={15} />
          <span>{selected?.title || 'Tool feed'}</span>
        </button>
      ) : null}
      <Drawer
        isOpen={open}
        onClose={() => setOpen(false)}
        position="right"
        size="min(760px, 100vw)"
        className="app-tool-feed-detached-drawer"
        portalClassName="app-tool-feed-detached-portal"
      >
        <section className="app-tool-feed-detached" aria-label="Detached tool feed">
          <header className="app-tool-feed-detached-header" style={{ '--feed-accent': accent }}>
            <div className="app-tool-feed-detached-title">
              <Icon icon={feedIconName(selected?.presentation)} size={16} />
              <span>{selected?.title || 'Tool feed'}</span>
            </div>
            <button type="button" onClick={() => setOpen(false)} aria-label="Close detached tool feed">Close</button>
          </header>
          {visibleFeeds.length > 1 ? (
            <div className="app-tool-feed-detached-tabs" role="tablist" aria-label="Detached tool feeds">
              {visibleFeeds.map((feed) => (
                <button
                  key={feed.feedId}
                  type="button"
                  role="tab"
                  aria-selected={feed.feedId === selected?.feedId}
                  onClick={() => {
                    setSelectedFeedId(feed.feedId);
                    activateExclusiveFeed(feed.feedId, feed.conversationId);
                  }}
                >
                  {feed.title || feed.feedId}
                </button>
              ))}
            </div>
          ) : null}
          <div className="app-tool-feed-detached-body">
            <ToolFeedDetail variant="rail" placement="detached" includeAuto={false} conversationId={conversationId} />
          </div>
        </section>
      </Drawer>
    </>
  );
}
