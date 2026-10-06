import { client } from './agentlyClient';
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
  'reportAdmissionRef',
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

async function request(path, body, { method = 'POST' } = {}) {
  const normalizedMethod = String(method || 'POST').trim().toUpperCase() || 'POST';
  const response = await fetch(`${RUNS_PATH}${path}`, {
    method: normalizedMethod,
    credentials: 'include',
    headers: {
      Accept: 'application/json',
      ...(normalizedMethod === 'GET' ? {} : { 'Content-Type': 'application/json' }),
    },
    ...(normalizedMethod === 'GET' ? {} : { body: JSON.stringify(body || {}) }),
  });
  return parseResponse(response);
}

// A missing route means the default-closed persistence feature is off. Only
// this case falls back to a legacy run; mounted endpoint failures must surface.
function validateCommandBinding(input, result) {
  const key = '_agentlyForecastCommand';
  if (Object.hasOwn(input.requestedParams || {}, key)) throw new Error('Report command metadata is server-owned.');
  if (input.reportAdmissionRef == null) return {};
  const canonical = value => Array.isArray(value) ? value.map(canonical) : value && typeof value === 'object'
    ? Object.fromEntries(Object.keys(value).sort().map(name => [name, canonical(value[name])])) : value;
  const params = result?.run?.requestedParams;
  const expected = { version: 1, ref: input.reportAdmissionRef, requestId: input.uiRunRequestId };
  if (!params || typeof params !== 'object' || Array.isArray(params)
      || JSON.stringify(canonical(params[key])) !== JSON.stringify(canonical(expected))
      || JSON.stringify(canonical(Object.fromEntries(Object.entries(params).filter(([name]) => name !== key)))) !== JSON.stringify(canonical(input.requestedParams))) {
    throw new Error('The report service did not preserve the admitted command identity and request.');
  }
  return { reportAdmissionRef: input.reportAdmissionRef };
}

export async function beginReportRun(input = {}) {
  if (Object.hasOwn(input.requestedParams || {}, '_agentlyForecastCommand')) throw new Error('Report command metadata is server-owned.');
  try {
    const result = await request('/begin', normalizeRequestBody(input, BEGIN_FIELDS));
    return { enabled: true, ...result, ...validateCommandBinding(input, result) };
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

export function getCompletedReportRun(input = {}) {
  const reportRunId = normalizeId(input.reportRunId);
  const conversationId = normalizeId(input.conversationId);
  if (!reportRunId || !conversationId) throw new Error('reportRunId and conversationId are required');
  return request(`/${encodeURIComponent(reportRunId)}?conversationId=${encodeURIComponent(conversationId)}`, {}, {method: 'GET'});
}

export function getReportRunContext(input = {}) {
  const conversationId = normalizeId(input.conversationId);
  if (!conversationId) throw new Error('conversationId is required');
  return request(
    `/context/${encodeURIComponent(conversationId)}`,
    undefined,
    { method: 'GET' },
  ).then((context) => ({ enabled: true, context })).catch((error) => {
    if (error?.status === 404 && typeof error?.payload === 'string') {
      return { enabled: false, context: null };
    }
    // Core deliberately uses the same scoped JSON 404 for an absent context
    // and an inaccessible conversation. Adoption performs the authoritative
    // exact conversation/owner check before any mutation.
    if (error?.status === 404 && error?.payload && typeof error.payload === 'object') {
      return { enabled: true, context: null };
    }
    throw error;
  });
}

export function adoptReportRun(input = {}) {
  const reportRunId = normalizeId(input.reportRunId);
  if (!reportRunId) throw new Error('reportRunId is required');
  return request(
    `/${encodeURIComponent(reportRunId)}/adopt`,
    normalizeRequestBody(input, ACTIVATE_FIELDS, reportRunId),
  ).catch((error) => {
    // The default-closed adopt route is unmounted as a plain 404. A mounted
    // route always returns structured JSON, including scoped not-found errors.
    if (error?.status === 404 && typeof error?.payload === 'string') {
      return { enabled: false };
    }
    throw error;
  });
}


// Linked report runs require the server compiler's artifact proof.
export async function compileReportRun({ conversationId, ...input } = {}) {
  if (typeof conversationId !== 'string' || !conversationId.trim()
      || typeof input.reportAdmissionRef !== 'string' || !input.reportAdmissionRef.trim()
      || typeof input.reportId !== 'string' || !input.reportId) {
    throw new Error('Linked report compilation requires exact conversation, request and admission identities.');
  }
  const raw = await client.executeTool('reporting:compile_fenced_report', input, { conversationId });
  const result = typeof raw === 'string' ? JSON.parse(raw) : raw;
  if (!result || typeof result !== 'object' || !result.reportSpec || !result.reportFill || !result.reportPrint) {
    throw new Error('The report compiler omitted authoritative artifacts.');
  }
  return result;
}
