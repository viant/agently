import { describe, expect, it } from 'vitest';
import { buildFeedReportRequest } from './feedReportExport';

describe('feed report export', () => {
  it('builds one backend Forge UI export request with referenced view and staged overrides', () => {
    const request = buildFeedReportRequest({
      feedId: 'catalog',
      conversationId: 'conversation-1',
      title: 'Catalog',
      dataMap: {
        summary: { count: 2 },
        rows: [{ id: 1 }, { id: 2 }],
      },
      target: { formFactor: 'tablet' },
    });
    expect(request.viewRef).toBe('feed://catalog');
    expect(request.dataSourceRefs).toEqual(['rows', 'summary']);
    expect(request.dataSourceOverrides.rows.collection).toHaveLength(2);
    expect(request.target).toEqual({ platform: 'web', formFactor: 'tablet', surface: 'browser' });
  });
});
