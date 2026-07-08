import { describe, expect, it } from 'vitest';

import {
  appendTargetContext,
  buildForgeReportStarterOptions,
  mergeForgeWindowPayloadWithScopedSnapshot,
  restoreScopedForgeWindowSnapshot,
  buildWindowParams,
  fetchForgeWindowPageState,
  buildForgeWindowRequestURL,
  buildWindowPayload,
  parseJSONParam,
  shouldShowForgeReportStarterChooser,
} from './MCPUIForgeWindowPage.jsx';

describe('MCPUIForgeWindowPage helpers', () => {
  it('parses valid window params json and falls back cleanly on invalid input', () => {
    expect(parseJSONParam('{"reportId":"r1","page":2}')).toEqual({
      reportId: 'r1',
      page: 2,
    });
    expect(parseJSONParam('not-json')).toEqual({});
    expect(parseJSONParam('')).toEqual({});
  });

  it('merges a direct conversationId query param into window params for standalone forge-window routes', () => {
    expect(buildWindowParams('{"reportId":"r1"}', 'conv-123')).toEqual({
      reportId: 'r1',
      conversationId: 'conv-123',
    });
    expect(buildWindowParams('', 'conv-xyz')).toEqual({
      conversationId: 'conv-xyz',
    });
    expect(buildWindowParams('{"conversationId":"from-json","reportId":"r1"}', 'conv-123')).toEqual({
      conversationId: 'conv-123',
      reportId: 'r1',
    });
  });

  it('builds a forge window payload from returned window metadata', () => {
    expect(buildWindowPayload('metricReportBuilder', {
      data: {
        namespace: 'Performance Metrics',
        presentation: 'hosted',
        region: 'chat.top',
        actions: { code: '(() => ({ stewardReportBuilder: { buildRequest() { return {}; } } }))()' },
        view: { content: { id: 'root' } },
      },
    }, { reportId: 'capacityTrendQ3', conversationId: 'conv-123' })).toMatchObject({
      windowId: 'mcpui:metricReportBuilder',
      windowKey: 'metricReportBuilder',
      id: 'metricReportBuilder',
      stateKey: 'metricReportBuilder',
      windowTitle: 'Performance Metrics',
      presentation: 'hosted',
      region: 'chat.top',
      conversationId: 'conv-123',
      parameters: { reportId: 'capacityTrendQ3', conversationId: 'conv-123' },
      isInTab: true,
      actions: {
        code: '(() => ({ stewardReportBuilder: { buildRequest() { return {}; } } }))()',
      },
    });
  });

  it('merges a scoped hosted workspace snapshot over the direct forge-window payload', () => {
    const merged = mergeForgeWindowPayloadWithScopedSnapshot(
      buildWindowPayload('metricReportBuilder', {
        data: {
          namespace: 'Performance Metrics',
          presentation: 'hosted',
          region: 'chat.top',
        },
      }, { conversationId: 'conv-123' }),
      {
        windowId: 'metricReportBuilder__conv-123',
        windowTitle: 'Performance Metrics',
        conversationId: 'conv-123',
        parentKey: 'chat/new',
        workspaceSharePct: 72,
        workspaceMinHeight: 500,
        parameters: {
          reportStarterId: '__blank__',
        },
        windowForm: {
          metricsCubeBuilder: {
            selectedMeasures: ['totalSpend'],
          },
        },
      },
    );
    expect(merged).toMatchObject({
      windowId: 'metricReportBuilder__conv-123',
      conversationId: 'conv-123',
      parentKey: 'chat/new',
      workspaceSharePct: 72,
      workspaceMinHeight: 500,
      parameters: {
        conversationId: 'conv-123',
        reportStarterId: '__blank__',
      },
      windowForm: {
        metricsCubeBuilder: {
          selectedMeasures: ['totalSpend'],
        },
      },
    });
  });

  it('restores a scoped forge window snapshot from canonical transcript turns when the current tab has no scoped state yet', async () => {
    const fetchStub = async () => new Response(JSON.stringify({
      conversation: {
        conversationId: 'conv-123',
        turns: [
          { id: 'turn-1', status: 'completed' },
        ],
      },
    }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
    const syncCalls = [];
    const next = await restoreScopedForgeWindowSnapshot({
      fetchImpl: fetchStub,
      conversationId: 'conv-123',
      windowKey: 'metricReportBuilder',
      getScopedWorkspaceWindowsStateImpl: () => ([
        {
          windowId: 'metricReportBuilder__conv-123',
          windowKey: 'metricReportBuilder',
          conversationId: 'conv-123',
          windowForm: {
            metricsCubeBuilder: {
              selectedMeasures: ['totalSpend'],
            },
          },
        },
      ]),
      getScopedWorkspaceSelectionImpl: () => 'metricReportBuilder__conv-123',
      syncScopedWorkspaceStateFromTranscriptTurnsImpl: (...args) => {
        syncCalls.push(args);
        return {
          selectedWindowId: 'metricReportBuilder__conv-123',
          windows: [
            {
              windowId: 'metricReportBuilder__conv-123',
              windowKey: 'metricReportBuilder',
            },
          ],
        };
      },
    });
    expect(syncCalls).toHaveLength(1);
    expect(syncCalls[0][0]).toBe('conv-123');
    expect(syncCalls[0][2]).toMatchObject({
      reopen: false,
      announce: false,
      allowRunning: true,
    });
    expect(next).toMatchObject({
      windowId: 'metricReportBuilder__conv-123',
      windowKey: 'metricReportBuilder',
      conversationId: 'conv-123',
      windowForm: {
        metricsCubeBuilder: {
          selectedMeasures: ['totalSpend'],
        },
      },
    });
  });

  it('extracts report starters from a forge window payload and only shows the chooser before selection', () => {
    const windowPayload = {
      reportBuilder: {
        reportDocumentTemplates: [
          { id: 'performance_overview', label: 'Performance Overview', description: 'Date-first starter.' },
          { id: 'channel_mix', label: 'Channel Mix', description: 'Channel-first starter.' },
        ],
      },
    };
    expect(buildForgeReportStarterOptions(windowPayload)).toEqual([
      { id: 'performance_overview', label: 'Performance Overview', description: 'Date-first starter.' },
      { id: 'channel_mix', label: 'Channel Mix', description: 'Channel-first starter.' },
    ]);
    expect(shouldShowForgeReportStarterChooser({
      window: windowPayload,
      windowParams: {},
    })).toBe(true);
    expect(shouldShowForgeReportStarterChooser({
      window: windowPayload,
      windowParams: { reportStarterId: 'performance_overview' },
    })).toBe(false);
    expect(shouldShowForgeReportStarterChooser({
      window: { reportBuilder: {} },
      windowParams: {},
    })).toBe(false);
  });

  it('defaults missing namespace and placement metadata for standalone route usage', () => {
    expect(buildWindowPayload('forecastingCubeBuilder', { data: {} }, null)).toMatchObject({
      windowId: 'mcpui:forecastingCubeBuilder',
      windowKey: 'forecastingCubeBuilder',
      id: 'forecastingCubeBuilder',
      stateKey: 'forecastingCubeBuilder',
      windowTitle: 'forecastingCubeBuilder',
      presentation: 'hosted',
      region: 'mcpui.bubble',
      parameters: {},
      isInTab: true,
    });
  });

  it('appends target context fields and repeated capabilities to a request url', () => {
    const params = new URLSearchParams();
    appendTargetContext(params, {
      platform: 'web',
      formFactor: 'desktop',
      surface: 'browser',
      capabilities: ['markdown', 'chart', 'code'],
    });
    expect(params.get('platform')).toBe('web');
    expect(params.get('formFactor')).toBe('desktop');
    expect(params.get('surface')).toBe('browser');
    expect(params.getAll('capabilities')).toEqual(['markdown', 'chart', 'code']);
  });

  it('builds metric and forecasting forge window request urls with target context', () => {
    const metricURL = new URL(buildForgeWindowRequestURL('metricReportBuilder', {
      platform: 'web',
      formFactor: 'desktop',
      surface: 'browser',
      capabilities: ['markdown', 'chart'],
    }, {
      conversationId: 'conv-123',
    }), 'http://example.test');
    expect(metricURL.pathname).toBe('/v1/api/agently/forge/window/metricReportBuilder');
    expect(metricURL.searchParams.get('platform')).toBe('web');
    expect(metricURL.searchParams.get('formFactor')).toBe('desktop');
    expect(metricURL.searchParams.get('surface')).toBe('browser');
    expect(metricURL.searchParams.get('conversationId')).toBe('conv-123');
    expect(metricURL.searchParams.getAll('capabilities')).toEqual(['markdown', 'chart']);

    const forecastingURL = new URL(buildForgeWindowRequestURL('forecastingCubeBuilder', {
      platform: 'web',
      formFactor: 'desktop',
      surface: 'browser',
      capabilities: ['markdown', 'chart', 'upload', 'code', 'diff'],
    }, {
      conversationId: 'conv-456',
    }), 'http://example.test');
    expect(forecastingURL.pathname).toBe('/v1/api/agently/forge/window/forecastingCubeBuilder');
    expect(forecastingURL.searchParams.get('conversationId')).toBe('conv-456');
    expect(forecastingURL.searchParams.getAll('capabilities')).toEqual(['markdown', 'chart', 'upload', 'code', 'diff']);
  });

  it('returns an empty request url when no window key is provided', () => {
    expect(buildForgeWindowRequestURL('', {
      platform: 'web',
      capabilities: ['markdown'],
    })).toBe('');
  });

  it('returns a missing-windowKey state without calling fetch', async () => {
    const fetchSpy = async () => {
      throw new Error('fetch should not be called');
    };
    await expect(fetchForgeWindowPageState({
      fetchImpl: fetchSpy,
      windowKey: '',
      targetContext: { platform: 'web' },
    })).resolves.toEqual({
      loading: false,
      error: 'windowKey is required',
      window: null,
    });
  });

  it('fetches and shapes a hosted metric report window payload', async () => {
    const calls = [];
    const fetchStub = async (url, options) => {
      calls.push({ url, options });
      return new Response(JSON.stringify({
        data: {
          namespace: 'Performance Metrics',
          presentation: 'hosted',
          region: 'chat.top',
          actions: {
            code: '(() => ({ stewardReportBuilder: { buildRequest() { return {}; } } }))()',
          },
          view: { content: { id: 'metric-root' } },
        },
      }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    };
    const next = await fetchForgeWindowPageState({
      fetchImpl: fetchStub,
      windowKey: 'metricReportBuilder',
      targetContext: {
        platform: 'web',
        formFactor: 'desktop',
        surface: 'browser',
        capabilities: ['markdown', 'chart'],
      },
      windowParams: {
        reportId: 'capacityTrendQ3',
        conversationId: 'conv-metric',
      },
    });
    expect(calls).toHaveLength(2);
    expect(calls[0].url).toContain('/v1/api/agently/forge/window/metricReportBuilder');
    expect(calls[0].url).toContain('conversationId=conv-metric');
    expect(calls[0].url).toContain('platform=web');
    expect(calls[0].url).toContain('formFactor=desktop');
    expect(calls[0].url).toContain('surface=browser');
    expect(calls[0].url).toContain('capabilities=markdown');
    expect(calls[0].url).toContain('capabilities=chart');
    expect(calls[0].options).toMatchObject({
      method: 'GET',
      credentials: 'include',
      headers: { Accept: 'application/json' },
    });
    expect(calls[1].url).toContain('/v1/conversations/conv-metric/transcript');
    expect(calls[1].url).toContain('includeModelCalls=true');
    expect(calls[1].url).toContain('includeToolCalls=true');
    expect(next).toMatchObject({
      loading: false,
      error: '',
      window: {
        windowId: 'mcpui:metricReportBuilder',
        windowKey: 'metricReportBuilder',
        windowTitle: 'Performance Metrics',
        presentation: 'hosted',
        region: 'chat.top',
        conversationId: 'conv-metric',
        parameters: { reportId: 'capacityTrendQ3', conversationId: 'conv-metric' },
        actions: {
          code: '(() => ({ stewardReportBuilder: { buildRequest() { return {}; } } }))()',
        },
      },
    });
  });

  it('fetches and shapes a hosted forecasting window payload', async () => {
    const fetchStub = async () => new Response(JSON.stringify({
      data: {
        namespace: 'Forecasting',
        presentation: 'hosted',
        region: 'chat.top',
        view: { content: { id: 'forecast-root' } },
      },
    }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
    const next = await fetchForgeWindowPageState({
      fetchImpl: fetchStub,
      windowKey: 'forecastingCubeBuilder',
      targetContext: {
        platform: 'web',
        formFactor: 'desktop',
        surface: 'browser',
        capabilities: ['markdown', 'chart', 'upload', 'code', 'diff'],
      },
      windowParams: {
        lineId: 7288336,
      },
    });
    expect(next).toMatchObject({
      loading: false,
      error: '',
      window: {
        windowId: 'mcpui:forecastingCubeBuilder',
        windowKey: 'forecastingCubeBuilder',
        windowTitle: 'Forecasting',
        presentation: 'hosted',
        region: 'chat.top',
        parameters: { lineId: 7288336 },
      },
    });
  });

  it('rejects an incomplete report builder window that declares hooks without action code', async () => {
    const fetchStub = async () => new Response(JSON.stringify({
      data: {
        namespace: 'Performance Metrics',
        presentation: 'hosted',
        region: 'chat.top',
        reportBuilder: {
          hooks: {
            initializeState: 'Performance Metrics.stewardReportBuilder.initializeState',
            buildRequest: 'Performance Metrics.stewardReportBuilder.buildRequest',
          },
        },
        view: { content: { id: 'metric-root' } },
      },
    }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
    await expect(fetchForgeWindowPageState({
      fetchImpl: fetchStub,
      windowKey: 'metricReportBuilder',
      targetContext: {
        platform: 'web',
      },
    })).rejects.toThrow('This workspace definition is incomplete. Refresh the backend and retry.');
  });

  it('surfaces backend error text when the hosted window request fails', async () => {
    const fetchStub = async () => new Response('workspace config failed', {
      status: 500,
      headers: { 'Content-Type': 'text/plain' },
    });
    await expect(fetchForgeWindowPageState({
      fetchImpl: fetchStub,
      windowKey: 'metricReportBuilder',
      targetContext: {},
    })).rejects.toThrow('workspace config failed');
  });

  it('falls back to status-based messaging when the failure body is empty', async () => {
    const fetchStub = async () => new Response('', {
      status: 404,
      headers: { 'Content-Type': 'text/plain' },
    });
    await expect(fetchForgeWindowPageState({
      fetchImpl: fetchStub,
      windowKey: 'forecastingCubeBuilder',
      targetContext: {},
    })).rejects.toThrow('window fetch failed (404)');
  });
});
