import { client } from './agentlyClient';

export function normalizeToolResult(raw) {
  if (raw == null) return null;
  if (typeof raw === 'string') {
    const text = raw.trim();
    if (!text) return null;
    try {
      return JSON.parse(text);
    } catch (_) {
      return text;
    }
  }
  return raw;
}

export function wrapToolError(error, fallbackMessage = '') {
  if (!error || typeof error !== 'object') {
    return error;
  }
  const body = typeof error.body === 'string' ? error.body.trim() : '';
  if (!body) {
    return error;
  }
  let envelope = null;
  try {
    envelope = JSON.parse(body);
  } catch (_) {
    return error;
  }
  if (!envelope || typeof envelope !== 'object' || Array.isArray(envelope)) {
    return error;
  }
  const message = typeof envelope.error === 'string' && envelope.error.trim()
    ? envelope.error.trim()
    : String(fallbackMessage || error.message || 'reporting tool request failed').trim();
  const wrapped = new Error(message || 'reporting tool request failed');
  wrapped.name = typeof error.name === 'string' && error.name.trim() ? error.name : 'Error';
  if ('status' in error) wrapped.status = error.status;
  if ('statusText' in error) wrapped.statusText = error.statusText;
  wrapped.body = body;
  wrapped.responseEnvelope = envelope;
  if (Object.prototype.hasOwnProperty.call(envelope, 'result')) {
    wrapped.toolResult = normalizeToolResult(envelope.result);
  }
  wrapped.cause = error;
  return wrapped;
}

export async function executeReportingTool(name, args, fallbackMessage = '') {
  try {
    return normalizeToolResult(await client.executeTool(name, args));
  } catch (error) {
    throw wrapToolError(error, fallbackMessage);
  }
}
