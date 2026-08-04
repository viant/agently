import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getProjectionMock } = vi.hoisted(() => ({
  getProjectionMock: vi.fn(),
}));

vi.mock('./chatStore', () => ({
  getProjection: getProjectionMock,
}));

import {
  buildReportProvenanceFromRows,
  createReportingHostServices,
  getReportBuildProvenance,
} from './reportingHostServices';

beforeEach(() => {
  getProjectionMock.mockReset();
});

describe('reportingHostServices provenance', () => {
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
              {
                name: 'publish/report',
              },
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
    getProjectionMock.mockReturnValue([
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
    expect(getProjectionMock).toHaveBeenCalledOnce();
    expect(getProjectionMock).toHaveBeenCalledWith('conv-42');

    getProjectionMock.mockClear();
    expect(getReportBuildProvenance({ conversationId: '   ' })).toEqual({
      initialPrompt: '',
      events: [],
    });
    expect(getProjectionMock).not.toHaveBeenCalled();
  });

  it('exposes the build provenance getter through reporting host services', () => {
    const services = createReportingHostServices();

    expect(services.reportProvenance).toEqual({
      getBuildContext: getReportBuildProvenance,
    });
  });
});
