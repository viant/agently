import React from 'react';
import { describe, expect, it, vi } from 'vitest';

vi.mock('../services/toolFeedBus', () => ({
  getActiveFeeds: vi.fn(() => []),
  onFeedChange: vi.fn(() => () => {}),
}));
vi.mock('../services/toolFeedSelection', () => ({ activateExclusiveFeed: vi.fn() }));
vi.mock('./ToolFeedDetail', () => ({ default: () => React.createElement('div', null, 'feed detail') }));

import { filterDetachedFeeds } from './ToolFeedDetached';

describe('detached tool feeds', () => {
  it('selects only detached feeds for the active conversation', () => {
    const feeds = [
      { feedId: 'conv-1::media', conversationId: 'conv-1', presentation: { target: 'detached' } },
      { feedId: 'conv-1::plan', conversationId: 'conv-1', presentation: { target: 'workspace' } },
      { feedId: 'conv-2::other', conversationId: 'conv-2', presentation: { target: 'detached' } },
    ];
    expect(filterDetachedFeeds(feeds, 'conv-1').map((feed) => feed.feedId)).toEqual(['conv-1::media']);
  });
});
