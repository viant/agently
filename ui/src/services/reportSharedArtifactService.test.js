import { describe, expect, it, vi } from 'vitest';

vi.mock('./reportingToolClient', () => ({
  executeReportingTool: vi.fn(async (name, args) => {
    if (name === 'reporting:list_shared_artifacts') {
      return {
        artifacts: [
          { artifactId: 'shared-1', kind: 'reportBuilder.savedView' },
        ],
        totalCount: 1,
      };
    }
    return {
      artifactId: args.artifactId,
      kind: 'reportBuilder.savedView',
    };
  }),
}));

import { executeReportingTool } from './reportingToolClient';
import { getReportSharedArtifact, listReportSharedArtifacts } from './reportSharedArtifactService';

describe('reportSharedArtifactService', () => {
  it('lists shared artifacts through reporting:list_shared_artifacts', async () => {
    const result = await listReportSharedArtifacts({ reportId: 'capacityQ3', limit: 5 });
    expect(executeReportingTool).toHaveBeenCalledWith(
      'reporting:list_shared_artifacts',
      {
        artifactRef: '',
        reportId: 'capacityQ3',
        kind: '',
        lifecycle: '',
        limit: 5,
      },
      'report shared artifact list request failed',
    );
    expect(result).toMatchObject({ totalCount: 1 });
    expect(result.artifacts[0]).toMatchObject({ artifactId: 'shared-1' });
  });

  it('gets a shared artifact through reporting:get_shared_artifact', async () => {
    const result = await getReportSharedArtifact({ artifactId: 'shared-1' });
    expect(executeReportingTool).toHaveBeenCalledWith(
      'reporting:get_shared_artifact',
      { artifactId: 'shared-1' },
      'report shared artifact request failed',
    );
    expect(result).toMatchObject({ artifactId: 'shared-1', kind: 'reportBuilder.savedView' });
  });
});
