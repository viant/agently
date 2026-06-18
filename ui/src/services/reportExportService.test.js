import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('./agentlyClient', () => ({
  client: {
    executeTool: vi.fn(),
  },
}));

import { client } from './agentlyClient';
import { getReportExportArtifact, getReportExportStatus, submitReportExportRequest } from './reportExportService';

describe('reportExportService', () => {
  beforeEach(() => {
    client.executeTool.mockReset();
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

  it('loads export artifacts and decodes base64 data', async () => {
    client.executeTool.mockResolvedValue(JSON.stringify({
      artifactId: 'artifact-1',
      contentType: 'application/pdf',
      data: 'JVBERg==',
    }));

    const result = await getReportExportArtifact({ artifactId: 'artifact-1' });

    expect(client.executeTool).toHaveBeenCalledWith('reporting:get_artifact', {
      artifactId: 'artifact-1',
    });
    expect(result).toMatchObject({
      artifactId: 'artifact-1',
      contentType: 'application/pdf',
      data: 'JVBERg==',
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
