import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  activateReportRun,
  adoptReportRun,
  beginReportRun,
  completeReportRun,
  failReportRun,
  getReportRunContext,
} from './reportRunService';

afterEach(() => {
  vi.unstubAllGlobals();
});

function response(status, body) {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => (body == null ? '' : JSON.stringify(body)),
  };
}

describe('reportRunService', () => {
  it('treats only an absent default-closed route as legacy feature off', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      status: 404,
      text: async () => '404 page not found\n',
    })));

    await expect(beginReportRun({ uiRunRequestId: 'request-1' })).resolves.toEqual({ enabled: false });
  });

  it('surfaces a scoped JSON 404 from the mounted endpoint', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => response(404, { error: 'report run: not found' })));

    await expect(beginReportRun({ uiRunRequestId: 'request-1' })).rejects.toMatchObject({
      message: 'report run: not found',
      status: 404,
    });
  });

  it('surfaces enabled persistence failures instead of reporting legacy success', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => response(500, { error: 'database unavailable' })));

    await expect(beginReportRun({ uiRunRequestId: 'request-1' })).rejects.toMatchObject({
      message: 'database unavailable',
      status: 500,
    });
  });

  it('treats only an unmounted plain-text adopt 404 as feature off', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      status: 404,
      text: async () => '404 page not found\n',
    })));

    await expect(adoptReportRun({
      reportRunId: 'run-server-1',
      conversationId: 'conv-2',
      expectedRunRevision: 3,
      expectedContextRevision: 4,
      source: 'manual',
    })).resolves.toEqual({ enabled: false });
  });

  it.each([
    [404, 'report run: not found'],
    [401, 'authorization required'],
    [403, 'forbidden'],
    [409, 'report run revision conflict'],
    [500, 'database unavailable'],
  ])('surfaces structured adopt error %s', async (status, message) => {
    vi.stubGlobal('fetch', vi.fn(async () => response(status, { error: message })));

    await expect(adoptReportRun({
      reportRunId: 'run-server-1',
      conversationId: 'conv-2',
      expectedRunRevision: 3,
      expectedContextRevision: 4,
      source: 'manual',
    })).rejects.toMatchObject({ message, status });
  });

  it('reads the exact authenticated conversation context without a request body', async () => {
    const fetchMock = vi.fn(async () => response(200, {
      conversationId: 'conv/2',
      activeReportRunId: 'run-existing',
      revision: 7,
    }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(getReportRunContext({ conversationId: ' conv/2 ' })).resolves.toEqual({
      enabled: true,
      context: {
        conversationId: 'conv/2',
        activeReportRunId: 'run-existing',
        revision: 7,
      },
    });
    expect(fetchMock).toHaveBeenCalledWith('/v1/api/report-runs/context/conv%2F2', {
      method: 'GET',
      credentials: 'include',
      headers: { Accept: 'application/json' },
    });
  });

  it('distinguishes an unavailable context route from a scoped context 404', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: false,
        status: 404,
        text: async () => '404 page not found\n',
      })
      .mockResolvedValueOnce(response(404, { error: 'report run: not found' }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(getReportRunContext({ conversationId: 'conv-2' })).resolves.toEqual({
      enabled: false,
      context: null,
    });
    await expect(getReportRunContext({ conversationId: 'conv-2' })).resolves.toEqual({
      enabled: true,
      context: null,
    });
  });

  it.each([
    [401, 'authorization required'],
    [403, 'forbidden'],
    [500, 'database unavailable'],
  ])('surfaces exact context-read error %s', async (status, message) => {
    vi.stubGlobal('fetch', vi.fn(async () => response(status, { error: message })));

    await expect(getReportRunContext({ conversationId: 'conv-2' }))
      .rejects.toMatchObject({ message, status });
  });

  it('omits Forge-only correlation fields from exact Core request bodies', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(response(200, {
        run: { reportRunId: 'run-server-1', revision: 1, status: 'running' },
      }))
      .mockResolvedValue(response(200, { reportRunId: 'run-server-1' }));
    vi.stubGlobal('fetch', fetchMock);

    const begun = await beginReportRun({
      uiRunRequestId: 'request-1',
      conversationId: 'conv-1',
      turnId: 'turn-7',
      windowId: 'window-3',
      origin: 'prompt',
      builderRef: 'enhanced-builder',
      presetId: 'inventory-brief',
      sourceKind: 'preset',
      sourceId: 'inventory-brief',
      requestedParams: { region: ['central'] },
      effectiveParams: { region: ['central'], limit: 10 },
    });
    expect(begun).toMatchObject({
      enabled: true,
      run: { reportRunId: 'run-server-1', revision: 1 },
    });

    await completeReportRun({
      reportRunId: 'run-server-1',
      conversationId: 'conv-1',
      turnId: 'turn-7',
      windowId: 'window-3',
      expectedRevision: 1,
      reportSpec: { kind: 'reportSpec', version: 1 },
      reportFill: { kind: 'reportFill', version: 1 },
      reportPrint: { kind: 'reportPrint', version: 1 },
    });
    await failReportRun({
      reportRunId: 'run-server-1',
      conversationId: 'conv-1',
      turnId: 'turn-7',
      windowId: 'window-3',
      expectedRevision: 2,
      failureCode: 'browser_run_failed',
      failureText: 'Datasource request failed.',
    });
    await activateReportRun({
      reportRunId: 'run-server-1',
      conversationId: 'conv-1',
      turnId: 'turn-7',
      windowId: 'window-3',
      expectedRunRevision: 3,
      expectedContextRevision: 9,
      source: 'prompt',
    });
    await adoptReportRun({
      reportRunId: 'run-server-1',
      conversationId: 'conv-2',
      turnId: 'turn-8',
      windowId: 'window-4',
      expectedRunRevision: 3,
      expectedContextRevision: 4,
      source: 'manual',
    });

    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      '/v1/api/report-runs/begin',
      '/v1/api/report-runs/run-server-1/complete',
      '/v1/api/report-runs/run-server-1/fail',
      '/v1/api/report-runs/run-server-1/activate',
      '/v1/api/report-runs/run-server-1/adopt',
    ]);
    expect(fetchMock.mock.calls.map(([, request]) => JSON.parse(request.body))).toEqual([
      {
        conversationId: 'conv-1',
        origin: 'prompt',
        builderRef: 'enhanced-builder',
        presetId: 'inventory-brief',
        sourceKind: 'preset',
        sourceId: 'inventory-brief',
        requestedParams: { region: ['central'] },
        effectiveParams: { region: ['central'], limit: 10 },
        uiRunRequestId: 'request-1',
      },
      {
        reportRunId: 'run-server-1',
        conversationId: 'conv-1',
        expectedRevision: 1,
        reportSpec: { kind: 'reportSpec', version: 1 },
        reportFill: { kind: 'reportFill', version: 1 },
        reportPrint: { kind: 'reportPrint', version: 1 },
      },
      {
        reportRunId: 'run-server-1',
        conversationId: 'conv-1',
        expectedRevision: 2,
        failureCode: 'browser_run_failed',
        failureText: 'Datasource request failed.',
      },
      {
        reportRunId: 'run-server-1',
        conversationId: 'conv-1',
        expectedRunRevision: 3,
        expectedContextRevision: 9,
        source: 'prompt',
      },
      {
        reportRunId: 'run-server-1',
        conversationId: 'conv-2',
        expectedRunRevision: 3,
        expectedContextRevision: 4,
        source: 'manual',
      },
    ]);
  });
});
