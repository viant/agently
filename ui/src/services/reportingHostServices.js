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
import {
  activateReportRun,
  adoptReportRun,
  beginReportRun,
  completeReportRun,
  failReportRun,
} from './reportRunService';

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
