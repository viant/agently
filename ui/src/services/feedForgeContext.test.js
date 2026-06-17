import { describe, expect, it, vi } from 'vitest';

vi.mock('./reportExportService', () => ({
  submitReportExportRequest: vi.fn(async ({ request, source }) => ({
    ok: true,
    source,
    title: request?.source?.title || '',
  })),
  getReportExportStatus: vi.fn(async ({ jobId }) => ({
    jobId,
    status: 'queued',
  })),
  getReportExportArtifact: vi.fn(async ({ artifactId }) => ({
    artifactId,
    bytes: new Uint8Array([9, 8, 7]),
  })),
}));

import { createFeedContext } from './feedForgeContext';
import { chatService } from './chatService';
import { getReportExportArtifact, getReportExportStatus, submitReportExportRequest } from './reportExportService';

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

  it('exposes a reportExport handler on root and sub-contexts', async () => {
    const context = createFeedContext('reports', {
      primary: {},
      secondary: { dataSourceRef: 'primary' },
    }, 'conv-1');

    const request = {
      version: 1,
      kind: 'reportExportRequest',
      target: { format: 'pdf' },
      source: {
        from: 'draft',
        artifactRef: 'dashboard.reportBuilder://demo',
        title: 'Demo Report',
      },
    };

    const rootResult = await context.handlers.reportExport.submitRequest({ request, source: 'draft' });
    expect(rootResult).toMatchObject({ ok: true, source: 'draft', title: 'Demo Report' });

    const subContext = context.Context('secondary');
    const subResult = await subContext.handlers.reportExport.submitRequest({ request, source: 'savedPayload' });
    expect(subResult).toMatchObject({ ok: true, source: 'savedPayload', title: 'Demo Report' });

    const status = await subContext.handlers.reportExport.getStatus({ jobId: 'job-1' });
    expect(status).toMatchObject({ jobId: 'job-1', status: 'queued' });

    const artifact = await subContext.handlers.reportExport.getArtifact({ artifactId: 'artifact-1' });
    expect(Array.from(artifact.bytes)).toEqual([9, 8, 7]);

    expect(submitReportExportRequest).toHaveBeenCalledTimes(2);
    expect(submitReportExportRequest).toHaveBeenNthCalledWith(1, { request, source: 'draft' });
    expect(submitReportExportRequest).toHaveBeenNthCalledWith(2, { request, source: 'savedPayload' });
    expect(getReportExportStatus).toHaveBeenCalledWith({ jobId: 'job-1' });
    expect(getReportExportArtifact).toHaveBeenCalledWith({ artifactId: 'artifact-1' });
  });
});
