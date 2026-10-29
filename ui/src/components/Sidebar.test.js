// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Sidebar from './Sidebar';
import { client } from '../services/agentlyClient';
import { openConversationInMainWindow } from '../services/conversationWindow';
vi.mock('../services/agentlyClient', () => ({ client: { listConversations: vi.fn() } }));
vi.mock('../services/conversationWindow', () => ({
  MAIN_CHAT_WINDOW_ID: 'chat/new',
  getScopedConversationSelection: () => 'selected',
  getWindowById: vi.fn(),
  isLinkedChildWindow: () => false,
  detachWorkspaceWindowsFromConversation: vi.fn(),
  openConversationInMainWindow: vi.fn(),
  requestNewConversationInMainWindow: vi.fn()
}));
import {
  applyConversationMetaPatchToRows,
  conversationActivityMetaPatch,
  conversationMetaPatchFromSnapshot,
  conversationDeleteButtonDisabled,
  conversationDeletionBlocked,
  conversationDeleteErrorMessage,
  conversationDeleteNeedsTermination,
  deleteConversationWithTermination,
  fillDeletedSidebarPageFromOlder,
  normalizeSidebarPageRequest,
  normalizeSidebarPage,
  removeConversationRow,
  sidebarPaginationRequest,
  sidebarPageStatusLabel,
  terminalConversationMetaPatch
} from './Sidebar';

describe('mounted history refresh', () => {
  let root;
  let container;
  const page = { data: [{ Id: 'selected', Title: 'Saved conversation', Status: 'succeeded' }], page: {} };
  const button = (label) => [...container.querySelectorAll('button')].find((element) => (element.getAttribute('aria-label') || element.textContent) === label);
  const click = async (label) => act(async () => button(label).click());
  async function mount(appearance = 'document', initialPage = page) {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    vi.useFakeTimers();
    client.listConversations.mockReset().mockResolvedValue(initialPage);
    container = document.createElement('div');
    document.body.append(container);
    root = createRoot(container);
    await act(async () => root.render(React.createElement(Sidebar, { appearance, history: { list: { virtualized: false } } })));
    await act(async () => vi.advanceTimersByTimeAsync(250));
  }
  afterEach(async () => {
    if (root) await act(async () => root.unmount());
    container?.remove();
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it.each(['document', ''])('shows failed refresh and retry while retaining rows and selection (%s)', async (appearance) => {
    await mount(appearance);
    client.listConversations.mockRejectedValueOnce(new TypeError('Failed to fetch'));
    await click(appearance ? 'Refresh conversations' : 'Refresh');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Could not refresh conversations');
    expect(container.querySelector('[role="alert"] p')?.getAttribute('data-forge-type')).toBe('small');
    expect(button('Retry').getAttribute('data-forge-variant')).toBe('secondary');
    expect(container.querySelector('.is-selected')?.textContent).toContain('Saved conversation');
    client.listConversations.mockResolvedValueOnce({ ...page, data: [{ ...page.data[0], Title: 'Updated conversation' }] });
    await click('Retry');
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(container.querySelector('.is-selected')?.textContent).toContain('Updated conversation');
    expect(openConversationInMainWindow).not.toHaveBeenCalled();
  });

  it('shows a server refresh error even when history is already populated', async () => {
    await mount();
    client.listConversations.mockRejectedValueOnce(new Error('Service unavailable'));
    await click('Refresh conversations');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Could not refresh conversations');
    expect(container.querySelector('.is-selected')).not.toBeNull();
  });

  it('shows retry instead of an empty-history message when refreshing an empty list fails', async () => {
    await mount('document', { data: [], page: {} });
    client.listConversations.mockRejectedValueOnce(new TypeError('Failed to fetch'));
    await click('Refresh conversations');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Check your connection and retry');
    expect(container.querySelector('.app-sidebar-empty')).toBeNull();
    client.listConversations.mockResolvedValueOnce(page);
    await click('Retry');
    expect(container.querySelector('.is-selected')?.textContent).toContain('Saved conversation');
  });

  it('fetches latest history, ends unchanged loading, and preserves selection without duplicates', async () => {
    await mount();
    let resolve;
    client.listConversations.mockImplementationOnce(() => new Promise((done) => { resolve = done; }));
    await click('Refresh conversations');
    expect(button('Refresh conversations').disabled).toBe(true);
    expect(container.querySelector('.is-selected')).not.toBeNull();
    await act(async () => resolve(page));
    expect(button('Refresh conversations').disabled).toBe(false);
    await click('Refresh conversations');
    expect(container.querySelectorAll('.app-conversation-row')).toHaveLength(1);
    expect(container.querySelector('.is-selected')).not.toBeNull();
    expect(client.listConversations).toHaveBeenLastCalledWith({ excludeScheduled: true, query: undefined, page: { limit: 12, direction: 'latest', cursor: undefined } });
  });

  it('retries the failed older page and appends without duplicating existing rows', async () => {
    await mount('document', { ...page, page: { hasOlder: true, cursor: 'selected' } });
    client.listConversations.mockRejectedValueOnce(new Error('Service unavailable'));
    await click('Load older conversations');
    expect(container.querySelector('[role="alert"]')).not.toBeNull();
    expect(container.querySelector('.is-selected')).not.toBeNull();
    client.listConversations.mockResolvedValueOnce({ data: [...page.data, { Id: 'older', Title: 'Older conversation' }], page: {} });
    await click('Retry');
    expect(client.listConversations).toHaveBeenLastCalledWith({ excludeScheduled: true, query: undefined, page: { limit: 12, direction: 'before', cursor: 'selected' } });
    expect(container.querySelectorAll('.app-conversation-row')).toHaveLength(2);
    expect(container.querySelector('.is-selected')?.textContent).toContain('Saved conversation');
  });

  it('uses existing activity and metadata events to update rows without a full list read', async () => {
    await mount();
    await act(async () => {
      window.dispatchEvent(new CustomEvent('agently:conversation-activity', { detail: { id: 'selected', type: 'turn_failed' } }));
      window.dispatchEvent(new CustomEvent('agently:conversation-meta-updated', { detail: { id: 'selected', patch: { title: 'Renamed conversation' } } }));
    });
    expect(container.querySelector('.is-selected')?.textContent).toContain('Renamed conversation');
    expect(client.listConversations).toHaveBeenCalledTimes(1);
  });

  it('uses the existing new-conversation event to read current backend history', async () => {
    await mount();
    client.listConversations.mockResolvedValueOnce({ data: [{ Id: 'new', Title: 'New conversation' }, ...page.data], page: {} });
    await act(async () => {
      window.dispatchEvent(new CustomEvent('agently:conversation-new', { detail: { id: 'new' } }));
      await vi.advanceTimersByTimeAsync(300);
    });
    expect(client.listConversations).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('New conversation');
  });
});

describe('applyConversationMetaPatchToRows', () => {
  it('patches title and summary without treating selection hydration as activity', () => {
    const rows = [
      { Id: 'conv-1', Title: 'Old title', Summary: '', UpdatedAt: '2026-04-15T00:00:00Z' },
      { Id: 'conv-2', Title: 'Another', Summary: '', UpdatedAt: '2026-04-15T00:10:00Z' }
    ];

    const got = applyConversationMetaPatchToRows(rows, 'conv-1', {
      title: 'Campaign 4821 Underpacing',
      summary: 'Needs attention'
    });

    expect(got).toHaveLength(2);
    expect(got.map((row) => row.Id)).toEqual(['conv-1', 'conv-2']);
    expect(got[0].Title).toBe('Campaign 4821 Underpacing');
    expect(got[0].title).toBe('Campaign 4821 Underpacing');
    expect(got[0].Summary).toBe('Needs attention');
    expect(got[0].summary).toBe('Needs attention');
    expect(got[0].UpdatedAt).toBe('2026-04-15T00:00:00Z');
  });

  it('re-sorts only when canonical last activity changes', () => {
    const rows = [
      { Id: 'conv-1', LastActivity: '2026-04-15T00:00:00Z' },
      { Id: 'conv-2', LastActivity: '2026-04-15T00:10:00Z' }
    ];

    const got = applyConversationMetaPatchToRows(rows, 'conv-1', {
      status: 'succeeded',
      lastActivity: '2026-04-15T00:20:00Z'
    });

    expect(got.map((row) => row.Id)).toEqual(['conv-1', 'conv-2']);
    expect(got[0].LastActivity).toBe('2026-04-15T00:20:00Z');
  });

  it('patches stage, status, and running flags for immediate sidebar tone updates', () => {
    const rows = [
      { Id: 'conv-1', Title: 'Compare run', Status: 'running', Stage: 'executing', running: true, UpdatedAt: '2026-04-15T00:00:00Z' }
    ];

    const got = applyConversationMetaPatchToRows(rows, 'conv-1', {
      status: 'succeeded',
      stage: 'done',
      running: false
    });

    expect(got[0].Status).toBe('succeeded');
    expect(got[0].status).toBe('succeeded');
    expect(got[0].Stage).toBe('done');
    expect(got[0].stage).toBe('done');
    expect(got[0].Running).toBe(false);
    expect(got[0].running).toBe(false);
  });
});

describe('sidebar status reconciliation', () => {
  it('maps a canonical completed snapshot over a stale running row', () => {
    expect(conversationMetaPatchFromSnapshot({
      Status: 'succeeded',
      Stage: 'done',
      Running: false
    })).toEqual({ status: 'succeeded', stage: 'done', running: false });
  });

  it('maps terminal stream events without waiting for a list refresh', () => {
    expect(terminalConversationMetaPatch('turn_completed')).toEqual({
      status: 'succeeded',
      stage: 'done',
      running: false
    });
    expect(terminalConversationMetaPatch('turn_failed')).toEqual({
      status: 'failed',
      stage: 'error',
      running: false
    });
  });

  it('patches conversation activity locally instead of requiring a history reload', () => {
    const occurredAt = '2026-09-16T18:30:00.000Z';

    expect(conversationActivityMetaPatch({
      type: 'turn_started',
      lastActivity: occurredAt
    })).toEqual({
      status: 'running',
      stage: 'executing',
      running: true,
      lastActivity: occurredAt
    });
    expect(conversationActivityMetaPatch({
      type: 'turn_completed'
    }, Date.parse(occurredAt))).toEqual({
      status: 'succeeded',
      stage: 'done',
      running: false,
      lastActivity: occurredAt
    });
    expect(conversationActivityMetaPatch({ type: 'turn_refreshed' })).toBeNull();
  });
});

describe('sidebar conversation pagination', () => {
  it('enables only older navigation on the newest cursor page', () => {
    const got = normalizeSidebarPage({
      data: [
        { Id: 'newest', LastActivity: '2026-05-08T10:00:00Z' },
        { Id: 'older', LastActivity: '2026-05-08T09:00:00Z' }
      ],
      page: {
        prevCursor: 'newest',
        cursor: 'older',
        hasMore: true,
        hasOlder: true,
        hasNewer: false
      }
    }, 'latest');

    expect(got.prevCursor).toBe('');
    expect(got.nextCursor).toBe('older');
    expect(sidebarPageStatusLabel(got)).toBe('Newest');
  });

  it('enables both directions on a middle cursor page', () => {
    const got = normalizeSidebarPage({
      data: [
        { Id: 'middle-newer', LastActivity: '2026-05-08T08:00:00Z' },
        { Id: 'middle-older', LastActivity: '2026-05-08T07:00:00Z' }
      ],
      page: {
        prevCursor: 'middle-newer',
        cursor: 'middle-older',
        hasMore: true,
        hasOlder: true,
        hasNewer: true
      }
    }, 'after');

    expect(got.prevCursor).toBe('middle-newer');
    expect(got.nextCursor).toBe('middle-older');
    expect(sidebarPageStatusLabel(got)).toBe('Middle');
  });

  it('enables only newer navigation on the oldest cursor page', () => {
    const got = normalizeSidebarPage({
      data: [
        { Id: 'oldest-newer', LastActivity: '2026-05-05T10:00:00Z' },
        { Id: 'oldest', LastActivity: '2026-05-05T09:00:00Z' }
      ],
      page: {
        prevCursor: 'oldest-newer',
        cursor: 'oldest',
        hasMore: false,
        hasOlder: false,
        hasNewer: true
      }
    }, 'after');

    expect(got.prevCursor).toBe('oldest-newer');
    expect(got.nextCursor).toBe('');
    expect(sidebarPageStatusLabel(got)).toBe('Oldest');
  });

  it('keeps newer navigation when the oldest page has a single row', () => {
    const got = normalizeSidebarPage({
      data: [
        { Id: 'oldest-only', LastActivity: '2026-05-05T09:00:00Z' }
      ],
      page: {
        prevCursor: 'oldest-only',
        cursor: 'oldest-only',
        hasMore: false,
        hasOlder: false,
        hasNewer: true
      }
    }, 'before', 'middle-edge');

    expect(got.prevCursor).toBe('oldest-only');
    expect(got.nextCursor).toBe('');
    expect(sidebarPageStatusLabel(got)).toBe('Oldest');
  });

  it('maps sidebar buttons to backend cursor directions', () => {
    expect(sidebarPaginationRequest('older', 'old-edge')).toEqual({
      direction: 'before',
      cursor: 'old-edge'
    });
    expect(sidebarPaginationRequest('newer', 'new-edge')).toEqual({
      direction: 'after',
      cursor: 'new-edge'
    });
  });

  it('falls back to latest when cursor pagination has no cursor', () => {
    expect(normalizeSidebarPageRequest('before', '')).toEqual({
      direction: 'latest',
      cursor: ''
    });
    expect(sidebarPaginationRequest('older', '')).toEqual({
      direction: 'latest',
      cursor: ''
    });
  });

  it('fills a deleted middle-page row from the next older page', () => {
    const got = fillDeletedSidebarPageFromOlder({
      rows: [
        { Id: 'row-2', LastActivity: '2026-05-08T11:00:00Z' },
        { Id: 'row-3', LastActivity: '2026-05-08T10:00:00Z' }
      ],
      olderPage: {
        rows: [
          { Id: 'row-4', LastActivity: '2026-05-08T09:00:00Z' },
          { Id: 'row-5', LastActivity: '2026-05-08T08:00:00Z' }
        ],
        nextCursor: 'row-5'
      },
      hadNewer: true,
      hadOlder: true,
      pageSize: 3
    });

    expect(got.rows.map((row) => row.Id)).toEqual(['row-2', 'row-3', 'row-4']);
    expect(got.prevCursor).toBe('row-2');
    expect(got.nextCursor).toBe('row-4');
  });
});

describe('conversation delete helpers', () => {
  it('keeps delete available for active-looking rows until their own request starts', () => {
    const row = { Id: 'conv-running', status: 'running', running: true };
    expect(conversationDeleteButtonDisabled(row, '')).toBe(false);
    expect(conversationDeleteButtonDisabled(row, 'conv-other')).toBe(false);
    expect(conversationDeleteButtonDisabled(row, 'conv-running')).toBe(true);
  });

  it('blocks deletion only for explicitly active conversations', () => {
    expect(conversationDeletionBlocked({ status: 'running' })).toBe(true);
    expect(conversationDeletionBlocked({ running: true })).toBe(true);
    expect(conversationDeletionBlocked({ status: 'succeeded', stage: 'done' })).toBe(false);
    expect(conversationDeletionBlocked({ status: 'succeeded', stage: 'executing', running: true })).toBe(false);
    expect(conversationDeletionBlocked({ title: 'Legacy row without lifecycle' })).toBe(false);
  });

  it('removes only the deleted conversation row', () => {
    const rows = [
      { Id: 'conv-1', Title: 'One' },
      { id: 'conv-2', Title: 'Two' },
      { Id: 'conv-3', Title: 'Three' }
    ];

    expect(removeConversationRow(rows, 'conv-2')).toEqual([
      { Id: 'conv-1', Title: 'One' },
      { Id: 'conv-3', Title: 'Three' }
    ]);
  });

  it('maps delete failures to actionable messages', () => {
    expect(conversationDeleteErrorMessage({ status: 409, body: 'conversation is still in progress' }))
      .toBe('Conversation is still in progress and cannot be deleted yet.');
    expect(conversationDeleteErrorMessage({ status: 409, body: 'conversation_graph_non_terminal: blocked' }))
      .toBe('Conversation or one of its child conversations has not finished yet.');
    expect(conversationDeleteErrorMessage({ status: 409, body: 'conversation_graph_referenced: blocked' }))
      .toBe('This conversation is still linked from another conversation and cannot be deleted.');
    expect(conversationDeleteErrorMessage({ status: 409, body: 'conversation_schedule_referenced: blocked' }))
      .toBe('A user schedule still references this conversation. Remove the schedule first.');
    expect(conversationDeleteErrorMessage({ status: 409, body: 'conversation_graph_too_large: blocked' }))
      .toBe('This conversation tree is too large to delete in one operation.');
    expect(conversationDeleteErrorMessage({ status: 409, payload: { error: 'conversation_graph_non_terminal: blocked' } }))
      .toBe('Conversation or one of its child conversations has not finished yet.');
    expect(conversationDeleteErrorMessage({ status: 403, body: 'permission denied' }))
      .toBe('Only the conversation owner can delete this conversation.');
    expect(conversationDeleteErrorMessage({ status: 404, body: 'conversation not found' }))
      .toBe('Conversation was already deleted or is no longer available.');
  });

  it('terminates and retries a backend-blocked non-terminal conversation', async () => {
    const calls = [];
    const api = {
      deleteConversation: vi.fn(async () => {
        calls.push('delete');
        if (calls.length === 1) {
          const err = new Error('conversation_graph_non_terminal: blocked');
          err.status = 409;
          throw err;
        }
      }),
      terminateConversation: vi.fn(async () => calls.push('terminate')),
    };

    expect(conversationDeleteNeedsTermination({ status: 409, body: 'conversation_graph_non_terminal: blocked' })).toBe(true);
    await deleteConversationWithTermination(api, 'conv-running');

    expect(calls).toEqual(['delete', 'terminate', 'delete']);
    expect(api.terminateConversation).toHaveBeenCalledWith('conv-running');
  });
});
