import { executeReportingTool } from './reportingToolClient';

export async function recordReportAuditEvent({ event } = {}) {
  if (!event || typeof event !== 'object' || Array.isArray(event)) {
    throw new Error('report audit event is required');
  }
  const result = await executeReportingTool('reporting:record_audit_event', {
    event,
  }, 'report audit request failed');
  if (result == null) {
    return { ok: true };
  }
  if (typeof result !== 'object' || Array.isArray(result)) {
    throw new Error(`unexpected reporting audit response: ${JSON.stringify(result)}`);
  }
  return {
    ...result,
    ok: true,
  };
}
