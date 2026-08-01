import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('./agentlyClient', () => ({
  client: {
    executeTool: vi.fn(),
  },
}));

import { client } from './agentlyClient';
import {
  getReportExportArtifact,
  getReportExportStatus,
  listReportExportArtifacts,
  listReportExportJobs,
  submitReportExportRun,
  submitReportExportRequest,
  submitReportExportSource,
} from './reportExportService';

describe('reportExportService', () => {
  beforeEach(() => {
    client.executeTool.mockReset();
    vi.unstubAllGlobals();
  });

  it('submits exact run references with one trusted header reused across transport retries', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: false,
        status: 503,
        json: vi.fn(async () => ({ error: 'temporary' })),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: vi.fn(async () => ({
          result: JSON.stringify({
            jobId: 'job-run-1',
            reportRunId: 'run-1',
            status: 'queued',
          }),
        })),
      });
    vi.stubGlobal('fetch', fetchMock);

    const result = await submitReportExportRun({
      reportRunId: 'run-1',
      format: 'pdf',
      conversationId: 'conversation-1',
      source: 'draft',
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    const [firstURL, firstRequest] = fetchMock.mock.calls[0];
    const [secondURL, secondRequest] = fetchMock.mock.calls[1];
    expect(firstURL).toBe('/v1/tools/reporting%3Asubmit_export/execute?conversationId=conversation-1');
    expect(secondURL).toBe(firstURL);
    expect(firstRequest.headers['X-Agently-Export-Request-ID']).toBeTruthy();
    expect(secondRequest.headers['X-Agently-Export-Request-ID']).toBe(
      firstRequest.headers['X-Agently-Export-Request-ID'],
    );
    expect(JSON.parse(firstRequest.body)).toEqual({
      reportRunId: 'run-1',
      format: 'pdf',
    });
    expect(firstRequest.body).not.toContain('exportRequestId');
    expect(firstRequest.body).not.toContain('reportRunRevision');
    expect(result).toMatchObject({
      ok: true,
      source: 'draft',
      jobId: 'job-run-1',
    });
    expect(result).not.toHaveProperty('reportRunRevision');
  });

  it('preserves compact JSON and string partial results on direct tool errors', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({
        ok: false,
        status: 409,
        json: async () => ({
          error: 'reporting: already exists',
          result: JSON.stringify({
            jobId: 'job-failed-1',
            status: 'failed',
            error: 'Artifact already exists.',
          }),
        }),
      })
      .mockResolvedValueOnce({
        ok: false,
        status: 409,
        json: async () => ({
          error: 'reporting: renderer failed',
          result: 'renderer stderr was unavailable',
        }),
      });
    vi.stubGlobal('fetch', fetchMock);

    await expect(getReportExportStatus({
      jobId: 'job-failed-1',
      conversationId: 'conversation-1',
    })).rejects.toMatchObject({
      message: 'reporting: already exists',
      status: 409,
      responseEnvelope: {
        error: 'reporting: already exists',
        result: JSON.stringify({
          jobId: 'job-failed-1',
          status: 'failed',
          error: 'Artifact already exists.',
        }),
      },
      toolResult: {
        jobId: 'job-failed-1',
        status: 'failed',
        error: 'Artifact already exists.',
      },
    });
    await expect(getReportExportStatus({
      jobId: 'job-failed-2',
      conversationId: 'conversation-1',
    })).rejects.toMatchObject({
      message: 'reporting: renderer failed',
      status: 409,
      responseEnvelope: {
        error: 'reporting: renderer failed',
        result: 'renderer stderr was unavailable',
      },
      toolResult: 'renderer stderr was unavailable',
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('preserves the final partial result after exhausting transient retries', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: false, status: 503 })
      .mockResolvedValueOnce({ ok: false, status: 503 })
      .mockResolvedValueOnce({
        ok: false,
        status: 503,
        json: async () => ({
          error: 'reporting worker unavailable',
          result: JSON.stringify({
            jobId: 'job-retry-1',
            status: 'failed',
            error: 'Worker remained unavailable.',
          }),
        }),
      });
    vi.stubGlobal('fetch', fetchMock);

    await expect(submitReportExportRun({
      reportRunId: 'run-1',
      format: 'pdf',
      conversationId: 'conversation-1',
    })).rejects.toMatchObject({
      message: 'reporting worker unavailable',
      status: 503,
      responseEnvelope: {
        error: 'reporting worker unavailable',
        result: JSON.stringify({
          jobId: 'job-retry-1',
          status: 'failed',
          error: 'Worker remained unavailable.',
        }),
      },
      toolResult: {
        jobId: 'job-retry-1',
        status: 'failed',
        error: 'Worker remained unavailable.',
      },
    });
    expect(fetchMock).toHaveBeenCalledTimes(3);
    const requestIDs = fetchMock.mock.calls.map(([, request]) => (
      request.headers['X-Agently-Export-Request-ID']
    ));
    expect(new Set(requestIDs).size).toBe(1);
  });

  it('allocates a distinct trusted request identity for each manual submission', async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ result: JSON.stringify({ jobId: `job-${fetchMock.mock.calls.length}` }) }),
    }));
    vi.stubGlobal('fetch', fetchMock);

    await submitReportExportRun({ reportRunId: 'run-1', format: 'pdf', conversationId: 'conversation-1' });
    await submitReportExportRun({ reportRunId: 'run-1', format: 'pdf', conversationId: 'conversation-1' });

    const firstID = fetchMock.mock.calls[0][1].headers['X-Agently-Export-Request-ID'];
    const secondID = fetchMock.mock.calls[1][1].headers['X-Agently-Export-Request-ID'];
    expect(firstID).toBeTruthy();
    expect(secondID).toBeTruthy();
    expect(secondID).not.toBe(firstID);
  });

  it('restricts exact run-reference export to a reportRunId and PDF', async () => {
    await expect(submitReportExportRun({ reportRunId: '', format: 'pdf' })).rejects.toThrow(
      'report export reportRunId is required',
    );
    await expect(submitReportExportRun({ reportRunId: 'run-1', format: 'xlsx' })).rejects.toThrow(
      'supports pdf only',
    );
  });

  it('submits canonical reportExportRequest through reporting:submit_export', async () => {
    client.executeTool.mockResolvedValue(JSON.stringify({
      jobId: 'job-1',
      status: 'queued',
      artifactRef: 'reportBuilder.savedReportPayload://rbreport_forecasting_q3',
    }));

    const request = {
      version: 1,
      kind: 'reportExportRequest',
      target: { format: 'pdf' },
      source: {
        from: 'savedPayload',
        artifactRef: 'reportBuilder.savedReportPayload://rbreport_forecasting_q3',
        title: 'Forecasting Q3',
      },
      reportSpec: { version: 1, kind: 'reportSpec' },
      reportFill: { version: 1, kind: 'reportFill' },
      reportPrint: { version: 1, kind: 'reportPrint' },
    };

    const result = await submitReportExportRequest({
      request,
      source: 'savedPayload',
    });

    expect(client.executeTool).toHaveBeenCalledWith('reporting:submit_export', {
      reportExportRequest: request,
    });
    expect(result).toMatchObject({
      jobId: 'job-1',
      status: 'queued',
      source: 'savedPayload',
    });
  });

  it('rejects missing requests', async () => {
    await expect(submitReportExportRequest({ request: null })).rejects.toThrow('report export request is required');
    expect(client.executeTool).not.toHaveBeenCalled();
  });

  it('submits a persisted report identity for export', async () => {
    client.executeTool.mockResolvedValue({
      jobId: 'job-saved',
      status: 'queued',
    });

    const result = await submitReportExportSource({
      source: { kind: 'report', reportId: 'saved-report' },
      format: 'pdf',
      conversationId: 'conversation-1',
      workspaceId: 'metricReportBuilder',
    });

    expect(client.executeTool).toHaveBeenCalledWith('reporting:submit_export', {
      source: { kind: 'report', reportId: 'saved-report' },
      format: 'pdf',
      conversationId: 'conversation-1',
      workspaceId: 'metricReportBuilder',
    });
    expect(result).toMatchObject({ ok: true, jobId: 'job-saved' });
  });

  it('passes through object results without requiring JSON strings', async () => {
    client.executeTool.mockResolvedValue({
      jobId: 'job-2',
      status: 'queued',
      ok: false,
    });

    const result = await submitReportExportRequest({
      request: {
        version: 1,
        kind: 'reportExportRequest',
        target: { format: 'pdf' },
        source: { from: 'draft', artifactRef: 'dashboard.reportBuilder://demo', title: 'Demo' },
        reportSpec: { version: 1, kind: 'reportSpec' },
        reportFill: { version: 1, kind: 'reportFill' },
        reportPrint: { version: 1, kind: 'reportPrint' },
      },
    });

    expect(result).toMatchObject({
      ok: true,
      jobId: 'job-2',
      status: 'queued',
    });
  });

  it('treats empty responses as successful no-op acknowledgements', async () => {
    client.executeTool.mockResolvedValue('');

    const result = await submitReportExportRequest({
      request: {
        version: 1,
        kind: 'reportExportRequest',
        target: { format: 'pdf' },
        source: { from: 'draft', artifactRef: 'dashboard.reportBuilder://demo', title: 'Demo' },
        reportSpec: { version: 1, kind: 'reportSpec' },
        reportFill: { version: 1, kind: 'reportFill' },
        reportPrint: { version: 1, kind: 'reportPrint' },
      },
      source: 'draft',
    });

    expect(result).toEqual({
      ok: true,
      source: 'draft',
    });
  });

  it('rejects unexpected non-object tool responses', async () => {
    client.executeTool.mockResolvedValue('Internal server error');

    await expect(submitReportExportRequest({
      request: {
        version: 1,
        kind: 'reportExportRequest',
        target: { format: 'pdf' },
        source: { from: 'draft', artifactRef: 'dashboard.reportBuilder://demo', title: 'Demo' },
        reportSpec: { version: 1, kind: 'reportSpec' },
        reportFill: { version: 1, kind: 'reportFill' },
        reportPrint: { version: 1, kind: 'reportPrint' },
      },
      source: 'draft',
    })).rejects.toThrow('unexpected reporting export response');
  });

  it('loads export status through reporting:get_export_status', async () => {
    client.executeTool.mockResolvedValue(JSON.stringify({
      jobId: 'job-3',
      status: 'queued',
      format: 'pdf',
    }));

    const result = await getReportExportStatus({ jobId: 'job-3' });

    expect(client.executeTool).toHaveBeenCalledWith('reporting:get_export_status', {
      jobId: 'job-3',
    });
    expect(result).toMatchObject({
      jobId: 'job-3',
      status: 'queued',
      format: 'pdf',
    });
  });

  it('uses authenticated conversation context for compact status and history responses', async () => {
    const compactResults = [
      { jobId: 'job-browser-1', status: 'queued' },
      { jobs: [{ jobId: 'job-browser-1' }], totalCount: 1 },
      { artifacts: [{ artifactId: 'artifact-browser-1' }], totalCount: 1 },
    ];
    const fetchMock = vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ result: JSON.stringify(compactResults.shift()) }),
    }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(getReportExportStatus({
      jobId: 'job-browser-1',
      conversationId: 'conversation-1',
    })).resolves.toMatchObject({ jobId: 'job-browser-1', status: 'queued' });
    await expect(listReportExportJobs({
      artifactRef: 'report://demo',
      limit: 2,
      conversationId: 'conversation-1',
    })).resolves.toMatchObject({ totalCount: 1 });
    await expect(listReportExportArtifacts({
      artifactRef: 'report://demo',
      limit: 2,
      conversationId: 'conversation-1',
    })).resolves.toMatchObject({ totalCount: 1 });

    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      '/v1/tools/reporting%3Aget_export_status/execute?conversationId=conversation-1',
      '/v1/tools/reporting%3Alist_export_jobs/execute?conversationId=conversation-1',
      '/v1/tools/reporting%3Alist_export_artifacts/execute?conversationId=conversation-1',
    ]);
  });

  it('loads export artifacts and decodes base64 data', async () => {
    client.executeTool.mockResolvedValue(JSON.stringify({
      artifactId: 'artifact-1',
      contentType: 'application/pdf',
      data: 'JVBERg==',
    }));

    const result = await getReportExportArtifact({ artifactId: 'artifact-1' });

    expect(client.executeTool).toHaveBeenCalledWith('reporting:get_artifact', {
      artifactId: 'artifact-1',
      includeData: true,
    });
    expect(result).toMatchObject({
      artifactId: 'artifact-1',
      contentType: 'application/pdf',
      data: 'JVBERg==',
    });
    expect(Array.from(result.bytes)).toEqual([37, 80, 68, 70]);
  });

  it('carries browser conversation context and requests bytes from compact artifact responses', async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({
        result: JSON.stringify({
          artifactId: 'artifact-browser-1',
          data: 'JVBERg==',
        }),
      }),
    }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getReportExportArtifact({
      artifactId: 'artifact-browser-1',
      conversationId: 'conversation-1',
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toBe(
      '/v1/tools/reporting%3Aget_artifact/execute?conversationId=conversation-1',
    );
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      artifactId: 'artifact-browser-1',
      includeData: true,
    });
    expect(Array.from(result.bytes)).toEqual([37, 80, 68, 70]);
  });

  it('preserves byte-array artifact payloads without requiring base64 data', async () => {
    client.executeTool.mockResolvedValue({
      artifactId: 'artifact-2',
      contentType: 'application/pdf',
      bytes: [37, 80, 68, 70],
    });

    const result = await getReportExportArtifact({ artifactId: 'artifact-2' });

    expect(client.executeTool).toHaveBeenCalledWith('reporting:get_artifact', {
      artifactId: 'artifact-2',
      includeData: true,
    });
    expect(result).toMatchObject({
      artifactId: 'artifact-2',
      contentType: 'application/pdf',
    });
    expect(Array.from(result.bytes)).toEqual([37, 80, 68, 70]);
  });

  it('rejects malformed byte arrays instead of silently zero-coercing them', async () => {
    client.executeTool.mockResolvedValue({
      artifactId: 'artifact-2b',
      contentType: 'application/pdf',
      bytes: [37, null, 'bad'],
    });

    await expect(getReportExportArtifact({ artifactId: 'artifact-2b' })).rejects.toThrow(
      'invalid report export artifact bytes',
    );
  });

  it('preserves Uint8Array artifact payloads without overwriting them', async () => {
    client.executeTool.mockResolvedValue({
      artifactId: 'artifact-3',
      contentType: 'application/pdf',
      bytes: new Uint8Array([37, 80, 68, 70]),
    });

    const result = await getReportExportArtifact({ artifactId: 'artifact-3' });

    expect(client.executeTool).toHaveBeenCalledWith('reporting:get_artifact', {
      artifactId: 'artifact-3',
      includeData: true,
    });
    expect(result).toMatchObject({
      artifactId: 'artifact-3',
      contentType: 'application/pdf',
    });
    expect(Array.from(result.bytes)).toEqual([37, 80, 68, 70]);
  });

  it('throws a stable error when artifact base64 data is malformed', async () => {
    client.executeTool.mockResolvedValue(JSON.stringify({
      artifactId: 'artifact-4',
      contentType: 'application/pdf',
      data: '***not-base64***',
    }));

    await expect(getReportExportArtifact({ artifactId: 'artifact-4' })).rejects.toThrow(
      'invalid report export artifact data',
    );
  });
});
