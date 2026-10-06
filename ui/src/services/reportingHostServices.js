import {getAuthMeSilently, getReportRestoreScope} from './agentlyClient';
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
  compileReportRun,
  failReportRun,
  getReportRunContext,
  getCompletedReportRun,
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
  signal = null,
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
    ...(signal ? [{signal}] : []),
  );
}

export function createCompletedReportRestoreReader({
  probeIdentity = getAuthMeSilently, getScope = getReportRestoreScope,
  readContext = getReportRunContext, readRun = getCompletedReportRun,
} = {}) {
  return async ({conversationId = ''} = {}) => {
    if (!normalizeText(conversationId)) return null;
    await probeIdentity();
    const scopeKey = getScope();
    const selected = await readContext({conversationId});
    if (scopeKey !== getScope()) throw new Error('Report restoration account changed.');
    if (!selected?.enabled || !selected.context?.activeReportRunId) return null;
    if (selected.context.conversationId !== conversationId || !selected.context.ownerId) throw new Error('Report restoration context is not scoped to this conversation.');
    const run = await readRun({conversationId, reportRunId: selected.context.activeReportRunId});
    if (scopeKey !== getScope()) throw new Error('Report restoration account changed.');
    return {context: selected.context, run, scopeKey};
  };
}
export function readCompletedReportRestore(input = {}) {
  return createCompletedReportRestoreReader()(input);
}

function subscribeReportRestoreScope(listener) {
  if (typeof window === 'undefined') return () => {};
  const changed = () => listener(getReportRestoreScope());
  const events = ['agently:session-reset', 'agently:logout', 'agently:authorized'];
  events.forEach(event => window.addEventListener(event, changed));
  return () => events.forEach(event => window.removeEventListener(event, changed));
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
      readCompletedRestore: readCompletedReportRestore,
      getRestoreScope: () => getReportRestoreScope(),
      subscribeRestoreScope: subscribeReportRestoreScope,
      begin: beginReportRun,
      complete: completeReportRun,
      compile: compileReportRun,
      fail: failReportRun,
      activate: activateReportRun,
      getContext: getReportRunContext,
      getRun: getCompletedReportRun,
      adopt: adoptReportRun,
    },
  };
}

export const reportingHostServices = createReportingHostServices();
