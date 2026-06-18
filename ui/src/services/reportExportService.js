import { client } from './agentlyClient';

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

export async function getReportExportStatus({ jobId } = {}) {
  const normalizedJobId = String(jobId || '').trim();
  if (!normalizedJobId) {
    throw new Error('report export jobId is required');
  }
  const result = normalizeToolResult(await client.executeTool('reporting:get_export_status', {
    jobId: normalizedJobId,
  }));
  if (!result || typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected reporting export status response: ${JSON.stringify(result)}`);
  }
  return result;
}

export async function getReportExportArtifact({ artifactId } = {}) {
  const normalizedArtifactId = String(artifactId || '').trim();
  if (!normalizedArtifactId) {
    throw new Error('report export artifactId is required');
  }
  const result = normalizeToolResult(await client.executeTool('reporting:get_artifact', {
    artifactId: normalizedArtifactId,
  }));
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
