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
import { getProjection, subscribe as subscribeToChatStore } from './chatStore';
import {
  activateReportRun,
  adoptReportRun,
  beginReportRun,
  completeReportRun,
  failReportRun,
  getReportRunContext,
} from './reportRunService';

function normalizeText(value = '') {
  return typeof value === 'string' ? value.trim() : '';
}

export function buildReportProvenanceFromRows(rows = []) {
  const normalizedRows = Array.isArray(rows) ? rows : [];
  const initialUserRow = normalizedRows.find((row) => (
    String(row?.kind || row?.role || '').trim().toLowerCase() === 'user'
    && normalizeText(row?.content || row?.text || row?.message)
  ));
  const events = [];
  const seen = new Set();
  normalizedRows.forEach((row) => {
    const executionGroups = Array.isArray(row?.executionGroups) ? row.executionGroups : [];
    const rounds = Array.isArray(row?.rounds) ? row.rounds : [];
    const toolCollections = [
      ...executionGroups.map((group) => group?.toolSteps),
      ...rounds.map((round) => round?.toolCalls),
    ];
    toolCollections.forEach((collection, groupIndex) => {
      const steps = Array.isArray(collection) ? collection : [];
      steps.forEach((step, stepIndex) => {
        const label = normalizeText(step?.toolName || step?.name);
        if (!label) return;
        const id = normalizeText(step?.toolCallId || step?.toolMessageId)
          || `${normalizeText(row?.turnId || row?.id) || 'turn'}:${groupIndex + 1}:${stepIndex + 1}:${label}`;
        if (seen.has(id)) return;
        seen.add(id);
        events.push({
          id,
          label,
          status: normalizeText(step?.status).toLowerCase() || 'completed',
          ...(normalizeText(step?.startedAt) ? { startedAt: normalizeText(step.startedAt) } : {}),
          ...(normalizeText(step?.completedAt || step?.finishedAt)
            ? { completedAt: normalizeText(step?.completedAt || step?.finishedAt) }
            : {}),
        });
      });
    });
  });
  return {
    initialPrompt: normalizeText(initialUserRow?.content || initialUserRow?.text || initialUserRow?.message),
    events: events.slice(-50),
  };
}

export function getReportBuildProvenance({ conversationId = '' } = {}) {
  const id = normalizeText(conversationId);
  return id ? buildReportProvenanceFromRows(getProjection(id)) : { initialPrompt: '', events: [] };
}

export function subscribeReportBuildProvenance({ conversationId = '' } = {}, listener) {
  const id = normalizeText(conversationId);
  if (!id || typeof listener !== 'function') return () => {};
  listener(buildReportProvenanceFromRows(getProjection(id)));
  return subscribeToChatStore(() => {
    listener(buildReportProvenanceFromRows(getProjection(id)));
  });
}

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
    reportProvenance: {
      getBuildContext: getReportBuildProvenance,
      subscribeBuildContext: subscribeReportBuildProvenance,
    },
    reportBuilderPreview: {
      fetchByRef: fetchReportBuilderPreviewByRef,
    },
    reportRuns: {
      begin: beginReportRun,
      complete: completeReportRun,
      fail: failReportRun,
      activate: activateReportRun,
      getContext: getReportRunContext,
      adopt: adoptReportRun,
    },
  };
}

export const reportingHostServices = createReportingHostServices();
