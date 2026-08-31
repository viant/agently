import { describe, expect, it, vi } from 'vitest';

const dispatchForgeUIActionMock = vi.hoisted(() => vi.fn());
vi.mock('./forgeUIActions', () => ({ dispatchForgeUIAction: dispatchForgeUIActionMock }));

vi.mock('./reportExportService', () => ({
  submitReportExportRequest: vi.fn(async ({ request, source }) => ({
    ok: true,
    source,
    title: request?.source?.title || '',
  })),
  submitReportExportRun: vi.fn(async ({ request, source }) => ({ ok: true, request, source })),
  submitReportExportSource: vi.fn(async ({ request, source }) => ({ ok: true, request, source })),
  getReportExportStatus: vi.fn(async ({ jobId }) => ({
    jobId,
    status: 'queued',
  })),
  getReportExportArtifact: vi.fn(async ({ artifactId }) => ({
    artifactId,
    bytes: new Uint8Array([9, 8, 7]),
  })),
  listReportExportJobs: vi.fn(async ({ artifactRef, limit }) => ({
    jobs: [{ jobId: 'job-1', artifactRef }],
    totalCount: limit || 1,
  })),
  listReportExportArtifacts: vi.fn(async ({ artifactRef, limit }) => ({
    artifacts: [{ artifactId: 'artifact-1', artifactRef }],
    totalCount: limit || 1,
  })),
}));

import { createFeedContext } from './feedForgeContext';
import { chatService } from './chatService';
import {
  getReportExportArtifact,
  getReportExportStatus,
  listReportExportArtifacts,
  listReportExportJobs,
  submitReportExportRequest,
} from './reportExportService';

describe('createFeedContext', () => {
  it('invokes the configured backend PDF exporter through feed.print', async () => {
    const exportPDF = vi.fn(async () => ({ ok: true }));
    const context = createFeedContext('printable', { result: {} }, 'conv-print', { exportPDF });
    expect(context.lookupHandler('feed.print')()).toBe(true);
    await Promise.resolve();
    await Promise.resolve();
    expect(exportPDF).toHaveBeenCalledTimes(1);
  });
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

  it('mirrors Forge multi-selection and emits an exact local uncheck event', () => {
    dispatchForgeUIActionMock.mockClear();
    const dataSources = {
      result: {},
      options: {
        selectionMode: 'multi',
        uniqueKey: [{ field: 'record_id' }, { field: 'name' }],
        paging: { enabled: true, size: 5 },
        selection: {
          field: 'selected',
          feedbackDataSourceRef: 'selectionStatus',
          callback: { type: 'local', eventName: 'feed_option_selection_changed' },
        },
      },
      selectionStatus: {},
    };
    const context = createFeedContext('selection-editor', dataSources, 'conv-1');
    context.Context('result').handlers.dataSource.setFormData({ values: { recordId: 'record-1', revision: 3 } });
    const options = context.Context('options');
    const rows = [
      { record_id: 'record-1', name: 'Option A' },
      { record_id: 'record-1', name: 'Option B' },
    ];
    options.handlers.dataSource.setCollection(rows);
    options.handlers.dataSource.setSelection({ selected: rows });

    options.handlers.dataSource.toggleSelection({ row: { ...rows[1] }, rowIndex: 1 });

    expect(options.handlers.dataSource.getSelection().selection).toEqual([rows[0]]);
    expect(options.signals.formStatus.value.dirty).toBe(true);
    expect(options.handlers.dataSource.isSelected({ row: { ...rows[0] }, rowIndex: 0 })).toBe(true);
    expect(options.handlers.dataSource.isSelected({ row: { ...rows[1] }, rowIndex: 1 })).toBe(false);
    expect(context.Context('selectionStatus').handlers.dataSource.getFormData()).toMatchObject({
      message: 'Option B excluded; 1 selected.',
      action: 'unselected',
      selectedCount: 1,
      changedCount: 1,
    });
    expect(dispatchForgeUIActionMock).toHaveBeenCalledTimes(1);
    expect(dispatchForgeUIActionMock.mock.calls[0][0]).toMatchObject({
      feedId: 'selection-editor',
      conversationId: 'conv-1',
      dataSourceRef: 'options',
      eventName: 'feed_option_selection_changed',
      action: 'unselected',
      row: { record_id: 'record-1', name: 'Option B', selected: false },
      selectedRows: [{ record_id: 'record-1', name: 'Option A', selected: true }],
      unselectedRows: [{ record_id: 'record-1', name: 'Option B', selected: false }],
      changedRows: [{ record_id: 'record-1', name: 'Option B', selected: false }],
    });

    const submitSelection = options.lookupHandler('feed.submitSelection');
    submitSelection({
      execution: {
        state: {
          callback: { type: 'llm_event', eventName: 'feed_apply_selection' },
          plannerSubmit: {
            domain: 'catalog',
            submitIntent: 'apply_option_selection',
            toolGuidance: { toolBundle: 'catalog-tools' },
          },
          contextBindings: {
            record_id: { dataSourceRef: 'result', path: 'recordId' },
            revision: { dataSourceRef: 'result', path: 'revision' },
          },
        },
      },
    });
    expect(dispatchForgeUIActionMock).toHaveBeenCalledTimes(2);
    expect(dispatchForgeUIActionMock.mock.calls[1][0]).toMatchObject({
      eventName: 'feed_apply_selection',
      callback: { type: 'llm_event', eventName: 'feed_apply_selection' },
      plannerSubmit: {
        domain: 'catalog',
        submitIntent: 'apply_option_selection',
        toolGuidance: { toolBundle: 'catalog-tools' },
      },
      callbackContext: { record_id: 'record-1', revision: 3 },
      selectedRows: [{ record_id: 'record-1', name: 'Option A', selected: true }],
      unselectedRows: [{ record_id: 'record-1', name: 'Option B', selected: false }],
      changedRows: [{ record_id: 'record-1', name: 'Option B', selected: false }],
    });

    options.handlers.dataSource.resetSelection();
    expect(options.handlers.dataSource.getSelection().selection).toEqual([]);
    options.handlers.dataSource.setAllSelection();
    expect(options.handlers.dataSource.getSelection().selection).toEqual(rows);
  });

  it('does not report a synthetic exclusion when a selected collection is replaced', () => {
    const context = createFeedContext('selection-editor', {
      options: {
        selectionMode: 'multi',
        uniqueKey: [{ field: 'Channel' }],
        selection: { field: 'selected', feedbackDataSourceRef: 'selectionStatus' },
      },
      selectionStatus: {},
    }, 'conv-1');
    const options = context.Context('options');
    const rows = [
      { Channel: 'CTV' },
      { Channel: 'Video' },
      { Channel: 'Display' },
      { Channel: 'DOOH' },
      { Channel: 'Audio' },
    ];

    options.handlers.dataSource.replaceCollection({ rows, selectAll: true });

    expect(context.Context('selectionStatus').handlers.dataSource.getFormData()).toMatchObject({
      message: 'Selection updated; 5 selected.',
      selectedCount: 5,
    });
  });

  it('submits feed-local instructions as a structured llm event', () => {
    dispatchForgeUIActionMock.mockClear();
    const context = createFeedContext('editable-record', { instructions: {} }, 'conv-1');
    const instructions = context.Context('instructions');
    instructions.handlers.dataSource.setFormData({ values: {
      record_id: 'record-1',
      instruction: 'Increase the first value and reduce the second.',
    } });
    const submit = instructions.lookupHandler('feed.submitInstructions');
    submit({ execution: { state: {
      callback: { type: 'llm_event', eventName: 'feed_instructions_submit' },
      plannerSubmit: { domain: 'catalog', submitIntent: 'apply_instructions', toolGuidance: { toolBundle: 'catalog-tools' } },
    } } });

    expect(dispatchForgeUIActionMock).toHaveBeenCalledWith(expect.objectContaining({
      eventName: 'feed_instructions_submit',
      formData: {
        record_id: 'record-1',
        instruction: 'Increase the first value and reduce the second.',
      },
    }));
  });

  it('submits one unified draft with form values and multi-selection state', () => {
    dispatchForgeUIActionMock.mockClear();
    const onDraftSubmit = vi.fn();
    const context = createFeedContext('project-plan', {
      identity: {},
      draft: {},
      options: {
        selectionMode: 'multi',
        uniqueKey: [{ field: 'id' }],
        selection: { field: 'selected' },
      },
    }, 'conv-1', { onDraftSubmit });
    context.Context('draft').handlers.dataSource.setFormData({ values: { budget: 42, instruction: 'Prefer option A.' } });
    const options = context.Context('options');
    const rows = [{ id: 'a' }, { id: 'b' }];
    options.handlers.dataSource.setCollection(rows);
    options.handlers.dataSource.setSelection({ selected: [rows[0]] });

    context.lookupHandler('feed.submitDraft')({ execution: { state: {
      formDataSourceRef: 'draft',
      selectionDataSourceRef: 'options',
      callback: { type: 'llm_event', eventName: 'project_plan_update_draft' },
      plannerSubmit: { domain: 'project_plan', submitIntent: 'update_plan_draft' },
    } } });

    expect(dispatchForgeUIActionMock).toHaveBeenCalledWith(expect.objectContaining({
      eventName: 'project_plan_update_draft',
      formData: { budget: 42, instruction: 'Prefer option A.' },
      selectedRows: [{ id: 'a', selected: true }],
      unselectedRows: [{ id: 'b', selected: false }],
      finalDataSourceSnapshot: [{ id: 'a', selected: true }, { id: 'b', selected: false }],
      selections: {
        options: expect.objectContaining({
          selectedRows: [{ id: 'a', selected: true }],
          unselectedRows: [{ id: 'b', selected: false }],
        }),
      },
    }));
    expect(onDraftSubmit).toHaveBeenCalledWith({
      formDataSourceRef: 'draft',
      formData: { budget: 42, instruction: 'Prefer option A.' },
      collections: {
        options: {
          rows,
          selectedRows: [rows[0]],
        },
      },
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

    const jobs = await subContext.handlers.reportExport.listJobs({ artifactRef: 'report://demo', limit: 2 });
    expect(jobs).toMatchObject({ totalCount: 2 });

    const artifacts = await subContext.handlers.reportExport.listArtifacts({ artifactRef: 'report://demo', limit: 3 });
    expect(artifacts).toMatchObject({ totalCount: 3 });

    expect(submitReportExportRequest).toHaveBeenCalledTimes(2);
    expect(submitReportExportRequest).toHaveBeenNthCalledWith(1, { request, source: 'draft' });
    expect(submitReportExportRequest).toHaveBeenNthCalledWith(2, { request, source: 'savedPayload' });
    expect(getReportExportStatus).toHaveBeenCalledWith({ jobId: 'job-1' });
    expect(getReportExportArtifact).toHaveBeenCalledWith({ artifactId: 'artifact-1' });
    expect(listReportExportJobs).toHaveBeenCalledWith({ artifactRef: 'report://demo', limit: 2 });
    expect(listReportExportArtifacts).toHaveBeenCalledWith({ artifactRef: 'report://demo', limit: 3 });
  });
});
