import { client } from './agentlyClient';
import { sdkBaseURL } from '../endpoint';

const EXPORT_REQUEST_HEADER = 'X-Agently-Export-Request-ID';
const TRANSIENT_TOOL_STATUSES = new Set([408, 425, 429, 500, 502, 503, 504]);

function normalizeToolResult(raw) {
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

function decodeBase64Bytes(value = '') {
  const source = String(value || '');
  if (!source) return new Uint8Array();
  try {
    if (typeof globalThis.atob === 'function') {
      const decoded = globalThis.atob(source);
      const bytes = new Uint8Array(decoded.length);
      for (let index = 0; index < decoded.length; index += 1) {
        bytes[index] = decoded.charCodeAt(index) & 0xff;
      }
      return bytes;
    }
    return Uint8Array.from(Buffer.from(source, 'base64'));
  } catch (error) {
    throw new Error(`invalid report export artifact data: ${String(error?.message || error || '')}`.trim());
  }
}

function normalizeByteArray(value) {
  if (!Array.isArray(value)) {
    return null;
  }
  const normalized = value.map((entry) => Number(entry));
  const valid = normalized.every((entry) => Number.isInteger(entry) && entry >= 0 && entry <= 255);
  if (!valid) {
    throw new Error('invalid report export artifact bytes');
  }
  return Uint8Array.from(normalized);
}

function newExportRequestId() {
  if (typeof globalThis.crypto?.randomUUID === 'function') {
    return globalThis.crypto.randomUUID();
  }
  if (typeof globalThis.crypto?.getRandomValues === 'function') {
    const bytes = new Uint8Array(16);
    globalThis.crypto.getRandomValues(bytes);
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    const hex = Array.from(bytes, (entry) => entry.toString(16).padStart(2, '0')).join('');
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
  }
  return `export-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

async function decodeDirectToolResponse(response) {
  const payload = await response.json().catch(() => null);
  if (!response.ok) {
    const message = String(payload?.error || payload?.message || `reporting tool request failed (${response.status})`).trim();
    const error = new Error(message);
    error.status = response.status;
    error.responseEnvelope = payload;
    if (payload && typeof payload === 'object' && !Array.isArray(payload)
      && Object.prototype.hasOwnProperty.call(payload, 'result')) {
      error.toolResult = normalizeToolResult(payload.result);
    }
    throw error;
  }
  return normalizeToolResult(payload?.result);
}

async function executeDirectReportingTool(name, args, {
  conversationId = '',
  exportRequestId = '',
  attempts = 1,
} = {}) {
  const normalizedConversationId = String(conversationId || '').trim();
  const normalizedRequestId = String(exportRequestId || '').trim();
  const query = normalizedConversationId
    ? `?conversationId=${encodeURIComponent(normalizedConversationId)}`
    : '';
  const url = `${sdkBaseURL}/tools/${encodeURIComponent(name)}/execute${query}`;
  const maxAttempts = Math.max(1, Number(attempts) || 1);
  let lastError = null;
  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    try {
      const response = await fetch(url, {
        method: 'POST',
        credentials: 'include',
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json',
          ...(normalizedRequestId ? { [EXPORT_REQUEST_HEADER]: normalizedRequestId } : {}),
        },
        body: JSON.stringify(args || {}),
      });
      if (TRANSIENT_TOOL_STATUSES.has(response.status) && attempt + 1 < maxAttempts) {
        continue;
      }
      return await decodeDirectToolResponse(response);
    } catch (error) {
      lastError = error;
      const status = Number(error?.status || 0);
      if (attempt + 1 >= maxAttempts || (status > 0 && !TRANSIENT_TOOL_STATUSES.has(status))) {
        throw error;
      }
    }
  }
  throw lastError || new Error('reporting tool request failed');
}

export async function submitReportExportRequest({ request, source = '' } = {}) {
  if (!request || typeof request !== 'object' || Array.isArray(request)) {
    throw new Error('report export request is required');
  }
  const result = normalizeToolResult(await client.executeTool('reporting:submit_export', {
    reportExportRequest: request,
  }));
  const normalizedSource = String(source || '').trim();
  if (result == null) {
    return {
      ok: true,
      ...(normalizedSource ? { source: normalizedSource } : {}),
    };
  }
  if (typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected reporting export response: ${JSON.stringify(result)}`);
  }
  return {
    ...result,
    ok: true,
    ...(normalizedSource ? { source: normalizedSource } : {}),
  };
}

export async function submitReportExportSource({
  source,
  format = 'pdf',
  conversationId = '',
  workspaceId = '',
} = {}) {
  if (!source || typeof source !== 'object' || Array.isArray(source)) {
    throw new Error('report export source is required');
  }
  const result = normalizeToolResult(await client.executeTool('reporting:submit_export', {
    source,
    format: String(format || 'pdf').trim().toLowerCase(),
    ...(String(conversationId || '').trim() ? { conversationId: String(conversationId).trim() } : {}),
    ...(String(workspaceId || '').trim() ? { workspaceId: String(workspaceId).trim() } : {}),
  }));
  if (!result || typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected reporting export response: ${JSON.stringify(result)}`);
  }
  return { ...result, ok: true };
}

export async function submitReportExportRun({
  reportRunId,
  format = 'pdf',
  conversationId = '',
  source = '',
} = {}) {
  const normalizedRunId = String(reportRunId || '').trim();
  const normalizedFormat = String(format || '').trim().toLowerCase();
  if (!normalizedRunId) {
    throw new Error('report export reportRunId is required');
  }
  if (normalizedFormat !== 'pdf') {
    throw new Error('report export run-reference mode supports pdf only');
  }
  const exportRequestId = newExportRequestId();
  const result = await executeDirectReportingTool('reporting:submit_export', {
    reportRunId: normalizedRunId,
    format: normalizedFormat,
  }, {
    conversationId,
    exportRequestId,
    attempts: 3,
  });
  if (!result || typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected reporting export response: ${JSON.stringify(result)}`);
  }
  const normalizedSource = String(source || '').trim();
  return {
    ...result,
    ok: true,
    ...(normalizedSource ? { source: normalizedSource } : {}),
  };
}

export async function getReportExportStatus({ jobId, conversationId = '' } = {}) {
  const normalizedJobId = String(jobId || '').trim();
  if (!normalizedJobId) {
    throw new Error('report export jobId is required');
  }
  const args = { jobId: normalizedJobId };
  const result = String(conversationId || '').trim()
    ? await executeDirectReportingTool('reporting:get_export_status', args, { conversationId })
    : normalizeToolResult(await client.executeTool('reporting:get_export_status', args));
  if (!result || typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected reporting export status response: ${JSON.stringify(result)}`);
  }
  return result;
}

export async function getReportExportArtifact({ artifactId, conversationId = '' } = {}) {
  const normalizedArtifactId = String(artifactId || '').trim();
  if (!normalizedArtifactId) {
    throw new Error('report export artifactId is required');
  }
  const args = {
    artifactId: normalizedArtifactId,
    includeData: true,
  };
  const result = String(conversationId || '').trim()
    ? await executeDirectReportingTool('reporting:get_artifact', args, { conversationId })
    : normalizeToolResult(await client.executeTool('reporting:get_artifact', args));
  if (!result || typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected reporting export artifact response: ${JSON.stringify(result)}`);
  }
  const encodedData = typeof result.data === 'string' ? result.data : '';
  const bytes = result.bytes instanceof Uint8Array
    ? new Uint8Array(result.bytes)
    : result.bytes != null
      ? normalizeByteArray(result.bytes)
      : (encodedData ? decodeBase64Bytes(encodedData) : new Uint8Array());
  return {
    ...result,
    bytes,
  };
}

function normalizeListResponse(result, itemKey) {
  if (!result || typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected reporting export ${itemKey} response: ${JSON.stringify(result)}`);
  }
  const items = result?.[itemKey];
  if (!Array.isArray(items)) {
    throw new Error(`unexpected reporting export ${itemKey} response: ${JSON.stringify(result)}`);
  }
  return {
    ...result,
    totalCount: Number.isFinite(Number(result.totalCount)) ? Number(result.totalCount) : items.length,
  };
}

export async function listReportExportJobs({ artifactRef = '', limit = 0, conversationId = '' } = {}) {
  const args = {
    ...(String(artifactRef || '').trim() ? { artifactRef: String(artifactRef || '').trim() } : {}),
    ...(Number.isFinite(Number(limit)) && Number(limit) > 0 ? { limit: Number(limit) } : {}),
  };
  const result = String(conversationId || '').trim()
    ? await executeDirectReportingTool('reporting:list_export_jobs', args, { conversationId })
    : normalizeToolResult(await client.executeTool('reporting:list_export_jobs', args));
  return normalizeListResponse(result, 'jobs');
}

export async function listReportExportArtifacts({ artifactRef = '', limit = 0, conversationId = '' } = {}) {
  const args = {
    ...(String(artifactRef || '').trim() ? { artifactRef: String(artifactRef || '').trim() } : {}),
    ...(Number.isFinite(Number(limit)) && Number(limit) > 0 ? { limit: Number(limit) } : {}),
  };
  const result = String(conversationId || '').trim()
    ? await executeDirectReportingTool('reporting:list_export_artifacts', args, { conversationId })
    : normalizeToolResult(await client.executeTool('reporting:list_export_artifacts', args));
  return normalizeListResponse(result, 'artifacts');
}
