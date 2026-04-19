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

  it('resolves scoped feed ids without double-scoping lookups or fetches', async () => {
    const mod = await import('./toolFeedBus');
    const { client } = await import('./agentlyClient');

    client.getFeedData.mockResolvedValueOnce({
      data: { output: { lines: ['scoped'] } },
      ui: { name: 'terminal' },
      dataSources: { output: { source: 'output' } },
    });

    mod.applyFeedEvent({
      type: 'tool_feed_active',
      feedId: 'terminal',
      conversationId: 'conv-a',
      feedTitle: 'Terminal',
      feedItemCount: 1,
      feedData: { output: { lines: ['inline'] } },
    });

    expect(mod.getFeedData('conv-a::terminal', 'conv-a')?.data?.output?.lines).toEqual(['inline']);
    mod.fetchFeedDataNow('conv-a::terminal', 'conv-a');
    await Promise.resolve();
    await Promise.resolve();

    expect(client.getFeedData).toHaveBeenLastCalledWith('terminal', 'conv-a');
    expect(mod.getFeedData('conv-a::terminal', 'conv-a')?.data?.output?.lines).toEqual(['scoped']);
  });

  it('preserves inline feed data when a spec fetch returns ui without data', async () => {
    const mod = await import('./toolFeedBus');
    const { client } = await import('./agentlyClient');

    client.getFeedData.mockResolvedValueOnce({
      data: null,
      ui: { title: 'Queue' },
      dataSources: { queueTurns: { source: 'output.queuedTurns' } },
    });

    mod.applyFeedEvent({
      type: 'tool_feed_active',
      feedId: 'queue',
      conversationId: 'conv-q',
      feedTitle: 'Queue',
      feedItemCount: 1,
      feedData: { output: { queuedTurns: [{ id: 'turn-q1', preview: 'queued follow-up' }] } },
    });

    await Promise.resolve();
    await Promise.resolve();

    expect(client.getFeedData).toHaveBeenLastCalledWith('queue', 'conv-q');
    expect(mod.getFeedData('queue', 'conv-q')?.data?.output?.queuedTurns).toEqual([
      { id: 'turn-q1', preview: 'queued follow-up' },
    ]);
    expect(mod.getFeedData('queue', 'conv-q')?.ui?.title).toBe('Queue');
  });
});
