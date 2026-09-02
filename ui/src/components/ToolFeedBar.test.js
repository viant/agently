import { beforeEach, describe, expect, it, vi } from 'vitest';

const { fetchFeedDataNowMock } = vi.hoisted(() => ({
  fetchFeedDataNowMock: vi.fn(),
}));

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

import * as toolFeedBar from './ToolFeedBar.jsx';
import * as toolFeedSelection from '../services/toolFeedSelection';

describe('ToolFeedBar state helpers', () => {
  beforeEach(() => {
    fetchFeedDataNowMock.mockReset();
    toolFeedBar.__resetToolFeedBarStateForTest();
    toolFeedSelection.registerFeedDataLoader(fetchFeedDataNowMock);
  });

  it('expands and selects a feed without collapsing it on row selection', () => {
    toolFeedBar.expandFeed('conv-1::explorer', 'conv-1');

    expect(toolFeedBar.isFeedExpanded('conv-1::explorer')).toBe(true);
    expect(toolFeedBar.getSelectedFeedId()).toBe('conv-1::explorer');
    expect(fetchFeedDataNowMock).toHaveBeenCalledWith('conv-1::explorer', 'conv-1');
  });

  it('collapses a feed only when explicitly toggled', () => {
    toolFeedBar.expandFeed('conv-1::explorer', 'conv-1');
    toolFeedBar.toggleFeedExpanded('conv-1::explorer', 'conv-1');

    expect(toolFeedBar.isFeedExpanded('conv-1::explorer')).toBe(false);
    expect(toolFeedBar.getSelectedFeedId()).toBe('');
  });

  it('resolves presentation metadata without classifying feed ids', () => {
    expect(toolFeedBar.feedIconName({ icon: 'chart' })).toBe('chart');
    expect(toolFeedBar.feedAccent({ accent: 'teal' })).toBe('#0a9b98');
    expect(toolFeedBar.feedIconName()).toBe('wrench');
    expect(toolFeedBar.feedAccent({ accent: 'not-a-color' })).toBe('#5965d8');
  });
});
