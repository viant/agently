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
import {
  getReportSharedArtifact,
  listReportSharedArtifacts,
} from './reportSharedArtifactService';
import { emitReportUIEvent } from './reportEventService';
import { fetchDatasource } from '../components/lookups/client';
import {
  activateReportRun,
  adoptReportRun,
  beginReportRun,
  completeReportRun,
  failReportRun,
} from './reportRunService';

export async function fetchReportBuilderPreviewByRef({
  dataSourceRef = '',
  parameters = {},
} = {}) {
  const normalizedDataSourceRef = String(dataSourceRef || '').trim();
  if (!normalizedDataSourceRef) {
    throw new Error('Report preview requires a data source reference.');
  }
  return fetchDatasource(
    normalizedDataSourceRef,
    parameters && typeof parameters === 'object' && !Array.isArray(parameters)
      ? parameters
      : {},
  );
}

export function createReportingHostServices() {
  return {
    reportExport: {
      submitRequest: submitReportExportRequest,
      submitRun: submitReportExportRun,
      submitSource: submitReportExportSource,
      getStatus: getReportExportStatus,
      getArtifact: getReportExportArtifact,
      listJobs: listReportExportJobs,
      listArtifacts: listReportExportArtifacts,
    },
    reportStore: {
      saveReport,
      getReport,
      listReports,
      updateReport,
      duplicateReport,
      deleteReport,
      recordReportRun,
    },
    reportLifecycle: {
      runAction: runReportLifecycleAction,
      shareArtifact: runReportLifecycleAction,
      transitionArtifact: runReportLifecycleAction,
    },
    reportSharedArtifacts: {
      listArtifacts: listReportSharedArtifacts,
      getArtifact: getReportSharedArtifact,
    },
    reportEvents: {
      emit: emitReportUIEvent,
    },
    reportBuilderPreview: {
      fetchByRef: fetchReportBuilderPreviewByRef,
    },
    reportRuns: {
      begin: beginReportRun,
      complete: completeReportRun,
      fail: failReportRun,
      activate: activateReportRun,
      adopt: adoptReportRun,
    },
  };
}

export const reportingHostServices = createReportingHostServices();
