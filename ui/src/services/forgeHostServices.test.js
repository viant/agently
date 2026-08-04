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
  submitReportExportRun: vi.fn(async ({ reportRunId, format, conversationId, source }) => ({
    ok: true,
    reportRunId,
    format,
    conversationId,
    source,
  })),
  submitReportExportSource: vi.fn(async ({ source, format }) => ({
    ok: true,
    source,
    format,
    jobId: 'job-source',
  })),
  getReportExportStatus: vi.fn(async ({ jobId }) => ({
    jobId,
    status: 'queued',
  })),
  getReportExportArtifact: vi.fn(async ({ artifactId }) => ({
    artifactId,
    bytes: new Uint8Array([1, 2, 3]),
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

vi.mock('./reportStoreService', () => ({
  saveReport: vi.fn(async (request) => ({ ok: true, artifactId: 'report-1', ...request })),
  getReport: vi.fn(async ({ artifactId }) => ({ artifactId, title: 'Stored Report' })),
  listReports: vi.fn(async ({ limit }) => ({ reports: [{ artifactId: 'report-1', title: 'Stored Report' }], totalCount: limit || 1 })),
  updateReport: vi.fn(async (request) => ({ ok: true, artifactId: request?.artifactId || 'report-1' })),
  duplicateReport: vi.fn(async (request) => ({ ok: true, artifactId: 'report-copy', ...request })),
  deleteReport: vi.fn(async (request) => ({ ok: true, deleted: true, ...request })),
  recordReportRun: vi.fn(async (request) => ({ ok: true, ...request })),
}));

vi.mock('./reportLifecycleService', () => ({
  runReportLifecycleAction: vi.fn(async (request) => ({ ok: true, action: request?.action || 'share' })),
}));

vi.mock('./reportSharedArtifactService', () => ({
  listReportSharedArtifacts: vi.fn(async ({ limit }) => ({ artifacts: [{ artifactId: 'shared-1', kind: 'reportBuilder.savedView' }], totalCount: limit || 1 })),
  getReportSharedArtifact: vi.fn(async ({ artifactId }) => ({ artifactId, kind: 'reportBuilder.savedView' })),
}));

import { chatService } from './chatService';
import { scheduleService } from './scheduleService';
import { prepareAgentlyDataConnectorRequest } from './datasourceRequestContext';
import {
  getReportExportArtifact,
  getReportExportStatus,
  listReportExportArtifacts,
  listReportExportJobs,
  submitReportExportRun,
  submitReportExportRequest,
  submitReportExportSource,
} from './reportExportService';
import {
  deleteReport,
  duplicateReport,
  getReport,
  listReports,
  recordReportRun,
  saveReport,
  updateReport,
} from './reportStoreService';
import { runReportLifecycleAction } from './reportLifecycleService';
import { getReportSharedArtifact, listReportSharedArtifacts } from './reportSharedArtifactService';
import { forgeHostServices } from './forgeHostServices';

describe('forgeHostServices', () => {
  it('exposes the hosted Forge service bundle including reporting services', async () => {
    expect(forgeHostServices.chat).toBe(chatService);
    expect(forgeHostServices.schedule).toBe(scheduleService);
    expect(forgeHostServices.prepareDataConnectorRequest).toBe(prepareAgentlyDataConnectorRequest);
    expect(typeof forgeHostServices.reportExport.submitRequest).toBe('function');
    expect(typeof forgeHostServices.reportExport.submitRun).toBe('function');
    expect(typeof forgeHostServices.reportExport.submitSource).toBe('function');
    expect(typeof forgeHostServices.reportExport.getStatus).toBe('function');
    expect(typeof forgeHostServices.reportExport.getArtifact).toBe('function');
    expect(typeof forgeHostServices.reportExport.listJobs).toBe('function');
    expect(typeof forgeHostServices.reportExport.listArtifacts).toBe('function');
    expect(typeof forgeHostServices.reportStore.saveReport).toBe('function');
    expect(typeof forgeHostServices.reportStore.getReport).toBe('function');
    expect(typeof forgeHostServices.reportStore.listReports).toBe('function');
    expect(typeof forgeHostServices.reportStore.updateReport).toBe('function');
    expect(typeof forgeHostServices.reportStore.duplicateReport).toBe('function');
    expect(typeof forgeHostServices.reportStore.deleteReport).toBe('function');
    expect(typeof forgeHostServices.reportStore.recordReportRun).toBe('function');
    expect(typeof forgeHostServices.reportLifecycle.shareArtifact).toBe('function');
    expect(typeof forgeHostServices.reportLifecycle.transitionArtifact).toBe('function');
    expect(typeof forgeHostServices.reportSharedArtifacts.listArtifacts).toBe('function');
    expect(typeof forgeHostServices.reportSharedArtifacts.getArtifact).toBe('function');
    expect(typeof forgeHostServices.reportBuilderPreview.fetchByRef).toBe('function');
    expect(typeof forgeHostServices.reportRuns.begin).toBe('function');
    expect(typeof forgeHostServices.reportRuns.complete).toBe('function');
    expect(typeof forgeHostServices.reportRuns.fail).toBe('function');
    expect(typeof forgeHostServices.reportRuns.activate).toBe('function');
    expect(typeof forgeHostServices.reportRuns.adopt).toBe('function');

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

    const runResult = await forgeHostServices.reportExport.submitRun({
      reportRunId: 'run-1',
      format: 'pdf',
      conversationId: 'conversation-1',
      source: 'draft',
    });
    expect(submitReportExportRun).toHaveBeenCalledWith({
      reportRunId: 'run-1',
      format: 'pdf',
      conversationId: 'conversation-1',
      source: 'draft',
    });
    expect(runResult).toMatchObject({ ok: true, reportRunId: 'run-1', format: 'pdf' });

    const sourceResult = await forgeHostServices.reportExport.submitSource({
      source: { kind: 'report', reportId: 'demo-report' },
      format: 'pdf',
    });
    expect(submitReportExportSource).toHaveBeenCalledWith({
      source: { kind: 'report', reportId: 'demo-report' },
      format: 'pdf',
    });
    expect(sourceResult).toMatchObject({ ok: true, jobId: 'job-source' });

    const status = await forgeHostServices.reportExport.getStatus({ jobId: 'job-1' });
    expect(getReportExportStatus).toHaveBeenCalledWith({ jobId: 'job-1' });
    expect(status).toMatchObject({ jobId: 'job-1', status: 'queued' });

    const artifact = await forgeHostServices.reportExport.getArtifact({ artifactId: 'artifact-1' });
    expect(getReportExportArtifact).toHaveBeenCalledWith({ artifactId: 'artifact-1' });
    expect(Array.from(artifact.bytes)).toEqual([1, 2, 3]);

    const jobs = await forgeHostServices.reportExport.listJobs({ artifactRef: 'report://demo', limit: 2 });
    expect(listReportExportJobs).toHaveBeenCalledWith({ artifactRef: 'report://demo', limit: 2 });
    expect(jobs).toMatchObject({ totalCount: 2 });

    const artifacts = await forgeHostServices.reportExport.listArtifacts({ artifactRef: 'report://demo', limit: 3 });
    expect(listReportExportArtifacts).toHaveBeenCalledWith({ artifactRef: 'report://demo', limit: 3 });
    expect(artifacts).toMatchObject({ totalCount: 3 });

    const saved = await forgeHostServices.reportStore.saveReport({ reportId: 'demo-report' });
    expect(saveReport).toHaveBeenCalledWith({ reportId: 'demo-report' });
    expect(saved).toMatchObject({ ok: true, artifactId: 'report-1', reportId: 'demo-report' });

    const got = await forgeHostServices.reportStore.getReport({ artifactId: 'report-1' });
    expect(getReport).toHaveBeenCalledWith({ artifactId: 'report-1' });
    expect(got).toMatchObject({ artifactId: 'report-1', title: 'Stored Report' });

    const listed = await forgeHostServices.reportStore.listReports({ limit: 5 });
    expect(listReports).toHaveBeenCalledWith({ limit: 5 });
    expect(listed).toMatchObject({ totalCount: 5 });

    const updated = await forgeHostServices.reportStore.updateReport({ artifactId: 'report-1', title: 'Updated' });
    expect(updateReport).toHaveBeenCalledWith({ artifactId: 'report-1', title: 'Updated' });
    expect(updated).toMatchObject({ ok: true, artifactId: 'report-1' });

    await forgeHostServices.reportStore.duplicateReport({ artifactId: 'report-1' });
    expect(duplicateReport).toHaveBeenCalledWith({ artifactId: 'report-1' });

    await forgeHostServices.reportStore.deleteReport({ artifactId: 'report-copy' });
    expect(deleteReport).toHaveBeenCalledWith({ artifactId: 'report-copy' });

    await forgeHostServices.reportStore.recordReportRun({ artifactId: 'report-1' });
    expect(recordReportRun).toHaveBeenCalledWith({ artifactId: 'report-1' });

    const shared = await forgeHostServices.reportLifecycle.shareArtifact({ action: 'share', artifactRef: 'report://x' });
    expect(runReportLifecycleAction).toHaveBeenCalledWith({ action: 'share', artifactRef: 'report://x' });
    expect(shared).toMatchObject({ ok: true, action: 'share' });

    const transitioned = await forgeHostServices.reportLifecycle.transitionArtifact({ action: 'publish', artifactRef: 'report://x' });
    expect(runReportLifecycleAction).toHaveBeenCalledWith({ action: 'publish', artifactRef: 'report://x' });
    expect(transitioned).toMatchObject({ ok: true, action: 'publish' });

    const sharedList = await forgeHostServices.reportSharedArtifacts.listArtifacts({ limit: 2 });
    expect(listReportSharedArtifacts).toHaveBeenCalledWith({ limit: 2 });
    expect(sharedList).toMatchObject({ totalCount: 2 });

    const sharedItem = await forgeHostServices.reportSharedArtifacts.getArtifact({ artifactId: 'shared-1' });
    expect(getReportSharedArtifact).toHaveBeenCalledWith({ artifactId: 'shared-1' });
    expect(sharedItem).toMatchObject({ artifactId: 'shared-1', kind: 'reportBuilder.savedView' });
  });
});
