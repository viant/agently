import React, { useEffect, useMemo, useState } from 'react';
import { Button, InputGroup, Spinner } from '@blueprintjs/core';
import ChangeFeed from './ChangeFeed';
import { resolveConversationSummary, resolveConversationTitle } from '../services/conversationTitle';
import { isConnectivityError } from '../services/networkError';
import { client } from '../services/agentlyClient';
import {
  getWindowById,
  MAIN_CHAT_WINDOW_ID,
  isLinkedChildWindow,
  openConversationInMainWindow,
  requestNewConversationInMainWindow
} from '../services/conversationWindow';

const PAGE_SIZE = 12;

function asEpoch(value) {
  if (!value) return 0;
  const epoch = Date.parse(String(value));
  return Number.isFinite(epoch) ? epoch : 0;
}

function conversationSortKey(row = {}) {
  return Math.max(
    asEpoch(row?.LastActivity),
    asEpoch(row?.lastActivity),
    asEpoch(row?.UpdatedAt),
    asEpoch(row?.updatedAt),
    asEpoch(row?.CreatedAt),
    asEpoch(row?.createdAt)
  );
}

function conversationTimestamp(row = {}) {
  return Math.max(
    asEpoch(row?.LastActivity),
    asEpoch(row?.lastActivity),
    asEpoch(row?.UpdatedAt),
    asEpoch(row?.updatedAt),
    asEpoch(row?.CreatedAt),
    asEpoch(row?.createdAt)
  );
}

function formatRelativeTime(epochMillis = 0) {
  const t = Number(epochMillis || 0);
  if (!t) return '';
  const diff = Math.max(0, Date.now() - t);
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (diff < hour) return `${Math.max(1, Math.floor(diff / minute))}m`;
  if (diff < day) return `${Math.floor(diff / hour)}h`;
  const days = Math.floor(diff / day);
  if (days < 7) return `${days}d`;
  return `${Math.floor(days / 7)}w`;
}

function sortConversations(rows = []) {
  return rows
    .map((row, index) => ({ row, index }))
    .sort((left, right) => {
      const a = left.row || {};
      const b = right.row || {};
      const timeDelta = conversationSortKey(b) - conversationSortKey(a);
      if (timeDelta !== 0) return timeDelta;

      const aID = String(a?.Id || a?.id || '');
      const bID = String(b?.Id || b?.id || '');
      const idDelta = aID.localeCompare(bID);
      if (idDelta !== 0) return idDelta;

      return left.index - right.index;
    })
    .map((entry) => entry.row);
}

export function applyConversationMetaPatchToRows(rows = [], conversationID = '', patch = {}) {
  const id = String(conversationID || '').trim();
  if (!id || !Array.isArray(rows) || rows.length === 0 || !patch || typeof patch !== 'object') {
    return rows;
  }
  let changed = false;
  const next = rows.map((row) => {
    const rowID = String(row?.Id || row?.id || '').trim();
    if (rowID !== id) return row;
    changed = true;
    const updated = { ...(row || {}) };
    if (Object.prototype.hasOwnProperty.call(patch, 'title')) {
      updated.Title = patch.title;
      updated.title = patch.title;
    }
    if (Object.prototype.hasOwnProperty.call(patch, 'summary')) {
      updated.Summary = patch.summary;
      updated.summary = patch.summary;
    }
    if (Object.prototype.hasOwnProperty.call(patch, 'agentId')) {
      updated.AgentId = patch.agentId;
      updated.agentId = patch.agentId;
    }
    if (Object.prototype.hasOwnProperty.call(patch, 'status')) {
      updated.Status = patch.status;
      updated.status = patch.status;
    }
    updated.UpdatedAt = new Date().toISOString();
    updated.updatedAt = updated.UpdatedAt;
    return updated;
  });
  return changed ? sortConversations(next) : rows;
}

function conversationStatusTone(row = {}) {
  const status = String(row?.Status || row?.status || '').trim().toLowerCase();
  const stage = String(row?.Stage || row?.stage || '').trim().toLowerCase();
  if (status === 'completed' || status === 'succeeded' || status === 'success') return 'success';
  if (status === 'failed' || status === 'error' || status === 'cancelled' || status === 'canceled') return 'error';
  if (stage === 'done') return 'success';
  if (stage === 'error' || stage === 'canceled') return 'error';
  if (!status || status === 'running' || status === 'in_progress' || status === 'processing' || status === 'queued') return 'running';
  return 'idle';
}

async function archiveConversation(id) {
  if (!id) return;
  try {
    await client.updateConversation(id, { visibility: 'archived' });
  } catch (_) {}
}

async function fetchPage({ query = '', direction = 'latest', cursor = '' }) {
  const page = await client.listConversations({
    excludeScheduled: true,
    query: query || undefined,
    page: { limit: PAGE_SIZE, direction, cursor: cursor || undefined },
  });
  const rows = sortConversations(Array.isArray(page?.data) ? page.data : []);
  const prev = String(page?.page?.prevCursor || '').trim();
  const next = String(page?.page?.cursor || '').trim();
  return {
    rows,
    prevCursor: prev,
    nextCursor: next,
    hasNewer: Boolean(page?.page?.hasNewer),
    hasOlder: Boolean(page?.page?.hasOlder),
  };
}

export default function Sidebar({ collapsed = false }) {
  const [query, setQuery] = useState('');
  const [rows, setRows] = useState([]);
  const [seedVersion, setSeedVersion] = useState(0);
  const [selectedID, setSelectedID] = useState(() => {
    if (typeof window === 'undefined') return '';
    return String(window.localStorage?.getItem('agently.selectedConversationId') || '').trim();
  });
  const [prevCursor, setPrevCursor] = useState('');
  const [nextCursor, setNextCursor] = useState('');
  const [hasNewer, setHasNewer] = useState(false);
  const [hasOlder, setHasOlder] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const rowsRef = React.useRef([]);
  const activityReloadTimerRef = React.useRef(null);

  const pageStatusLabel = useMemo(() => {
    if (loading) return 'Loading...';
    if (hasNewer && hasOlder) return 'Middle';
    if (hasNewer && !hasOlder) return 'Oldest';
    if (!hasNewer && hasOlder) return 'Newest';
    return 'Single page';
  }, [loading, hasNewer, hasOlder]);
  const showPagination = loading || hasNewer || hasOlder;

  const reload = async (direction = 'latest', cursor = '') => {
    setLoading(true);
    try {
      const page = await fetchPage({ query: query.trim(), direction, cursor });
      setRows(Array.isArray(page.rows) ? page.rows : []);
      setPrevCursor(page.prevCursor);
      setNextCursor(page.nextCursor);
      setHasNewer(page.hasNewer);
      setHasOlder(page.hasOlder);
      setError('');
    } catch (err) {
      if (isConnectivityError(err)) {
        setError('');
      } else {
        setError(String(err?.message || err));
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    rowsRef.current = rows;
  }, [rows]);

  useEffect(() => {
    const timer = setTimeout(() => {
      void reload('latest', '');
    }, 200);
    return () => clearTimeout(timer);
  }, [query]); // eslint-disable-line react-hooks/exhaustive-deps

  const content = useMemo(() => {
    if (loading) return <div className="app-sidebar-loading"><Spinner size={18} /></div>;
    if (error) return <div className="app-sidebar-error">{error}</div>;
    if (rows.length === 0) return <div className="app-sidebar-empty">No conversations</div>;

    return (
      <div className="app-conversation-list">
        {rows.map((row) => {
          const id = String(row?.Id || row?.id || '').trim();
          const title = resolveConversationTitle(row);
          const summary = resolveConversationSummary(row);
          const subtitle = String(row?.Agent || row?.agent || '').trim();
          const relative = formatRelativeTime(conversationTimestamp(row));
          const isSelected = id && id === selectedID;
          const tone = conversationStatusTone(row);
          const hoverText = summary || title;
          return (
            <div
              key={id}
              className={`app-conversation-row ${isSelected ? 'is-selected' : ''}`}
            >
              <span className={`app-conversation-status-dot tone-${tone}`} title={tone} />
              <button
                className="app-conversation-row-body"
                title={hoverText}
                aria-label={hoverText}
                onClick={() => {
                  setSelectedID(id);
                  openConversationInMainWindow(id);
                }}
              >
                <div className="app-conversation-topline">
                  <div className="app-conversation-title" title={title}>{title}</div>
                  {relative ? <div className="app-conversation-meta">{relative}</div> : null}
                </div>
                {subtitle ? <div className="app-conversation-subtitle" title={subtitle}>{subtitle}</div> : null}
              </button>
              <button
                className="app-conversation-trash"
                title="Archive conversation"
                onClick={(e) => {
                  e.stopPropagation();
                  archiveConversation(id).then(() => {
                    setRows((prev) => prev.filter((r) => String(r?.Id || r?.id || '').trim() !== id));
                  });
                }}
              >
                🗑
              </button>
            </div>
          );
        })}
      </div>
    );
  }, [rows, loading, error, selectedID, seedVersion]);

  useEffect(() => {
    if (typeof window === 'undefined') return () => {};
    const onActive = (event) => {
      const windowId = String(event?.detail?.windowId || '').trim();
      if (windowId && isLinkedChildWindow(getWindowById(windowId))) return;
      const id = String(event?.detail?.id || '').trim();
      setSelectedID(id);
      const hasRows = rowsRef.current.length > 0;
      const hasSelectedRow = id && rowsRef.current.some((row) => String(row?.Id || row?.id || '').trim() === id);
      if (!hasRows || !hasSelectedRow) {
        void reload('latest', '');
      }
    };
    const onConversationNew = () => {
      // Small delay to allow the backend to commit the new conversation
      // before the sidebar queries the list.
      setTimeout(() => void reload('latest', ''), 300);
    };
    const onConversationActivity = () => {
      if (activityReloadTimerRef.current) {
        clearTimeout(activityReloadTimerRef.current);
      }
      activityReloadTimerRef.current = setTimeout(() => {
        activityReloadTimerRef.current = null;
        void reload('latest', '');
      }, 150);
    };
    const onConversationMetaUpdated = (event) => {
      const id = String(event?.detail?.id || '').trim();
      const patch = event?.detail?.patch || {};
      if (!id || !patch || typeof patch !== 'object') return;
      setRows((current) => applyConversationMetaPatchToRows(current, id, patch));
      if (Object.prototype.hasOwnProperty.call(patch, 'title')) {
        setSeedVersion((value) => value + 1);
      }
    };
    window.addEventListener('forge:conversation-active', onActive);
    window.addEventListener('agently:conversation-new', onConversationNew);
    window.addEventListener('agently:conversation-activity', onConversationActivity);
    window.addEventListener('agently:conversation-meta-updated', onConversationMetaUpdated);
    return () => {
      if (activityReloadTimerRef.current) {
        clearTimeout(activityReloadTimerRef.current);
        activityReloadTimerRef.current = null;
      }
      window.removeEventListener('forge:conversation-active', onActive);
      window.removeEventListener('agently:conversation-new', onConversationNew);
      window.removeEventListener('agently:conversation-activity', onConversationActivity);
      window.removeEventListener('agently:conversation-meta-updated', onConversationMetaUpdated);
    };
  }, []);

  useEffect(() => {
    if (typeof window === 'undefined') return () => {};
    const onSeed = () => setSeedVersion((value) => value + 1);
    window.addEventListener('agently:conversation-title-seed', onSeed);
    return () => window.removeEventListener('agently:conversation-title-seed', onSeed);
  }, []);

  return (
    <aside className={`app-sidebar ${collapsed ? 'is-collapsed' : ''}`}>
      <Button minimal icon="plus" alignText="left" onClick={() => requestNewConversationInMainWindow()}>
        {collapsed ? '' : 'New Conversation'}
      </Button>

      {!collapsed ? (
        <InputGroup
          leftIcon="search"
          placeholder="Filter conversations"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          small
        />
      ) : null}

      {!collapsed ? (
        <>
          <ChangeFeed anchor="sidebar_top" compact />
          <div className="app-sidebar-section" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
            <span>Conversations</span>
            <Button minimal small icon="refresh" onClick={() => void reload('latest', '')}>
              Refresh
            </Button>
          </div>
          <div className="app-sidebar-scroll">{content}</div>
          <ChangeFeed anchor="sidebar_bottom" compact />
          {showPagination ? (
            <div className="app-sidebar-pagination">
              <Button
                small
                minimal
                icon="chevron-left"
                className="app-sidebar-pagination-btn"
                disabled={!hasNewer || !prevCursor}
                onClick={() => void reload('after', prevCursor)}
              >
                Newer
              </Button>
              <div className="app-sidebar-pagination-status">{pageStatusLabel}</div>
              <Button
                small
                minimal
                icon="chevron-right"
                className="app-sidebar-pagination-btn"
                disabled={!hasOlder || !nextCursor}
                onClick={() => void reload('before', nextCursor)}
              >
                Older
              </Button>
            </div>
          ) : null}
        </>
      ) : (
        <div className="app-sidebar-collapsed-tools">
          <Button minimal small icon="search" />
          <Button minimal small icon="history" onClick={() => void reload('latest', '')} />
        </div>
      )}
    </aside>
  );
}
