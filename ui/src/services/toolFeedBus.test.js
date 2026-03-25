import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('./agentlyClient', () => ({
  client: {
    getFeedData: vi.fn(() => Promise.resolve(null)),
  },
}));

describe('toolFeedBus conversation scoping', () => {
  afterEach(async () => {
    const mod = await import('./toolFeedBus');
    mod.clearFeedState();
  });

  it('keeps same feed ids isolated across conversations', async () => {
    const mod = await import('./toolFeedBus');

    mod.applyFeedEvent({
      type: 'tool_feed_active',
      feedId: 'terminal',
      conversationId: 'conv-a',
      feedTitle: 'Terminal',
      feedItemCount: 1,
      feedData: { output: { lines: ['a'] } },
    });
    mod.applyFeedEvent({
      type: 'tool_feed_active',
      feedId: 'terminal',
      conversationId: 'conv-b',
      feedTitle: 'Terminal',
      feedItemCount: 2,
      feedData: { output: { lines: ['b'] } },
    });

    const feeds = mod.getActiveFeeds();
    expect(feeds).toHaveLength(2);
    expect(feeds.map((feed) => feed.feedId).sort()).toEqual(['conv-a::terminal', 'conv-b::terminal']);
    expect(mod.getFeedData('terminal', 'conv-a')?.data?.output?.lines).toEqual(['a']);
    expect(mod.getFeedData('terminal', 'conv-b')?.data?.output?.lines).toEqual(['b']);

    mod.applyFeedEvent({
      type: 'tool_feed_inactive',
      feedId: 'terminal',
      conversationId: 'conv-a',
    });

    const remaining = mod.getActiveFeeds();
    expect(remaining).toHaveLength(1);
    expect(remaining[0].feedId).toBe('conv-b::terminal');
    expect(mod.getFeedData('terminal', 'conv-a')).toBeNull();
    expect(mod.getFeedData('terminal', 'conv-b')?.data?.output?.lines).toEqual(['b']);
  });
});
