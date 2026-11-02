import {sdkBaseURL} from '../endpoint.js';

const base = `${sdkBaseURL}/api/conversation-audit`;

async function request(path, options = {}) {
  const response = await fetch(`${base}${path}`, {credentials: 'include', ...options});
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new Error(body.error || `Audit request failed (${response.status}).`);
  }
  return response;
}

export const auditCapabilities = signal => request('/capabilities', {signal}).then(r => r.json());
export const auditTurns = (filter, signal) => request(`/turns?${new URLSearchParams(filter)}`, {signal}).then(r => r.json());
const turnPath = row => `/conversations/${encodeURIComponent(row.conversationId)}/turns/${encodeURIComponent(row.id)}`;
export const auditDetail = (row, signal) => request(turnPath(row), {signal}).then(r => r.json());
export const auditPayload = (row, id, signal) => request(`${turnPath(row)}/payloads/${encodeURIComponent(id)}`, {signal}).then(r => r.json());
export const auditCSV = (filter, signal) => request('/export', {
  method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(filter), signal,
}).then(r => r.blob());

export function auditDuration(milliseconds) {
  if (typeof milliseconds !== 'number' || !Number.isFinite(milliseconds) || milliseconds < 0) return 'unknown';
  if (milliseconds < 1000) return `${milliseconds} ms`;
  const hours = Math.floor(milliseconds / 3600000);
  const minutes = Math.floor(milliseconds / 60000) % 60;
  const seconds = (milliseconds % 60000) / 1000;
  return [hours && `${hours} h`, minutes && `${minutes} min`, seconds && `${seconds} s`].filter(Boolean).join(' ');
}

export function auditFilter(values) {
  const filter = {};
  if (values.userId?.trim()) filter.userId = values.userId.trim();
  if (values.conversationId?.trim()) {
    filter.conversationId = values.conversationId.trim();
  } else {
    const from = new Date(values.from), to = new Date(values.to);
    if (!values.from || !values.to || !Number.isFinite(+from) || !Number.isFinite(+to) || to <= from || to - from > 31 * 86400000) {
      throw new Error('Choose a conversation or a date range of at most 31 days.');
    }
    filter.from = from.toISOString(); filter.to = to.toISOString();
  }
  return filter;
}

export function payloadReferences(messages = []) {
  const result = new Map();
  for (const m of messages) {
    for (const [source, fields] of [[m, ['attachmentPayloadId', 'elicitationPayloadId']],
      [m.modelCall, ['requestPayloadId', 'responsePayloadId', 'providerRequestPayloadId', 'providerResponsePayloadId', 'streamPayloadId']],
      [m.messageToolCall, ['requestPayloadId', 'responsePayloadId']]]) {
      for (const key of fields) if (source?.[key]) result.set(source[key], key.replace(/([A-Z])/g, ' $1').toLowerCase());
    }
  }
  return [...result].map(([id, label]) => ({id, label}));
}
