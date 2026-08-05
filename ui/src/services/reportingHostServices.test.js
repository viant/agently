import { beforeEach, describe, expect, it, vi } from 'vitest';

const { fetchDatasource } = vi.hoisted(() => ({
  fetchDatasource: vi.fn(),
}));
const chatStore = vi.hoisted(() => ({
  getProjection: vi.fn(),
  subscribe: vi.fn(),
}));

vi.mock('../components/lookups/client', () => ({
  fetchDatasource,
}));
vi.mock('./chatStore', () => chatStore);

import {
  buildReportProvenanceFromRows,
  createReportingHostServices,
  fetchReportBuilderPreviewByRef,
  getReportBuildProvenance,
  subscribeReportBuildProvenance,
} from './reportingHostServices';

describe('reportingHostServices report-builder preview adapter', () => {
  beforeEach(() => {
    fetchDatasource.mockReset();
    chatStore.getProjection.mockReset();
    chatStore.subscribe.mockReset();
  });

  it('routes each logical report dataset request through the authenticated datasource API', async () => {
    fetchDatasource.mockResolvedValue({ rows: [{ totalSpend: 42 }] });
    const services = createReportingHostServices();
    const request = {
      measures: { totalSpend: true },
      dimensions: { eventDate: true },
      filters: { orderIds: [2672373] },
    };

    await expect(services.reportBuilderPreview.fetchByRef({
      dataSourceRef: 'metrics_ad_cube_report',
      parameters: request,
      omitConversationId: true,
      builderContext: { conversationId: 'not-forwarded' },
    })).resolves.toEqual({ rows: [{ totalSpend: 42 }] });

    expect(fetchDatasource).toHaveBeenCalledOnce();
    expect(fetchDatasource).toHaveBeenCalledWith('metrics_ad_cube_report', request);
  });

  it('rejects an authored dataset without a registered data source reference', async () => {
    await expect(fetchReportBuilderPreviewByRef({
      parameters: { measures: { impressions: true } },
    })).rejects.toThrow('Report preview requires a data source reference.');
    expect(fetchDatasource).not.toHaveBeenCalled();
  });

  it('projects the initial request and deduplicated tool history without interpreting it', () => {
    expect(buildReportProvenanceFromRows([
      { kind: 'user', content: 'Build a delivery report.' },
      {
        role: 'assistant',
        turnId: 'turn-1',
        executionGroups: [{
          toolSteps: [
            { toolCallId: 'call-1', toolName: 'steward/MetricsAdCube', status: 'completed', completedAt: '2026-08-04T12:00:00Z' },
            { toolCallId: 'call-1', toolName: 'steward/MetricsAdCube', status: 'completed' },
            { toolCallId: 'call-2', toolName: 'ui/window/setFormData', status: 'failed' },
          ],
        }],
      },
    ])).toEqual({
      initialPrompt: 'Build a delivery report.',
      events: [
        {
          id: 'call-1',
          label: 'steward/MetricsAdCube',
          status: 'completed',
          completedAt: '2026-08-04T12:00:00Z',
        },
        {
          id: 'call-2',
          label: 'ui/window/setFormData',
          status: 'failed',
        },
      ],
    });
  });

  it('projects the first non-empty user prompt and unique tool events with fallbacks', () => {
    const provenance = buildReportProvenanceFromRows([
      { kind: 'user', content: '   ' },
      { role: ' USER ', text: '  Build a sales report  ' },
      { kind: 'user', message: 'Do not use this later prompt' },
      {
        kind: 'iteration',
        turnId: ' turn-7 ',
        executionGroups: [
          {
            toolSteps: [
              {
                toolName: ' lookup/orders ',
                toolCallId: ' call-1 ',
                status: ' RUNNING ',
                startedAt: ' 2026-08-04T08:00:00Z ',
                completedAt: ' 2026-08-04T08:00:01Z ',
              },
              {
                name: ' render/report ',
                toolMessageId: ' message-2 ',
                status: '',
                completedAt: '',
                finishedAt: ' 2026-08-04T08:00:02Z ',
              },
              {
                toolName: '   ',
                toolCallId: 'ignored-empty-label',
              },
            ],
          },
          {
            toolSteps: [
              { name: 'publish/report' },
              {
                toolName: 'duplicate should be ignored',
                toolCallId: 'call-1',
                status: 'failed',
              },
            ],
          },
        ],
      },
    ]);

    expect(provenance).toEqual({
      initialPrompt: 'Build a sales report',
      events: [
        {
          id: 'call-1',
          label: 'lookup/orders',
          status: 'running',
          startedAt: '2026-08-04T08:00:00Z',
          completedAt: '2026-08-04T08:00:01Z',
        },
        {
          id: 'message-2',
          label: 'render/report',
          status: 'completed',
          completedAt: '2026-08-04T08:00:02Z',
        },
        {
          id: 'turn-7:2:1:publish/report',
          label: 'publish/report',
          status: 'completed',
        },
      ],
    });
  });

  it('keeps only the last 50 projected tool events', () => {
    const toolSteps = Array.from({ length: 55 }, (_, index) => ({
      toolName: `tool-${index}`,
      toolCallId: `call-${index}`,
    }));

    const provenance = buildReportProvenanceFromRows([
      {
        id: 'turn-limit',
        executionGroups: [{ toolSteps }],
      },
    ]);

    expect(provenance.events).toHaveLength(50);
    expect(provenance.events.map((event) => event.id)).toEqual(
      Array.from({ length: 50 }, (_, index) => `call-${index + 5}`),
    );
  });

  it('builds provenance from the trimmed conversation projection', () => {
    chatStore.getProjection.mockReturnValue([
      { kind: 'user', content: 'Projected prompt' },
      {
        turnId: 'turn-projected',
        executionGroups: [{
          toolSteps: [{ toolName: 'project/tool', toolCallId: 'call-projected' }],
        }],
      },
    ]);

    expect(getReportBuildProvenance({ conversationId: ' conv-42 ' })).toEqual({
      initialPrompt: 'Projected prompt',
      events: [{
        id: 'call-projected',
        label: 'project/tool',
        status: 'completed',
      }],
    });
    expect(chatStore.getProjection).toHaveBeenCalledOnce();
    expect(chatStore.getProjection).toHaveBeenCalledWith('conv-42');

    chatStore.getProjection.mockClear();
    expect(getReportBuildProvenance({ conversationId: '   ' })).toEqual({
      initialPrompt: '',
      events: [],
    });
    expect(chatStore.getProjection).not.toHaveBeenCalled();
  });

  it('publishes provenance again when the conversation projection hydrates', () => {
    let notifyStore;
    const unsubscribe = vi.fn();
    chatStore.getProjection
      .mockReturnValueOnce([])
      .mockReturnValueOnce([
        { kind: 'user', content: 'Build the hydrated report.' },
        {
          kind: 'iteration',
          turnId: 'turn-1',
          rounds: [{
            toolCalls: [{
              toolCallId: 'call-1',
              toolName: 'ui/view/open',
              status: 'completed',
              startedAt: '2026-08-04T12:00:00Z',
              completedAt: '2026-08-04T12:00:01Z',
            }],
          }],
        },
      ]);
    chatStore.subscribe.mockImplementation((listener) => {
      notifyStore = listener;
      return unsubscribe;
    });
    const listener = vi.fn();
    const services = createReportingHostServices();

    const stop = services.reportProvenance.subscribeBuildContext(
      { conversationId: 'conversation-1' },
      listener,
    );
    notifyStore();
    stop();

    expect(listener).toHaveBeenNthCalledWith(1, { initialPrompt: '', events: [] });
    expect(listener).toHaveBeenNthCalledWith(2, {
      initialPrompt: 'Build the hydrated report.',
      events: [{
        id: 'call-1',
        label: 'ui/view/open',
        status: 'completed',
        startedAt: '2026-08-04T12:00:00Z',
        completedAt: '2026-08-04T12:00:01Z',
      }],
    });
    expect(unsubscribe).toHaveBeenCalledOnce();
  });

  it('exposes both provenance getters through reporting host services', () => {
    const services = createReportingHostServices();

    expect(services.reportProvenance).toEqual({
      getBuildContext: getReportBuildProvenance,
      subscribeBuildContext: subscribeReportBuildProvenance,
    });
  });
});
