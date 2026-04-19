import { describe, expect, it } from 'vitest';
import { createFeedContext } from './feedForgeContext';
import { chatService } from './chatService';

describe('createFeedContext', () => {
  it('exposes Forge-compatible signals on the root and sub-contexts', () => {
    const context = createFeedContext('plan', {
      tasks: { paging: { enabled: true, size: 3 } },
      details: { dataSourceRef: 'tasks' },
    }, 'conv-1');

    expect(context.identity.windowId).toBe('feed-plan-conv-1');
    expect(context.dataSource).toEqual({ paging: { enabled: true, size: 3 } });
    expect(context.dataSources.tasks).toEqual({ paging: { enabled: true, size: 3 } });
    expect(context.signals.collection).toBeTruthy();
    expect(context.signals.control).toBeTruthy();
    expect(context.signals.selection).toBeTruthy();
    expect(context.signals.form).toBeTruthy();

    const detailContext = context.Context('details');
    expect(detailContext.identity.dataSourceRef).toBe('details');
    expect(detailContext.dataSource).toEqual({ dataSourceRef: 'tasks' });
    expect(detailContext.signals.collection).toBeTruthy();
    expect(detailContext.signals.control).toBeTruthy();
    expect(detailContext.signals.selection).toBeTruthy();
    expect(detailContext.signals.form).toBeTruthy();
    expect(detailContext.handlers.dataSource.peekFilter()).toEqual({});
    expect(detailContext.handlers.dataSource.getCollection()).toEqual([]);
    expect(typeof detailContext.handlers.dataSource.setFilter).toBe('function');
    expect(typeof detailContext.handlers.dataSource.setPage).toBe('function');
  });

  it('paginates feed collections 3 items at a time', () => {
    const context = createFeedContext('explorer', {
      results: {},
    }, 'conv-1');

    const rows = Array.from({ length: 12 }, (_, index) => ({ id: index + 1 }));
    context.handlers.dataSource.setCollection(rows);

    expect(context.handlers.dataSource.getCollectionInfo()).toMatchObject({
      totalCount: 12,
      pageCount: 4,
      pageSize: 3,
      page: 1,
    });
    expect(context.handlers.dataSource.getCollection()).toEqual(rows.slice(0, 3));

    context.handlers.dataSource.setPage(2);
    expect(context.handlers.dataSource.getCollectionInfo()).toMatchObject({ page: 2 });
    expect(context.handlers.dataSource.getCollection()).toEqual(rows.slice(3, 6));

    context.handlers.dataSource.setPage(4);
    expect(context.handlers.dataSource.getCollection()).toEqual(rows.slice(9, 12));
  });

  it('resolves chat handlers for feed UI actions', () => {
    const context = createFeedContext('explorer', { results: {} }, 'conv-1');
    expect(context.lookupHandler('chat.explorerRead')).toBe(chatService.explorerRead);
    expect(context.lookupHandler('chat.taskStatusIcon')).toBe(chatService.taskStatusIcon);
  });

  it('supports selection toggling for table/file browser interactions', () => {
    const context = createFeedContext('explorer', { results: {} }, 'conv-1');
    const row = { uri: '/tmp/file.go' };

    context.handlers.dataSource.toggleSelection({ row, rowIndex: 0 });
    expect(context.handlers.dataSource.getSelection()).toMatchObject({
      selected: row,
      rowIndex: 0,
    });
    expect(context.handlers.dataSource.isSelected({ row, rowIndex: 0 })).toBe(true);

    context.handlers.dataSource.toggleSelection({ row, rowIndex: 0 });
    expect(context.handlers.dataSource.getSelection()).toMatchObject({
      selected: null,
      rowIndex: -1,
    });
  });

  it('can mirror a selected row into form state for feed editors', () => {
    const context = createFeedContext('queue', { queueTurns: {} }, 'conv-1');
    const row = { id: 'turn-q1', preview: 'queued follow-up' };

    context.handlers.dataSource.selectIntoForm({ row, rowIndex: 0 });

    expect(context.handlers.dataSource.getSelection()).toMatchObject({
      selected: row,
      rowIndex: 0,
    });
    expect(context.handlers.dataSource.getFormData()).toMatchObject({
      id: 'turn-q1',
      preview: 'queued follow-up',
    });
  });
});
