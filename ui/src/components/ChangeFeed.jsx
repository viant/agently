import React from 'react';
import { Button, Tag } from '@blueprintjs/core';
import { client } from '../services/agentlyClient';
import {
  CHANGE_FEED_ANCHORS,
  normalizeChangeFeedAnchor,
  setChangeFeedAnchor,
  useChangeFeed
} from '../services/changeFeedBus';
import {
  openCodeDiffDialog,
  openFileViewDialog,
  updateCodeDiffDialog,
  updateFileViewDialog
} from '../utils/dialogBus';

const ANCHOR_OPTIONS = [
  { value: CHANGE_FEED_ANCHORS.COMPOSER_TOP, label: 'Above composer' },
  { value: CHANGE_FEED_ANCHORS.SIDEBAR_TOP, label: 'Sidebar top' },
  { value: CHANGE_FEED_ANCHORS.SIDEBAR_BOTTOM, label: 'Sidebar bottom' }
];

function trimPath(value = '') {
  const text = String(value || '').trim();
  return text ? text.split('/').pop() : '';
}

function kindIntent(kind = '') {
  switch (String(kind || '').toLowerCase()) {
    case 'add':
    case 'added':
    case 'create':
    case 'created':
      return 'success';
    case 'delete':
    case 'deleted':
      return 'danger';
    default:
      return 'primary';
  }
}

function kindLabel(kind = '') {
  const text = String(kind || '').trim().toLowerCase();
  return text ? text.replace(/_/g, ' ') : 'modified';
}

async function fetchText(uri = '') {
  const value = String(uri || '').trim();
  if (!value) return '';
  try {
    return await client.downloadWorkspaceFile(value);
  } catch (_) {
    return '';
  }
}

async function openFile(change = {}) {
  const title = trimPath(change?.url || change?.origUrl) || 'File';
  const uri = String(change?.url || change?.origUrl || '').trim();
  openFileViewDialog({ title, uri, loading: true, content: '' });
  try {
    const content = await fetchText(uri);
    updateFileViewDialog({ content, loading: false });
  } catch (err) {
    updateFileViewDialog({ content: String(err?.message || err || 'failed to load file'), loading: false });
  }
}

async function openDiff(change = {}) {
  const title = trimPath(change?.url || change?.origUrl) || 'Changed File';
  const currentUri = String(change?.url || '').trim();
  const prevUri = String(change?.origUrl || '').trim();
  openCodeDiffDialog({
    title,
    loading: true,
    currentUri,
    prevUri,
    hasPrev: !!prevUri
  });
  try {
    const [current, prev] = await Promise.all([
      fetchText(currentUri),
      prevUri ? fetchText(prevUri) : Promise.resolve('')
    ]);
    updateCodeDiffDialog({
      current,
      prev,
      diff: String(change?.diff || ''),
      hasPrev: !!prev,
      loading: false
    });
  } catch (_) {
    updateCodeDiffDialog({
      current: '',
      prev: '',
      diff: String(change?.diff || ''),
      hasPrev: false,
      loading: false
    });
  }
}

export default function ChangeFeed({ anchor = CHANGE_FEED_ANCHORS.SIDEBAR_TOP, compact = false }) {
  const feed = useChangeFeed();
  const selectedAnchor = normalizeChangeFeedAnchor(feed?.anchor || CHANGE_FEED_ANCHORS.COMPOSER_TOP);
  const changes = Array.isArray(feed?.changes) ? feed.changes : [];
  const isComposerTop = anchor === CHANGE_FEED_ANCHORS.COMPOSER_TOP;
  const showAnchorControls = !compact && !isComposerTop;
  const showComposerDockAction = !compact && isComposerTop;
  if (selectedAnchor !== anchor) return null;
  if (changes.length === 0) return null;
  const visible = isComposerTop ? changes.slice(0, 4) : changes;

  return (
    <section
      className={`app-change-feed app-change-feed-${anchor}${compact ? ' is-compact' : ''}`}
      data-testid={`change-feed-${anchor}`}
    >
      <div className="app-change-feed-head">
        <div className="app-change-feed-title-wrap">
          <div className="app-change-feed-title">{changes.length} {changes.length === 1 ? 'file changed' : 'files changed'}</div>
          {feed?.workdir ? <div className="app-change-feed-subtitle">{feed.workdir}</div> : null}
        </div>
        <div className="app-change-feed-actions">
          {showAnchorControls ? (
            ANCHOR_OPTIONS.map((option) => (
              <Button
                key={option.value}
                minimal
                small
                className={`app-change-feed-anchor-btn${selectedAnchor === option.value ? ' is-active' : ''}`}
                onClick={() => setChangeFeedAnchor(option.value)}
              >
                {option.label}
              </Button>
            ))
          ) : null}
          {showComposerDockAction ? (
            <button
              type="button"
              className="app-change-feed-dock-action"
              onClick={() => setChangeFeedAnchor(CHANGE_FEED_ANCHORS.SIDEBAR_TOP)}
              aria-label="Move change feed to sidebar"
              title="Move change feed to sidebar"
            >
              ↗
            </button>
          ) : null}
        </div>
      </div>
      <div className="app-change-feed-list">
        {visible.map((change, index) => (
          <article className="app-change-feed-item" key={change?.id || `${change?.url || change?.origUrl || 'change'}:${index}`}>
            <div className="app-change-feed-item-main">
              <div className="app-change-feed-item-name" title={change?.url || change?.origUrl || ''}>{change?.name || trimPath(change?.url || change?.origUrl)}</div>
              <div className="app-change-feed-item-path">{change?.url || change?.origUrl || ''}</div>
            </div>
            <Tag minimal intent={kindIntent(change?.kind)}>{kindLabel(change?.kind)}</Tag>
            <div className="app-change-feed-item-actions">
              <Button small minimal onClick={() => openFile(change)}>Open</Button>
              <Button small minimal intent="primary" onClick={() => openDiff(change)}>Diff</Button>
            </div>
          </article>
        ))}
      </div>
      {isComposerTop && changes.length > visible.length ? (
        <div className="app-change-feed-footer">
          <span>{changes.length - visible.length} more hidden</span>
          <Button small minimal onClick={() => setChangeFeedAnchor(CHANGE_FEED_ANCHORS.SIDEBAR_TOP)}>Browse all</Button>
        </div>
      ) : null}
    </section>
  );
}
