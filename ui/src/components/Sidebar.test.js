import { describe, expect, it } from 'vitest';
import {
  applyConversationMetaPatchToRows,
  conversationDeleteErrorMessage,
  fillDeletedSidebarPageFromOlder,
  normalizeSidebarPageRequest,
  normalizeSidebarPage,
  removeConversationRow,
  sidebarPaginationRequest,
  sidebarPageStatusLabel
} from './Sidebar';

describe('applyConversationMetaPatchToRows', () => {
  it('patches title and summary in memory and re-sorts by updated time', () => {
    const rows = [
      { Id: 'conv-1', Title: 'Old title', Summary: '', UpdatedAt: '2026-04-15T00:00:00Z' },
      { Id: 'conv-2', Title: 'Another', Summary: '', UpdatedAt: '2026-04-15T00:10:00Z' }
    ];

    const got = applyConversationMetaPatchToRows(rows, 'conv-1', {
      title: 'Campaign 4821 Underpacing',
      summary: 'Needs attention'
    });

    expect(got).toHaveLength(2);
    expect(got[0].Id).toBe('conv-1');
    expect(got[0].Title).toBe('Campaign 4821 Underpacing');
    expect(got[0].title).toBe('Campaign 4821 Underpacing');
    expect(got[0].Summary).toBe('Needs attention');
    expect(got[0].summary).toBe('Needs attention');
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
    expect(conversationDeleteErrorMessage({ status: 403, body: 'permission denied' }))
      .toBe('Only the conversation owner can delete this conversation.');
    expect(conversationDeleteErrorMessage({ status: 404, body: 'conversation not found' }))
      .toBe('Conversation was already deleted or is no longer available.');
  });
});
