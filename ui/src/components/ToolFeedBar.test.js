import { beforeEach, describe, expect, it, vi } from 'vitest';

const fetchFeedDataNowMock = vi.fn();

vi.mock('../services/toolFeedBus', () => ({
  getActiveFeeds: vi.fn(() => []),
  onFeedChange: vi.fn(() => () => {}),
  fetchFeedDataNow: fetchFeedDataNowMock,
  splitFeedKey: vi.fn((feedKey = '') => {
    const raw = String(feedKey || '').trim();
    const idx = raw.indexOf('::');
    if (idx === -1) return { feedId: raw, conversationId: '' };
    return { conversationId: raw.slice(0, idx), feedId: raw.slice(idx + 2) };
  }),
}));

vi.mock('../services/conversationWindow', () => ({
  getScopedConversationSelection: vi.fn(() => ''),
  getSelectedWindow: vi.fn(() => null),
}));

describe('ToolFeedBar state helpers', () => {
  beforeEach(async () => {
    fetchFeedDataNowMock.mockReset();
    const mod = await import('./ToolFeedBar.jsx');
    mod.__resetToolFeedBarStateForTest();
  });

  it('expands and selects a feed without collapsing it on row selection', async () => {
    const mod = await import('./ToolFeedBar.jsx');
    mod.expandFeed('conv-1::explorer', 'conv-1');

    expect(mod.isFeedExpanded('conv-1::explorer')).toBe(true);
    expect(mod.getSelectedFeedId()).toBe('conv-1::explorer');
    expect(fetchFeedDataNowMock).toHaveBeenCalledWith('conv-1::explorer', 'conv-1');
  });

  it('collapses a feed only when explicitly toggled', async () => {
    const mod = await import('./ToolFeedBar.jsx');
    mod.expandFeed('conv-1::explorer', 'conv-1');
    mod.toggleFeedExpanded('conv-1::explorer', 'conv-1');

    expect(mod.isFeedExpanded('conv-1::explorer')).toBe(false);
    expect(mod.getSelectedFeedId()).toBe('');
  });
});
