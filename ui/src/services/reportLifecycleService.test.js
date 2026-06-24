import { describe, expect, it, vi } from 'vitest';

vi.mock('./reportingToolClient', () => ({
  executeReportingTool: vi.fn(async (name, args) => ({
    tool: name,
    args,
  })),
}));

import { executeReportingTool } from './reportingToolClient';
import { runReportLifecycleAction } from './reportLifecycleService';

describe('reportLifecycleService', () => {
  it('routes share actions to reporting:share_artifact', async () => {
    const request = {
      action: 'share',
      artifactRef: 'reportBuilder.savedReportPayload://rbreport_capacity_q3',
      version: 4,
      reportDocument: { kind: 'reportDocument', id: 'capacityQ3' },
      reportExportRequest: { kind: 'reportExportRequest' },
    };
    const result = await runReportLifecycleAction(request);
    expect(executeReportingTool).toHaveBeenCalledWith(
      'reporting:share_artifact',
      request,
      'report lifecycle share request failed',
    );
    expect(result).toMatchObject({ ok: true, tool: 'reporting:share_artifact' });
  });

  it('routes publish actions to reporting:transition_artifact', async () => {
    const request = {
      action: 'publish',
      artifactRef: 'reportBuilder.savedReportPayload://rbreport_capacity_q3',
      lifecycle: 'draft',
      version: 4,
      reportDocument: { kind: 'reportDocument', id: 'capacityQ3' },
      reportExportRequest: { kind: 'reportExportRequest' },
      transition: {
        artifactRef: 'reportBuilder.savedReportPayload://rbreport_capacity_q3',
        from: 'draft',
        to: 'published',
      },
    };
    const result = await runReportLifecycleAction(request);
    expect(executeReportingTool).toHaveBeenCalledWith(
      'reporting:transition_artifact',
      expect.objectContaining({
        artifactRef: 'reportBuilder.savedReportPayload://rbreport_capacity_q3',
        from: 'draft',
        to: 'published',
        version: 4,
      }),
      'report lifecycle transition request failed',
    );
    expect(result).toMatchObject({ ok: true, tool: 'reporting:transition_artifact' });
  });
});
