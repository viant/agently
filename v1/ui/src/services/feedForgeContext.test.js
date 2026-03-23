import { describe, expect, it } from 'vitest';
import { createFeedContext } from './feedForgeContext';

describe('createFeedContext', () => {
  it('exposes Forge-compatible signals on the root and sub-contexts', () => {
    const context = createFeedContext('plan', {
      tasks: {},
      details: { dataSourceRef: 'tasks' },
    }, 'conv-1');

    expect(context.identity.windowId).toBe('feed-plan-conv-1');
    expect(context.signals.collection).toBeTruthy();
    expect(context.signals.control).toBeTruthy();
    expect(context.signals.selection).toBeTruthy();
    expect(context.signals.form).toBeTruthy();

    const detailContext = context.Context('details');
    expect(detailContext.identity.dataSourceRef).toBe('details');
    expect(detailContext.signals.collection).toBeTruthy();
    expect(detailContext.signals.control).toBeTruthy();
    expect(detailContext.signals.selection).toBeTruthy();
    expect(detailContext.signals.form).toBeTruthy();
    expect(detailContext.handlers.dataSource.peekFilter()).toEqual({});
    expect(detailContext.handlers.dataSource.getCollection()).toEqual([]);
    expect(typeof detailContext.handlers.dataSource.setFilter).toBe('function');
    expect(typeof detailContext.handlers.dataSource.setPage).toBe('function');
  });
});
