import React, { useEffect, useMemo, useState } from 'react';
import { Button, InputGroup, Spinner } from '@blueprintjs/core';
import { resolveConversationSummary, resolveConversationTitle } from '../services/conversationTitle';
import { isConnectivityError } from '../services/networkError';
import { client } from '../services/agentlyClient';
import { openConfirmDialog } from '../utils/dialogBus';
import {
  getWindowById,
  getScopedConversationSelection,
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
    if (Object.prototype.hasOwnProperty.call(patch, 'stage')) {
      updated.Stage = patch.stage;
      updated.stage = patch.stage;
    }
    if (Object.prototype.hasOwnProperty.call(patch, 'running')) {
      updated.Running = patch.running;
      updated.running = !!patch.running;
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

export function removeConversationRow(rows = [], conversationID = '') {
  const id = String(conversationID || '').trim();
  if (!id || !Array.isArray(rows)) return rows;
  return rows.filter((row) => String(row?.Id || row?.id || '').trim() !== id);
}

function conversationID(row = {}) {
  return String(row?.Id || row?.id || '').trim();
}

export function fillDeletedSidebarPageFromOlder({
  rows = [],
  olderPage = null,
  hadNewer = false,
  hadOlder = false,
  pageSize = PAGE_SIZE
} = {}) {
  const baseRows = sortConversations(Array.isArray(rows) ? rows : []);
  const limit = Math.max(1, Number(pageSize || PAGE_SIZE));
  const need = Math.max(0, limit - baseRows.length);
  const seen = new Set(baseRows.map(conversationID).filter(Boolean));
  const olderRows = sortConversations(Array.isArray(olderPage?.rows) ? olderPage.rows : []);
  const candidates = [];

  for (const row of olderRows) {
    const id = conversationID(row);
    if (!id || seen.has(id)) continue;
    seen.add(id);
    candidates.push(row);
  }

  const rowsToAppend = candidates.slice(0, need);
  const mergedRows = sortConversations([...baseRows, ...rowsToAppend]).slice(0, limit);
  const firstID = conversationID(mergedRows[0]);
  const lastID = conversationID(mergedRows[mergedRows.length - 1]);
  const hasMoreOlder = Boolean(hadOlder && lastID && (candidates.length > need || olderPage?.nextCursor));

  return {
    rows: mergedRows,
    prevCursor: hadNewer && firstID ? firstID : '',
    nextCursor: hasMoreOlder ? lastID : ''
  };
}

export function conversationDeleteErrorMessage(err) {
  const status = Number(err?.status || err?.statusCode || 0);
  const message = String(err?.body || err?.message || err || '').trim();
  if (status === 409 || /still in progress|in progress|active/i.test(message)) {
    return 'Conversation is still in progress and cannot be deleted yet.';
  }
  if (status === 403 || /permission denied|forbidden/i.test(message)) {
    return 'Only the conversation owner can delete this conversation.';
  }
  if (status === 404 || /not found/i.test(message)) {
    return 'Conversation was already deleted or is no longer available.';
  }
  return message || 'Failed to delete conversation.';
}

export function normalizeSidebarPage(page = {}, direction = 'latest', requestedCursor = '') {
  const rows = sortConversations(Array.isArray(page?.data) ? page.data : []);
  const pageInfo = page?.page || {};
  const hasRequestedCursor = String(requestedCursor || '').trim() !== '';
  const hasMore = Boolean(pageInfo?.hasMore);
  const hasOlder = typeof pageInfo?.hasOlder === 'boolean'
    ? pageInfo.hasOlder
    : (direction === 'after' ? hasRequestedCursor : hasMore);
  const hasNewer = typeof pageInfo?.hasNewer === 'boolean'
    ? pageInfo.hasNewer
    : (direction === 'before' ? hasRequestedCursor : (direction === 'after' ? hasMore : false));
  const prev = String(pageInfo?.prevCursor || '').trim();
  const next = String(pageInfo?.cursor || '').trim();
  return {
    rows,
    prevCursor: hasNewer && prev ? prev : '',
    nextCursor: hasOlder && next ? next : ''
  };
}

export function sidebarPageStatusLabel({ loading = false, prevCursor = '', nextCursor = '' } = {}) {
  if (loading) return 'Loading...';
  if (prevCursor && nextCursor) return 'Middle';
  if (!prevCursor && nextCursor) return 'Newest';
  if (prevCursor && !nextCursor) return 'Oldest';
  return 'Single';
}

export function sidebarPaginationRequest(kind = '', cursor = '') {
  if (kind === 'newer') return normalizeSidebarPageRequest('after', cursor);
  if (kind === 'older') return normalizeSidebarPageRequest('before', cursor);
  return normalizeSidebarPageRequest('latest', '');
}

export function normalizeSidebarPageRequest(direction = 'latest', cursor = '') {
  const nextDirection = String(direction || 'latest').trim();
  const nextCursor = String(cursor || '').trim();
  if ((nextDirection === 'before' || nextDirection === 'after') && nextCursor) {
    return { direction: nextDirection, cursor: nextCursor };
  }
  return { direction: 'latest', cursor: '' };
}

async function fetchPage({ query = '', direction = 'latest', cursor = '' }) {
  const page = await client.listConversations({
    excludeScheduled: true,
    query: query || undefined,
    page: { limit: PAGE_SIZE, direction, cursor: cursor || undefined },
  });
  return normalizeSidebarPage(page, direction, cursor);
}

export default function Sidebar({ collapsed = false, onNavigate = null }) {
  const [query, setQuery] = useState('');
  const [rows, setRows] = useState([]);
  const [seedVersion, setSeedVersion] = useState(0);
  const [selectedID, setSelectedID] = useState(() => {
    if (typeof window === 'undefined') return '';
    return getScopedConversationSelection(MAIN_CHAT_WINDOW_ID);
  });
  const [prevCursor, setPrevCursor] = useState('');
  const [nextCursor, setNextCursor] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [deleteError, setDeleteError] = useState('');
  const [deletingID, setDeletingID] = useState('');
  const rowsRef = React.useRef([]);
  const queryRef = React.useRef('');
  const prevCursorRef = React.useRef('');
  const nextCursorRef = React.useRef('');
  const activityReloadTimerRef = React.useRef(null);
  const queryReloadTimerRef = React.useRef(null);
  const inFlightReloadRef = React.useRef({ key: '', promise: null });
  const reloadSeqRef = React.useRef(0);
  const lastResolvedReloadKeyRef = React.useRef('');

  const pageStatusLabel = useMemo(() => {
    return sidebarPageStatusLabel({ loading, prevCursor, nextCursor });
  }, [loading, prevCursor, nextCursor]);
  const showPagination = loading || !!prevCursor || !!nextCursor;
  const navigate = React.useCallback(() => {
    if (typeof onNavigate === 'function') {
      onNavigate();
    }
  }, [onNavigate]);

  const requestDeleteConversation = (row) => {
    const id = String(row?.Id || row?.id || '').trim();
    if (!id || deletingID) return;
    const title = resolveConversationTitle(row);
    openConfirmDialog({
      title: 'Delete conversation',
      message: `Delete "${title}" and its conversation tree? Conversations still in progress cannot be deleted.`,
      confirmText: 'Delete',
      intent: 'danger',
      onConfirm: async () => {
        setDeletingID(id);
        try {
          await client.deleteConversation(id);
          const beforeRows = rowsRef.current;
          const remainingRows = removeConversationRow(beforeRows, id);
          const hadNewer = !!prevCursorRef.current;
          const hadOlder = !!nextCursorRef.current;
          setRows(remainingRows);
          setError('');
          setDeleteError('');
          if (selectedID === id) {
            setSelectedID('');
            requestNewConversationInMainWindow();
          }
          try {
            if (remainingRows.length === 0) {
              await reload('latest', '', { force: true });
            } else if (remainingRows.length < PAGE_SIZE && hadOlder) {
              const oldestRemainingID = conversationID(sortConversations(remainingRows)[remainingRows.length - 1]);
              const olderPage = oldestRemainingID
                ? await fetchPage({ query: query.trim(), direction: 'before', cursor: oldestRemainingID })
                : null;
              const filled = fillDeletedSidebarPageFromOlder({
                rows: remainingRows,
                olderPage,
                hadNewer,
                hadOlder,
                pageSize: PAGE_SIZE
              });
              setRows(filled.rows);
              setPrevCursor(filled.prevCursor);
              setNextCursor(filled.nextCursor);
            } else if (remainingRows.length < PAGE_SIZE) {
              const filled = fillDeletedSidebarPageFromOlder({
                rows: remainingRows,
                hadNewer,
                hadOlder: false,
                pageSize: PAGE_SIZE
              });
              setRows(filled.rows);
              setPrevCursor(filled.prevCursor);
              setNextCursor(filled.nextCursor);
            }
          } catch (_) {
            // The delete itself succeeded; keep the locally removed row state if refill fails.
          }
        } catch (err) {
          setDeleteError(conversationDeleteErrorMessage(err));
          throw err;
        } finally {
          setDeletingID('');
        }
      }
    });
  };

  const reload = async (direction = 'latest', cursor = '', options = {}) => {
    const activeQuery = String(queryRef.current || '').trim();
    const pageRequest = normalizeSidebarPageRequest(direction, cursor);
    const requestKey = JSON.stringify({
      query: activeQuery,
      direction: pageRequest.direction,
      cursor: pageRequest.cursor,
    });
    if (!options?.force && inFlightReloadRef.current.promise && inFlightReloadRef.current.key === requestKey) {
      return inFlightReloadRef.current.promise;
    }
    const requestSeq = reloadSeqRef.current + 1;
    reloadSeqRef.current = requestSeq;
    setLoading(true);
    const request = (async () => {
      try {
        const page = await fetchPage({ query: activeQuery, direction: pageRequest.direction, cursor: pageRequest.cursor });
        if (reloadSeqRef.current !== requestSeq) return page;
        lastResolvedReloadKeyRef.current = requestKey;
        setRows(Array.isArray(page.rows) ? page.rows : []);
        setPrevCursor(page.prevCursor);
        setNextCursor(page.nextCursor);
        setError('');
        setDeleteError('');
        return page;
      } catch (err) {
        if (reloadSeqRef.current === requestSeq) {
          if (isConnectivityError(err)) {
            setError('');
          } else {
            setError(String(err?.message || err));
          }
        }
        throw err;
      } finally {
        if (inFlightReloadRef.current.key === requestKey) {
          inFlightReloadRef.current = { key: '', promise: null };
        }
        if (reloadSeqRef.current === requestSeq) {
          setLoading(false);
        }
      }
    })();
    inFlightReloadRef.current = { key: requestKey, promise: request };
    return request;
  };

  useEffect(() => {
    rowsRef.current = rows;
  }, [rows]);

  useEffect(() => {
    queryRef.current = query;
  }, [query]);

  useEffect(() => {
    prevCursorRef.current = prevCursor;
    nextCursorRef.current = nextCursor;
  }, [prevCursor, nextCursor]);

  useEffect(() => {
    if (queryReloadTimerRef.current) {
      clearTimeout(queryReloadTimerRef.current);
      queryReloadTimerRef.current = null;
    }
    queryReloadTimerRef.current = setTimeout(() => {
      void reload('latest', '');
    }, 200);
    return () => {
      if (queryReloadTimerRef.current) {
        clearTimeout(queryReloadTimerRef.current);
        queryReloadTimerRef.current = null;
      }
    };
  }, [query]); // eslint-disable-line react-hooks/exhaustive-deps

  const content = useMemo(() => {
    if (loading) return <div className="app-sidebar-loading"><Spinner size={18} /></div>;
    if (error) return <div className="app-sidebar-error">{error}</div>;
    if (rows.length === 0) return <div className="app-sidebar-empty">No conversations</div>;

    return (
      <>
        {deleteError ? <div className="app-sidebar-error">{deleteError}</div> : null}
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
                    navigate();
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
                  title="Delete conversation"
                  aria-label="Delete conversation"
                  disabled={deletingID === id}
                  onClick={(e) => {
                    e.stopPropagation();
                    requestDeleteConversation(row);
                  }}
                >
                  🗑
                </button>
              </div>
            );
          })}
        </div>
      </>
    );
  }, [rows, loading, error, deleteError, selectedID, seedVersion, deletingID, navigate]);

  useEffect(() => {
    if (typeof window === 'undefined') return () => {};
    const onActive = (event) => {
      const windowId = String(event?.detail?.windowId || '').trim();
      if (windowId && isLinkedChildWindow(getWindowById(windowId))) return;
      const id = String(event?.detail?.id || '').trim();
      setSelectedID(id);
      if (queryReloadTimerRef.current) {
        clearTimeout(queryReloadTimerRef.current);
        queryReloadTimerRef.current = null;
      }
      const latestRequestKey = JSON.stringify({
        query: String(queryRef.current || '').trim(),
        direction: 'latest',
        cursor: '',
      });
      if (lastResolvedReloadKeyRef.current === latestRequestKey) {
        return;
      }
      if (inFlightReloadRef.current.key === latestRequestKey) {
        return;
      }
      const hasRows = rowsRef.current.length > 0;
      if (!hasRows) {
        void reload('latest', '');
      }
    };
    const onConversationNew = () => {
      setSelectedID('');
      if (queryReloadTimerRef.current) {
        clearTimeout(queryReloadTimerRef.current);
        queryReloadTimerRef.current = null;
      }
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
    window.addEventListener('agently:conversation-select', onActive);
    window.addEventListener('agently:conversation-new', onConversationNew);
    window.addEventListener('agently:conversation-activity', onConversationActivity);
    window.addEventListener('agently:conversation-meta-updated', onConversationMetaUpdated);
    return () => {
      if (queryReloadTimerRef.current) {
        clearTimeout(queryReloadTimerRef.current);
        queryReloadTimerRef.current = null;
      }
      if (activityReloadTimerRef.current) {
        clearTimeout(activityReloadTimerRef.current);
        activityReloadTimerRef.current = null;
      }
      window.removeEventListener('forge:conversation-active', onActive);
      window.removeEventListener('agently:conversation-select', onActive);
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
      <Button minimal icon="plus" alignText="left" onClick={() => {
        requestNewConversationInMainWindow();
        navigate();
      }}>
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
          <div className="app-sidebar-section" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
            <span>Conversations</span>
            <Button minimal small icon="refresh" onClick={() => void reload('latest', '')}>
              Refresh
            </Button>
          </div>
          <div className="app-sidebar-scroll">{content}</div>
          {showPagination ? (
            <div className="app-sidebar-pagination">
              <Button
                small
                minimal
                icon="chevron-left"
                className="app-sidebar-pagination-btn"
                disabled={!prevCursor}
                aria-label="Load newer conversations"
                title="Load newer conversations"
                onClick={() => {
                  const request = sidebarPaginationRequest('newer', prevCursor);
                  void reload(request.direction, request.cursor);
                }}
              />
              <div className="app-sidebar-pagination-status">{pageStatusLabel}</div>
              <Button
                small
                minimal
                icon="chevron-right"
                className="app-sidebar-pagination-btn"
                disabled={!nextCursor}
                aria-label="Load older conversations"
                title="Load older conversations"
                onClick={() => {
                  const request = sidebarPaginationRequest('older', nextCursor);
                  void reload(request.direction, request.cursor);
                }}
              />
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
