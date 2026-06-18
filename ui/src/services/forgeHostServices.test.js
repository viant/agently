import { describe, expect, it, vi } from 'vitest';

vi.mock('./chatService', () => ({
  chatService: {
    explorerRead: vi.fn(),
  },
}));

vi.mock('./scheduleService', () => ({
  scheduleService: {
    saveSchedule: vi.fn(),
  },
}));

vi.mock('./datasourceRequestContext', () => ({
  prepareAgentlyDataConnectorRequest: vi.fn(() => ({ ok: true })),
}));

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
    bytes: new Uint8Array([1, 2, 3]),
  })),
}));

import { chatService } from './chatService';
import { scheduleService } from './scheduleService';
import { prepareAgentlyDataConnectorRequest } from './datasourceRequestContext';
import { getReportExportArtifact, getReportExportStatus, submitReportExportRequest } from './reportExportService';
import { forgeHostServices } from './forgeHostServices';

describe('forgeHostServices', () => {
  it('exposes the hosted Forge service bundle including reportExport', async () => {
    expect(forgeHostServices.chat).toBe(chatService);
    expect(forgeHostServices.schedule).toBe(scheduleService);
    expect(forgeHostServices.prepareDataConnectorRequest).toBe(prepareAgentlyDataConnectorRequest);
    expect(typeof forgeHostServices.reportExport.submitRequest).toBe('function');
    expect(typeof forgeHostServices.reportExport.getStatus).toBe('function');
    expect(typeof forgeHostServices.reportExport.getArtifact).toBe('function');

    const request = {
      version: 1,
      kind: 'reportExportRequest',
      target: { format: 'pdf' },
      source: { from: 'draft', title: 'Demo Report' },
    };

    const result = await forgeHostServices.reportExport.submitRequest({
      request,
      source: 'draft',
    });

    expect(submitReportExportRequest).toHaveBeenCalledWith({ request, source: 'draft' });
    expect(result).toMatchObject({ ok: true, source: 'draft', title: 'Demo Report' });

    const status = await forgeHostServices.reportExport.getStatus({ jobId: 'job-1' });
    expect(getReportExportStatus).toHaveBeenCalledWith({ jobId: 'job-1' });
    expect(status).toMatchObject({ jobId: 'job-1', status: 'queued' });

    const artifact = await forgeHostServices.reportExport.getArtifact({ artifactId: 'artifact-1' });
    expect(getReportExportArtifact).toHaveBeenCalledWith({ artifactId: 'artifact-1' });
    expect(Array.from(artifact.bytes)).toEqual([1, 2, 3]);
  });
});
