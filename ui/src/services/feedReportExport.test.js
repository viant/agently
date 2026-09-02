import { describe, expect, it } from 'vitest';
import { buildFeedExportTitle, buildFeedReportRequest } from './feedReportExport';

describe('feed report export', () => {
  it('builds one backend Forge UI export request with referenced view and staged overrides', () => {
    const request = buildFeedReportRequest({
      feedId: 'catalog',
      conversationId: 'conversation-1',
      title: 'Catalog',
      ui: { title: 'Catalog', containers: [{ id: 'rows' }] },
      dataMap: {
        summary: { count: 2 },
        rows: [{ id: 1 }, { id: 2 }],
      },
      target: { formFactor: 'tablet' },
    });
    expect(request.viewRef).toBe('feed://catalog');
    expect(request.ui.containers[0].id).toBe('rows');
    expect(request.dataSourceRefs).toEqual(['rows', 'summary']);
    expect(request.dataSourceOverrides.rows.collection).toHaveLength(2);
    expect(request.target).toEqual({ platform: 'web', formFactor: 'tablet', surface: 'browser' });
  });
});

describe('feed report export title', () => {
  it('uses the feed entity label for the PDF title and download name', () => {
    expect(buildFeedExportTitle({ title: 'Media Plan', entityLabel: 'CoinPoker' }))
      .toBe('CoinPoker Media Plan');
    expect(buildFeedExportTitle({ title: 'Media Plan', entityLabel: '' }))
      .toBe('Media Plan');
  });
});
