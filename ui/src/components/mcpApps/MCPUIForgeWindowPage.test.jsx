import { describe, expect, it } from 'vitest';

import { appendTargetContext, buildWindowPayload, parseJSONParam } from './MCPUIForgeWindowPage.jsx';

describe('MCPUIForgeWindowPage current helpers', () => {
  it('parses optional JSON parameters without throwing', () => {
    expect(parseJSONParam('{"reportId":"r1"}')).toEqual({ reportId: 'r1' });
    expect(parseJSONParam('invalid')).toEqual({});
    expect(parseJSONParam('')).toEqual({});
  });

  it('builds the hosted Forge window identity consumed by WindowContent', () => {
    expect(buildWindowPayload('metricReportBuilder', {
      data: { namespace: 'platform', title: 'Performance Metrics', presentation: 'hosted', region: 'chat.top' },
    }, { conversationId: 'conv-1' })).toMatchObject({
      windowId: 'mcpui:metricReportBuilder',
      windowKey: 'metricReportBuilder',
      id: 'metricReportBuilder',
      stateKey: 'metricReportBuilder',
      windowTitle: 'Performance Metrics',
      presentation: 'hosted',
      region: 'chat.top',
      parameters: { conversationId: 'conv-1' },
      isInTab: true,
    });
  });

  it('shows the authored title without source prefixes while retaining routing identity', () => {
    const key = 'window://platform/deliver/orders';
    expect(buildWindowPayload(key, { data: {
      namespace: 'AI Studio', title: 'Orders', resource: { providerIdentity: 'aistudio' },
    } }, {})).toMatchObject({
      windowKey: key, namespace: 'AI Studio', windowTitle: 'Orders',
      resource: { providerIdentity: 'aistudio' },
    });
  });

  it('uses the original window title or a neutral fallback instead of source or panel labels', () => {
    expect(buildWindowPayload('window://platform/reports', {
      data: { namespace: 'AI Studio', viewMetadata: { title: 'Reports' } },
    }, {}).windowTitle).toBe('Reports');
    expect(buildWindowPayload('window://platform/reports', {
      data: { namespace: 'AI Studio', windowTitle: 'AI Studio: Reports', view: { content: { title: 'Panel' } } },
    }, {}).windowTitle).toBe('Workspace');
  });

  it('adds target context and repeated capabilities to the request query', () => {
    const params = new URLSearchParams();
    appendTargetContext(params, {
      platform: 'web', formFactor: 'desktop', surface: 'browser', capabilities: ['markdown', 'reporting'],
    });
    expect(params.get('platform')).toBe('web');
    expect(params.get('formFactor')).toBe('desktop');
    expect(params.get('surface')).toBe('browser');
    expect(params.getAll('capabilities')).toEqual(['markdown', 'reporting']);
  });
});
