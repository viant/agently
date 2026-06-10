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

  it('unwraps feed payload envelopes that place ui and data under a nested data object', async () => {
    const mod = await import('./toolFeedBus');

    mod.updateFeedData('goal', {
      data: {
        ui: {
          title: 'Goal',
          renderMode: 'forge',
          dataSources: {
            goalState: { source: 'goal' },
          },
        },
        data: {
          goal: {
            objective: 'Ship the Go task',
            status: 'active',
          },
        },
      },
    }, 'conv-goal');

    const feed = mod.getFeedData('goal', 'conv-goal');
    expect(feed?.ui?.title).toBe('Goal');
    expect(feed?.ui?.renderMode).toBe('forge');
    expect(feed?.data?.goal?.objective).toBe('Ship the Go task');
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

  it('remembers inactive feeds until a fresh active event arrives', async () => {
    const mod = await import('./toolFeedBus');

    mod.applyFeedEvent({
      type: 'tool_feed_active',
      feedId: 'plan',
      conversationId: 'conv-plan',
      feedTitle: 'Plan',
      feedItemCount: 1,
      feedData: { output: { rows: [{ id: 1 }] } },
    });

    expect(mod.isFeedInactive('plan', 'conv-plan')).toBe(false);

    mod.applyFeedEvent({
      type: 'tool_feed_inactive',
      feedId: 'plan',
      conversationId: 'conv-plan',
    });

    expect(mod.isFeedInactive('plan', 'conv-plan')).toBe(true);
    expect(mod.getFeedData('plan', 'conv-plan')).toBeNull();
    expect(mod.getActiveFeeds()).toHaveLength(0);

    mod.applyFeedEvent({
      type: 'tool_feed_active',
      feedId: 'plan',
      conversationId: 'conv-plan',
      feedTitle: 'Plan',
      feedItemCount: 1,
      feedData: { output: { rows: [{ id: 2 }] } },
    });

    expect(mod.isFeedInactive('plan', 'conv-plan')).toBe(false);
    expect(mod.getFeedData('plan', 'conv-plan')?.data?.output?.rows).toEqual([{ id: 2 }]);
    expect(mod.getActiveFeeds()).toHaveLength(1);
  });

  it('clears shared feed selection when feed state is cleared', async () => {
    const mod = await import('./toolFeedBus');
    const selection = await import('./toolFeedSelection');

    mod.applyFeedEvent({
      type: 'tool_feed_active',
      feedId: 'plan',
      conversationId: 'conv-plan',
      feedTitle: 'Plan',
      feedItemCount: 1,
      feedData: { output: { rows: [{ id: 1 }] } },
    });
    selection.activateExclusiveFeed('conv-plan::plan', 'conv-plan');

    expect(selection.getSelectedFeedId('conv-plan')).toBe('conv-plan::plan');

    mod.clearFeedState();

    expect(selection.getSelectedFeedId('conv-plan')).toBe('');
    expect(Array.from(selection.getExpandedFeedIds())).toEqual([]);
  });

  it('does not refetch feed spec when the scoped feed already has ui and dataSources', async () => {
    const mod = await import('./toolFeedBus');
    const { client } = await import('./agentlyClient');

    client.getFeedData.mockReset();
    client.getFeedData.mockResolvedValue({
      data: { output: { queuedTurns: [{ id: 'turn-q1' }] } },
      ui: { title: 'Queue' },
      dataSources: { queueTurns: { source: 'output.queuedTurns' } },
    });

    mod.applyFeedEvent({
      type: 'tool_feed_active',
      feedId: 'queue',
      conversationId: 'conv-q',
      feedTitle: 'Queue',
      feedItemCount: 1,
      feedData: { output: { queuedTurns: [{ id: 'turn-q1' }] } },
    });

    await Promise.resolve();
    await Promise.resolve();

    expect(client.getFeedData).toHaveBeenCalledTimes(1);

    mod.applyFeedEvent({
      type: 'tool_feed_active',
      feedId: 'queue',
      conversationId: 'conv-q',
      feedTitle: 'Queue',
      feedItemCount: 1,
      feedData: { output: { queuedTurns: [{ id: 'turn-q1' }] } },
    });

    await Promise.resolve();
    await Promise.resolve();

    expect(client.getFeedData).toHaveBeenCalledTimes(1);
  });

  it('clears feed state only for the targeted conversation', async () => {
    const mod = await import('./toolFeedBus');
    const selection = await import('./toolFeedSelection');

    mod.applyFeedEvent({
      type: 'tool_feed_active',
      feedId: 'plan',
      conversationId: 'conv-a',
      feedTitle: 'Plan',
      feedItemCount: 1,
      feedData: { output: { rows: [{ id: 1 }] } },
    });
    mod.applyFeedEvent({
      type: 'tool_feed_active',
      feedId: 'changes',
      conversationId: 'conv-b',
      feedTitle: 'Changes',
      feedItemCount: 1,
      feedData: { output: { changes: [{ path: 'b.go' }] } },
    });
    selection.activateExclusiveFeed('conv-a::plan', 'conv-a');
    selection.activateExclusiveFeed('conv-b::changes', 'conv-b');

    mod.clearFeedStateForConversation('conv-a');

    expect(mod.getFeedData('plan', 'conv-a')).toBeNull();
    expect(mod.getFeedData('changes', 'conv-b')?.data?.output?.changes).toEqual([{ path: 'b.go' }]);
    expect(mod.getActiveFeeds().map((feed) => feed.feedId)).toEqual(['conv-b::changes']);
    expect(selection.getSelectedFeedId('conv-a')).toBe('');
    expect(selection.getSelectedFeedId('conv-b')).toBe('conv-b::changes');
  });
});
