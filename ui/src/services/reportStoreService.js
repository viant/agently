import { executeReportingTool } from './reportingToolClient';

const REPORT_STORE_CHANGED_EVENT = 'forge:report-store-changed';

function notifyReportStoreChanged(detail = {}) {
  if (typeof globalThis?.dispatchEvent !== 'function' || typeof globalThis?.CustomEvent !== 'function') {
    return;
  }
  globalThis.dispatchEvent(new globalThis.CustomEvent(REPORT_STORE_CHANGED_EVENT, {
    detail,
  }));
}

export async function saveReport(request = null) {
  if (!request || typeof request !== 'object' || Array.isArray(request)) {
    throw new Error('save report request is required');
  }
  const result = await executeReportingTool('reporting:save_report', request, 'report save request failed');
  const response = result && typeof result === 'object' && !Array.isArray(result)
    ? { ...result, ok: true }
    : { ok: true };
  notifyReportStoreChanged({ action: 'saved', report: response });
  return response;
}

export async function getReport(request = {}) {
  if (!request || typeof request !== 'object' || Array.isArray(request)) {
    throw new Error('get report request is required');
  }
  const result = await executeReportingTool('reporting:get_report', request, 'report get request failed');
  if (!result || typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected report get response: ${JSON.stringify(result)}`);
  }
  return result;
}

export async function listReports(request = {}) {
  if (request != null && (typeof request !== 'object' || Array.isArray(request))) {
    throw new Error('list reports request must be an object');
  }
  const result = await executeReportingTool('reporting:list_reports', request || {}, 'report list request failed');
  if (!result || typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected report list response: ${JSON.stringify(result)}`);
  }
  return result;
}

export async function updateReport(request = null) {
  if (!request || typeof request !== 'object' || Array.isArray(request)) {
    throw new Error('update report request is required');
  }
  const result = await executeReportingTool('reporting:update_report', request, 'report update request failed');
  if (!result || typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected report update response: ${JSON.stringify(result)}`);
  }
  const response = {
    ...result,
    ok: true,
  };
  notifyReportStoreChanged({ action: 'updated', report: response });
  return response;
}

export async function duplicateReport(request = null) {
  if (!request || typeof request !== 'object' || Array.isArray(request)) {
    throw new Error('duplicate report request is required');
  }
  const result = await executeReportingTool('reporting:duplicate_report', request, 'report duplicate request failed');
  const response = result && typeof result === 'object' && !Array.isArray(result)
    ? { ...result, ok: true }
    : { ok: true };
  notifyReportStoreChanged({ action: 'duplicated', report: response });
  return response;
}

export async function deleteReport(request = null) {
  if (!request || typeof request !== 'object' || Array.isArray(request)) {
    throw new Error('delete report request is required');
  }
  const result = await executeReportingTool('reporting:delete_report', request, 'report delete request failed');
  if (!result || typeof result !== 'object' || Array.isArray(result) || result.deleted !== true) {
    throw new Error(`unexpected report delete response: ${JSON.stringify(result)}`);
  }
  notifyReportStoreChanged({ action: 'deleted', ...result });
  return { ...result, ok: true };
}

export async function recordReportRun(request = null) {
  if (!request || typeof request !== 'object' || Array.isArray(request)) {
    throw new Error('record report run request is required');
  }
  const result = await executeReportingTool('reporting:record_report_run', request, 'report run record request failed');
  if (!result || typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected report run record response: ${JSON.stringify(result)}`);
  }
  notifyReportStoreChanged({ action: 'ran', report: result });
  return result;
}
