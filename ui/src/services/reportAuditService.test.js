import { describe, expect, it, vi } from 'vitest';

vi.mock('./reportingToolClient', () => ({
  executeReportingTool: vi.fn(async (_name, args) => ({
    ...args.event,
    recorded: true,
  })),
}));

import { executeReportingTool } from './reportingToolClient';
import { recordReportAuditEvent } from './reportAuditService';

describe('reportAuditService', () => {
  it('submits a structured reporting audit event through reporting:record_audit_event', async () => {
    const event = {
      eventType: 'report.publish',
      artifactRef: 'reportBuilder.savedView://saved_view_capacity_q3',
      version: 8,
      actorRef: 'user://awitas',
      occurredAt: '2026-06-24T12:00:00Z',
      metadata: {
        source: 'reportBuilder',
      },
    };

    const result = await recordReportAuditEvent({ event });

    expect(executeReportingTool).toHaveBeenCalledWith(
      'reporting:record_audit_event',
      { event },
      'report audit request failed',
    );
    expect(result).toMatchObject({
      ok: true,
      recorded: true,
      eventType: 'report.publish',
      actorRef: 'user://awitas',
    });
  });
});
