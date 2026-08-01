import { sdkBaseURL } from '../endpoint';

const RUNS_PATH = `${String(sdkBaseURL || '').replace(/\/+$/, '')}/api/report-runs`;

const BEGIN_FIELDS = [
  'conversationId',
  'origin',
  'builderRef',
  'presetId',
  'sourceKind',
  'sourceId',
  'requestedParams',
  'effectiveParams',
  'uiRunRequestId',
];
const COMPLETE_FIELDS = [
  'reportRunId',
  'conversationId',
  'expectedRevision',
  'reportSpec',
  'reportFill',
  'reportPrint',
];
const FAIL_FIELDS = [
  'reportRunId',
  'conversationId',
  'expectedRevision',
  'failureCode',
  'failureText',
];
const ACTIVATE_FIELDS = [
  'reportRunId',
  'conversationId',
  'expectedRunRevision',
  'expectedContextRevision',
  'source',
];

function normalizeId(value = '') {
  return String(value || '').trim();
}

function normalizeRequestBody(input, fields, reportRunId = '') {
  const source = input && typeof input === 'object' && !Array.isArray(input) ? input : {};
  const result = {};
  fields.forEach((field) => {
    if (Object.prototype.hasOwnProperty.call(source, field) && source[field] !== undefined) {
      result[field] = source[field];
    }
  });
  if (reportRunId) {
    result.reportRunId = reportRunId;
  }
  return result;
}

async function parseResponse(response) {
  const text = await response.text();
  let payload = null;
  if (text.trim()) {
    try {
      payload = JSON.parse(text);
    } catch (_) {
      payload = text;
    }
  }
  if (!response.ok) {
    const message = typeof payload?.error === 'string'
      ? payload.error
      : (typeof payload === 'string' && payload.trim() ? payload.trim() : `report run request failed (${response.status})`);
    const error = new Error(message);
    error.status = response.status;
    error.payload = payload;
    throw error;
  }
  return payload;
}

async function request(path, body) {
  const response = await fetch(`${RUNS_PATH}${path}`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body || {}),
  });
  return parseResponse(response);
}

// A missing route means the default-closed persistence feature is off. Only
// this case falls back to a legacy run; mounted endpoint failures must surface.
export async function beginReportRun(input = {}) {
  try {
    const result = await request('/begin', normalizeRequestBody(input, BEGIN_FIELDS));
    return { enabled: true, ...result };
  } catch (error) {
    // The server's unmounted route is a plain 404. Mounted lifecycle errors
    // are structured JSON, including scoped not-found responses.
    if (error?.status === 404 && typeof error?.payload === 'string') {
      return { enabled: false };
    }
    throw error;
  }
}

export function completeReportRun(input = {}) {
  const reportRunId = normalizeId(input.reportRunId);
  if (!reportRunId) throw new Error('reportRunId is required');
  return request(
    `/${encodeURIComponent(reportRunId)}/complete`,
    normalizeRequestBody(input, COMPLETE_FIELDS, reportRunId),
  );
}

export function failReportRun(input = {}) {
  const reportRunId = normalizeId(input.reportRunId);
  if (!reportRunId) throw new Error('reportRunId is required');
  return request(
    `/${encodeURIComponent(reportRunId)}/fail`,
    normalizeRequestBody(input, FAIL_FIELDS, reportRunId),
  );
}

export function activateReportRun(input = {}) {
  const reportRunId = normalizeId(input.reportRunId);
  if (!reportRunId) throw new Error('reportRunId is required');
  return request(
    `/${encodeURIComponent(reportRunId)}/activate`,
    normalizeRequestBody(input, ACTIVATE_FIELDS, reportRunId),
  );
}

export function adoptReportRun(input = {}) {
  const reportRunId = normalizeId(input.reportRunId);
  if (!reportRunId) throw new Error('reportRunId is required');
  return request(
    `/${encodeURIComponent(reportRunId)}/adopt`,
    normalizeRequestBody(input, ACTIVATE_FIELDS, reportRunId),
  );
}
