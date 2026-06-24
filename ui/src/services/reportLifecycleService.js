import { executeReportingTool } from './reportingToolClient';

function normalizeAction(action = '') {
  return String(action || '').trim().toLowerCase();
}

export async function runReportLifecycleAction(request = null) {
  if (!request || typeof request !== 'object' || Array.isArray(request)) {
    throw new Error('report lifecycle request is required');
  }
  const action = normalizeAction(request.action);
  if (action === 'share') {
    const result = await executeReportingTool('reporting:share_artifact', request, 'report lifecycle share request failed');
    return result && typeof result === 'object' && !Array.isArray(result)
      ? { ...result, ok: true }
      : { ok: true };
  }
  if (action === 'publish' || action === 'archive') {
    const payload = {
      artifactRef: request.artifactRef,
      from: request.transition?.from || request.lifecycle,
      to: request.transition?.to || action,
      reason: request.transition?.reason || '',
      version: request.version,
      reportDocument: request.reportDocument,
      reportExportRequest: request.reportExportRequest,
      metadata: request.metadata,
    };
    const result = await executeReportingTool('reporting:transition_artifact', payload, 'report lifecycle transition request failed');
    return result && typeof result === 'object' && !Array.isArray(result)
      ? { ...result, ok: true }
      : { ok: true };
  }
  throw new Error(`unsupported report lifecycle action ${JSON.stringify(action)}`);
}
