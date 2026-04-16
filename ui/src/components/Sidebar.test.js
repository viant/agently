import { describe, expect, it } from 'vitest';
import { applyConversationMetaPatchToRows } from './Sidebar';

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
});
