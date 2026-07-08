import { executeReportingTool } from './reportingToolClient';

export async function saveReport(request = null) {
  if (!request || typeof request !== 'object' || Array.isArray(request)) {
    throw new Error('save report request is required');
  }
  const result = await executeReportingTool('reporting:save_report', request, 'report save request failed');
  return result && typeof result === 'object' && !Array.isArray(result)
    ? { ...result, ok: true }
    : { ok: true };
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
  return {
    ...result,
    ok: true,
  };
}
