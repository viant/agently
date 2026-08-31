import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, InputGroup, Spinner } from '@blueprintjs/core';
import { client } from '../services/agentlyClient';
import { resolveConversationSummary, resolveConversationTitle } from '../services/conversationTitle';
import { openConversationInMainWindow } from '../services/conversationWindow';
import {
  conversationStatusTone,
  conversationTimestamp,
  formatRelativeTime,
  normalizeSidebarPage,
  sidebarPageStatusLabel,
  sidebarPaginationRequest,
} from './Sidebar';

const PAGE_SIZE = 12;

export function scheduleHistoryFilter(windowEntry = {}) {
  const parameters = windowEntry?.parameters && typeof windowEntry.parameters === 'object'
    ? windowEntry.parameters
    : {};
  return {
    scheduleId: String(parameters.scheduleId || '').trim(),
    scheduleName: String(parameters.scheduleName || '').trim() || 'Automation',
  };
}

export default function ScheduleConversationHistory({ window: windowEntry }) {
  const { scheduleId, scheduleName } = useMemo(
    () => scheduleHistoryFilter(windowEntry),
    [windowEntry]
  );
  const [query, setQuery] = useState('');
  const [rows, setRows] = useState([]);
  const [prevCursor, setPrevCursor] = useState('');
  const [nextCursor, setNextCursor] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const load = useCallback(async (direction = 'latest', cursor = '') => {
    if (!scheduleId) {
      setRows([]);
      setError('Select an automation to view its run history.');
      return;
    }
    const request = sidebarPaginationRequest(
      direction === 'after' ? 'newer' : direction === 'before' ? 'older' : 'latest',
      cursor
    );
    setLoading(true);
    try {
      const page = await client.listConversations({
        scheduleId,
        query: query.trim() || undefined,
        page: {
          limit: PAGE_SIZE,
          direction: request.direction,
          cursor: request.cursor || undefined,
        },
      });
      const normalized = normalizeSidebarPage(page, request.direction, request.cursor);
      setRows(normalized.rows);
      setPrevCursor(normalized.prevCursor);
      setNextCursor(normalized.nextCursor);
      setError('');
    } catch (loadError) {
      setRows([]);
      setPrevCursor('');
      setNextCursor('');
      setError(String(loadError?.message || loadError || 'Unable to load run history.'));
    } finally {
      setLoading(false);
    }
  }, [query, scheduleId]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load('latest', ''), 200);
    return () => window.clearTimeout(timer);
  }, [load]);

  const pageStatus = sidebarPageStatusLabel({ loading, prevCursor, nextCursor });

  return (
    <section className="app-schedule-conversation-history" aria-label={`${scheduleName} run history`}>
      <div className="app-schedule-conversation-history-header">
        <div>
          <h2>Run History</h2>
          <div className="app-schedule-conversation-history-subtitle">Schedule: {scheduleName}</div>
        </div>
        <Button
          minimal
          icon="refresh"
          aria-label="Refresh schedule run history"
          title="Refresh schedule run history"
          loading={loading}
          onClick={() => void load('latest', '')}
        />
      </div>

      <InputGroup
        leftIcon="search"
        placeholder="Search schedule runs"
        aria-label="Search schedule runs"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        large
      />

      <div className="app-schedule-conversation-history-list">
        {loading && rows.length === 0 ? (
          <div className="app-schedule-conversation-history-state"><Spinner size={22} /></div>
        ) : error ? (
          <div className="app-schedule-conversation-history-state is-error">{error}</div>
        ) : rows.length === 0 ? (
          <div className="app-schedule-conversation-history-state">No runs for {scheduleName}</div>
        ) : rows.map((row) => {
          const id = String(row?.Id || row?.id || '').trim();
          const title = resolveConversationTitle(row);
          const summary = resolveConversationSummary(row);
          const relative = formatRelativeTime(conversationTimestamp(row));
          const open = () => openConversationInMainWindow(id);
          return (
            <div key={id} className="app-conversation-row app-schedule-conversation-history-row">
              <span className={`app-conversation-status-dot tone-${conversationStatusTone(row)}`} />
              <button
                type="button"
                className="app-conversation-row-body"
                title={summary || title}
                aria-label={summary || title}
                onClick={open}
              >
                <div className="app-conversation-topline">
                  <div className="app-conversation-title" title={title}>{title}</div>
                  {relative ? <div className="app-conversation-meta">{relative}</div> : null}
                </div>
                {summary ? <div className="app-conversation-subtitle">{summary}</div> : null}
              </button>
              <Button
                minimal
                icon="eye-open"
                aria-label={`Open ${title}`}
                title="Open conversation"
                onClick={open}
              />
            </div>
          );
        })}
      </div>

      {(loading || prevCursor || nextCursor) ? (
        <div className="app-schedule-conversation-history-pagination">
          <Button
            minimal
            icon="chevron-left"
            aria-label="Load newer schedule runs"
            disabled={!prevCursor || loading}
            onClick={() => void load('after', prevCursor)}
          />
          <span>{pageStatus}</span>
          <Button
            minimal
            icon="chevron-right"
            aria-label="Load older schedule runs"
            disabled={!nextCursor || loading}
            onClick={() => void load('before', nextCursor)}
          />
        </div>
      ) : null}
    </section>
  );
}
