import { executeReportingTool } from './reportingToolClient';

function normalizeListResult(result) {
  if (result == null) {
    return {
      artifacts: [],
      totalCount: 0,
    };
  }
  if (!result || typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected reporting shared artifact list response: ${JSON.stringify(result)}`);
  }
  if (!Array.isArray(result.artifacts)) {
    throw new Error(`unexpected reporting shared artifact list response: ${JSON.stringify(result)}`);
  }
  return {
    ...result,
    totalCount: Number.isFinite(Number(result.totalCount)) ? Number(result.totalCount) : result.artifacts.length,
  };
}

export async function listReportSharedArtifacts({
  artifactRef = '',
  reportId = '',
  kind = '',
  lifecycle = '',
  limit = 0,
} = {}) {
  const result = await executeReportingTool('reporting:list_shared_artifacts', {
    artifactRef: String(artifactRef || '').trim(),
    reportId: String(reportId || '').trim(),
    kind: String(kind || '').trim(),
    lifecycle: String(lifecycle || '').trim(),
    limit: Number.isFinite(Number(limit)) ? Number(limit) : 0,
  }, 'report shared artifact list request failed');
  return normalizeListResult(result);
}

export async function getReportSharedArtifact({ artifactId } = {}) {
  const normalizedArtifactId = String(artifactId || '').trim();
  if (!normalizedArtifactId) {
    throw new Error('report shared artifact artifactId is required');
  }
  const result = await executeReportingTool('reporting:get_shared_artifact', {
    artifactId: normalizedArtifactId,
  }, 'report shared artifact request failed');
  if (!result || typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected reporting shared artifact response: ${JSON.stringify(result)}`);
  }
  return result;
}
