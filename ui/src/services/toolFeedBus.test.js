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

  it('moves preview updates to the new turn and resists stale transcript replay', async () => {
    const mod = await import('./toolFeedBus');
    const { getFormSignal } = await import('forge/core');
    mod.applyFeedEvent({
      type: 'tool_feed_active', feedId: 'editable-record', conversationId: 'conv-record', turnId: 'turn-1',
      feedTitle: 'Editable Record', feedTarget: 'inline', feedItemCount: 1, feedData: { output: { record: { amount: 250 } } },
    });
    mod.updateFeedData('editable-record', {
      dataSources: {
        result: { source: 'output' },
        record: { dataSourceRef: 'result', selectors: { data: 'record' } },
      },
      data: { output: { record: { amount: 250 } } },
    }, 'conv-record');
    getFormSignal('feed-editable-record-conv-recordDSrecord').value = { amount: 250 };

    expect(mod.applyActiveFeedUpdate({
      feedId: 'editable-record', conversationId: 'conv-record', turnId: 'turn-2',
      operations: [{ dataSourceRef: 'record', op: 'replace', path: '/amount', value: 260 }],
    })).toBe(true);
    expect(mod.getFeedData('editable-record', 'conv-record')?.data?.output?.record?.amount).toBe(260);
    expect(mod.getActiveFeeds()[0].turnId).toBe('turn-2');

    mod.applyFeedEvent({
      type: 'tool_feed_active', feedId: 'editable-record', conversationId: 'conv-record', turnId: 'turn-1',
      feedTitle: 'Editable Record', feedTarget: 'inline', feedItemCount: 1,
    });
    expect(mod.getActiveFeeds()[0].turnId).toBe('turn-2');

    mod.applyFeedEvent({
      type: 'tool_feed_active', feedId: 'editable-record', conversationId: 'conv-record', turnId: 'turn-2',
      createdAt: '2026-08-30T12:00:00Z', feedTitle: 'Editable Record', feedTarget: 'inline', feedItemCount: 1,
      feedData: { output: { record: { amount: 250 } } },
    });
    expect(mod.getFeedData('editable-record', 'conv-record')?.data?.output?.record?.amount).toBe(260);
    expect(mod.getFeedData('editable-record', 'conv-record')?._dirtyDataSourceRefs).toContain('record');
  });

  it('maps form, collection, and selection view paths onto one canonical feed datasource', async () => {
    const mod = await import('./toolFeedBus');
    const { getCollectionSignal } = await import('forge/core');
    mod.applyFeedEvent({
      type: 'tool_feed_active', feedId: 'view-paths', conversationId: 'conv-view', turnId: 'turn-1',
      feedTitle: 'View Paths', feedTarget: 'inline', feedItemCount: 1,
      feedData: { output: { record: { rows: [{ name: 'First', value: 'old' }] }, status: {} } },
    });
    mod.updateFeedData('view-paths', {
      dataSources: {
        result: { source: 'output' },
        record: { dataSourceRef: 'result', selectors: { data: 'record' } },
        rows: { dataSourceRef: 'record', selectors: { data: 'rows' } },
        status: { source: 'output.status' },
      },
      data: { output: { record: { rows: [{ name: 'First', value: 'old' }] }, status: {} } },
    }, 'conv-view');

    expect(mod.applyActiveFeedUpdate({
      feedId: 'view-paths', conversationId: 'conv-view', turnId: 'turn-2',
      operations: [
        { dataSourceRef: 'rows', op: 'replace', path: '/collection/0/value', value: 'new' },
        { dataSourceRef: 'rows', op: 'replace', path: '/selection/selection/0/value', value: 'new' },
        { dataSourceRef: 'status', op: 'add', path: '/form/message', value: 'Preview staged' },
      ],
    })).toBe(true);
    expect(mod.getFeedData('view-paths', 'conv-view')?.data).toEqual({
      output: {
        record: { rows: [{ name: 'First', value: 'new' }] },
        status: { message: 'Preview staged' },
      },
    });
    expect(mod.getFeedData('view-paths', 'conv-view')?._dirtyDataSourceRefs.sort()).toEqual(['rows', 'status']);
    expect(getCollectionSignal('feed-view-paths-conv-viewDSrows').value).toEqual([{ name: 'First', value: 'new' }]);
  });

  it('patches first, middle, and last collection items without shifting indices', async () => {
    const mod = await import('./toolFeedBus');
    const items = [
      { id: 'a', value: 1 },
      { id: 'b', value: 2 },
      { id: 'c', value: 3 },
      { id: 'd', value: 4 },
      { id: 'e', value: 5 },
    ];
    mod.applyFeedEvent({
      type: 'tool_feed_active', feedId: 'editable-list', conversationId: 'conv-positions', turnId: 'turn-1',
      feedTitle: 'Editable List', feedTarget: 'inline', feedItemCount: items.length,
      feedData: { output: { record: { items } } },
    });
    mod.updateFeedData('editable-list', {
      dataSources: {
        result: { source: 'output' },
        record: { dataSourceRef: 'result', selectors: { data: 'record' } },
        items: { dataSourceRef: 'record', selectors: { data: 'items' } },
      },
      data: { output: { record: { items } } },
    }, 'conv-positions');

    expect(mod.applyActiveFeedUpdate({
      feedId: 'editable-list', conversationId: 'conv-positions', turnId: 'turn-2',
      operations: [
        { dataSourceRef: 'items', op: 'replace', path: '/collection/0/value', value: 10 },
        { dataSourceRef: 'items', op: 'replace', path: '/collection/2/value', value: 30 },
        { dataSourceRef: 'items', op: 'replace', path: '/collection/4/value', value: 50 },
      ],
    })).toBe(true);

    expect(mod.getFeedData('editable-list', 'conv-positions')?.data?.output?.record?.items).toEqual([
      { id: 'a', value: 10 },
      { id: 'b', value: 2 },
      { id: 'c', value: 30 },
      { id: 'd', value: 4 },
      { id: 'e', value: 50 },
    ]);
    expect(mod.getFeedData('editable-list', 'conv-positions')?._dirtyDataSourceRefs).toContain('items');
  });

  it('does not let a delayed backend fetch overwrite a newer dirty preview', async () => {
    const mod = await import('./toolFeedBus');
    const { client } = await import('./agentlyClient');
    let resolveFetch;
    client.getFeedData.mockReturnValueOnce(new Promise((resolve) => { resolveFetch = resolve; }));
    mod.applyFeedEvent({
      type: 'tool_feed_active', feedId: 'race-safe-feed', conversationId: 'conv-race', turnId: 'turn-1',
      feedTitle: 'Race-safe feed', feedTarget: 'inline', feedItemCount: 1,
      feedData: { output: { record: { value: 'clean' } } },
    });
    mod.updateFeedData('race-safe-feed', {
      dataSources: {
        result: { source: 'output' },
        record: { dataSourceRef: 'result', selectors: { data: 'record' } },
      },
    }, 'conv-race');

    mod.fetchFeedDataNow('race-safe-feed', 'conv-race');
    expect(mod.applyActiveFeedUpdate({
      feedId: 'race-safe-feed', conversationId: 'conv-race', turnId: 'turn-2',
      operations: [{ dataSourceRef: 'record', op: 'replace', path: '/value', value: 'preview' }],
    })).toBe(true);
    resolveFetch({
      data: { output: { record: { value: 'stale' } } },
      ui: { title: 'Fetched metadata' },
    });
    await Promise.resolve();
    await Promise.resolve();

    expect(mod.getFeedData('race-safe-feed', 'conv-race')?.data?.output?.record?.value).toBe('preview');
    expect(mod.getFeedData('race-safe-feed', 'conv-race')?._dirtyDataSourceRefs).toContain('record');
    expect(mod.getFeedData('race-safe-feed', 'conv-race')?.ui?.title).toBe('Fetched metadata');
  });

  it('applies cached array removals exactly once before Forge rewiring', async () => {
    vi.useFakeTimers();
    try {
      const mod = await import('./toolFeedBus');
      const { getCollectionSignal, getFormSignal, getFormStatusSignal } = await import('forge/core');
      mod.applyFeedEvent({
        type: 'tool_feed_active', feedId: 'editable-array', conversationId: 'conv-array', turnId: 'turn-1',
        feedTitle: 'Editable Array', feedTarget: 'inline', feedItemCount: 1,
        feedData: { output: { record: { items: ['first', 'middle', 'last'] } } },
      });
      mod.updateFeedData('editable-array', {
        dataSources: {
          result: { source: 'output' },
          record: { dataSourceRef: 'result', selectors: { data: 'record' } },
          items: { dataSourceRef: 'record', selectors: { data: 'items' } },
        },
        data: { output: { record: { items: ['first', 'middle', 'last'] } } },
      }, 'conv-array');
      const dataSourceId = 'feed-editable-array-conv-arrayDSrecord';
      getFormSignal(dataSourceId).value = { items: ['first', 'middle', 'last'] };
      getCollectionSignal(dataSourceId).value = [{ items: ['first', 'middle', 'last'] }];

      expect(mod.applyActiveFeedUpdate({
        feedId: 'editable-array', conversationId: 'conv-array', turnId: 'turn-2',
        operations: [
          { dataSourceRef: 'items', op: 'remove', path: '/1' },
          { dataSourceRef: 'record', op: 'remove', path: '/items/1' },
        ],
      })).toBe(true);
      await vi.runAllTimersAsync();

      expect(mod.getFeedData('editable-array', 'conv-array')?.data?.output?.record?.items).toEqual(['first', 'last']);
      expect(getFormSignal(dataSourceId).value.items).toEqual(['first', 'last']);
      expect(getCollectionSignal(dataSourceId).value[0].items).toEqual(['first', 'last']);
      expect(getFormStatusSignal('feed-editable-array-conv-arrayDSitems').value.dirty).toBe(true);
      expect(getFormStatusSignal(dataSourceId).value.dirty).toBe(true);
    } finally {
      vi.useRealTimers();
    }
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
      presentation: { icon: 'terminal', accent: 'orange' },
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
    expect(mod.getActiveFeeds()[0]?.presentation).toEqual({ icon: 'terminal', accent: 'orange' });
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

  it('preserves fetched UI metadata across later live feed updates', async () => {
    const mod = await import('./toolFeedBus');
    mod.applyFeedEvent({
      type: 'tool_feed_active', feedId: 'changes', conversationId: 'conv-live',
      feedTitle: 'Changes', feedItemCount: 1, feedData: { output: { changes: [{ url: '/tmp/a' }] } },
    });
    mod.updateFeedData('changes', {
      ui: { containers: [{ fileBrowser: { preview: { kind: 'codeDiff' } } }] },
      dataSources: { changes: { source: 'output.changes' } },
    }, 'conv-live');
    mod.applyFeedEvent({
      type: 'tool_feed_active', feedId: 'changes', conversationId: 'conv-live',
      feedTitle: 'Changes', feedItemCount: 1, feedData: { output: { changes: [{ url: '/tmp/b' }] } },
    });

    expect(mod.getFeedData('changes', 'conv-live')?.ui?.containers?.[0]?.fileBrowser?.preview?.kind).toBe('codeDiff');
    expect(mod.getFeedData('changes', 'conv-live')?.data?.output?.changes?.[0]?.url).toBe('/tmp/b');
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
