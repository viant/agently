import { describe, expect, it } from 'vitest';

import { buildCanonicalTranscriptRows, buildConversationRenderRows } from './renderRows';

describe('buildCanonicalTranscriptRows', () => {
  it('keeps intake pages visible in execution groups even when iteration is 0', () => {
    const turn = {
      turnId: 'turn-intake',
      createdAt: '2026-04-18T10:00:00Z',
      status: 'running',
      user: {
        messageId: 'user-intake',
        content: 'Analyze order 1'
      },
      execution: {
        pages: [
          {
            pageId: 'page-intake',
            assistantMessageId: 'page-intake',
            iteration: 0,
            phase: 'intake',
            status: 'completed',
            narration: '',
            content: '',
            modelCall: {
              provider: 'openai',
              model: 'gpt-5-mini',
              status: 'completed'
            },
            toolCalls: []
          }
        ]
      }
    };

    const { rows } = buildCanonicalTranscriptRows([turn]);
    const assistant = rows.find((row) => String(row?.role || '').toLowerCase() === 'assistant');
    expect(assistant?.executionGroups).toHaveLength(1);
    expect(assistant?.executionGroups?.[0]).toMatchObject({
      pageId: 'page-intake',
      phase: 'intake'
    });
  });

  it('does not add a second assistant elicitation row when the turn already has an assistant execution row', () => {
    const turn = {
      turnId: 'turn-1',
      createdAt: '2026-04-15T23:00:00Z',
      status: 'waiting_for_user',
      user: {
        messageId: 'user-1',
        content: 'Before answering, ask me for my favorite color.'
      },
      execution: {
        pages: [
          {
            assistantMessageId: 'assistant-1',
            sequence: 1,
            status: 'waiting_for_user',
            narration: '',
            content: 'Please provide your favorite color.',
            modelCall: {
              provider: 'openai',
              model: 'gpt-5-mini',
              status: 'completed'
            },
            toolCalls: []
          }
        ]
      },
      elicitation: {
        elicitationId: 'elic-1',
        status: 'pending',
        message: 'Please provide your favorite color.',
        requestedSchema: {
          type: 'object',
          properties: {
            favoriteColor: { type: 'string' }
          }
        }
      }
    };

    const { rows } = buildCanonicalTranscriptRows([turn]);
    const assistantRows = rows.filter((row) => String(row?.role || '').toLowerCase() === 'assistant');

    expect(assistantRows).toHaveLength(1);
    expect(assistantRows[0]?.id).toBe('assistant-1');
  });

  it('does not default historical transcript elicitation rows to pending when assistant status is rejected', () => {
    const turn = {
      turnId: 'turn-elic-rejected',
      createdAt: '2026-05-19T21:16:21Z',
      status: 'succeeded',
      user: {
        messageId: 'user-elic-rejected',
        content: 'recommend sites'
      },
      assistant: {
        final: {
          status: 'rejected',
          content: ''
        }
      },
      elicitation: {
        elicitationId: 'elic-rejected-1',
        message: 'Review the selected site recommendation changes before patching.',
        requestedSchema: {
          type: 'object',
          properties: {
            rows: { type: 'array' }
          }
        }
      },
      execution: {
        pages: []
      }
    };

    const { rows } = buildCanonicalTranscriptRows([turn]);
    const elicitationRow = rows.find((row) => row?.elicitationId === 'elic-rejected-1');

    expect(elicitationRow?.status).toBe('rejected');
    expect(elicitationRow?.elicitation?.status).toBe('rejected');
  });

  it('preserves failed historical transcript elicitation status', () => {
    const turn = {
      turnId: 'turn-elic-failed',
      createdAt: '2026-05-29T15:27:06Z',
      status: 'canceled',
      user: {
        messageId: 'user-elic-failed',
        content: 'standby'
      },
      assistant: {
        final: {
          status: 'failed',
          content: ''
        }
      },
      elicitation: {
        elicitationId: 'elic-failed-1',
        status: 'failed',
        message: 'Please provide missing inputs.',
        requestedSchema: {
          type: 'object',
          properties: {
            input: { type: 'string' }
          }
        }
      },
      execution: {
        pages: []
      }
    };

    const { rows } = buildCanonicalTranscriptRows([turn]);
    const elicitationRow = rows.find((row) => row?.elicitationId === 'elic-failed-1');

    expect(elicitationRow?.status).toBe('failed');
    expect(elicitationRow?.elicitation?.status).toBe('failed');
  });

  it('preserves tool step content on canonical execution rows', () => {
    const turn = {
      turnId: 'turn-status',
      createdAt: '2026-04-21T00:00:00Z',
      status: 'running',
      user: {
        messageId: 'user-status',
        content: 'Check blockers'
      },
      execution: {
        pages: [
          {
            pageId: 'page-status',
            assistantMessageId: 'page-status',
            parentMessageId: 'page-status',
            iteration: 1,
            status: 'running',
            narration: '',
            content: '',
            modelSteps: [{
              assistantMessageId: 'page-status',
              provider: 'openai',
              model: 'gpt-5.4',
              status: 'completed'
            }],
            toolSteps: [{
              toolCallId: 'call-status',
              toolMessageId: 'tool-status',
              toolName: 'llm/agents:status',
              status: 'running',
              content: 'Reviewing blocker diagnosis on each order in parallel.'
            }]
          }
        ]
      }
    };

    const { rows } = buildCanonicalTranscriptRows([turn]);
    const assistant = rows.find((row) => String(row?.role || '').toLowerCase() === 'assistant');
    expect(assistant?.executionGroups?.[0]?.toolSteps?.[0]).toMatchObject({
      toolName: 'llm/agents:status',
      status: 'running',
      content: 'Reviewing blocker diagnosis on each order in parallel.'
    });
  });

  it('carries explicit turnStartedAt on assistant execution rows from transcript turns', () => {
    const turn = {
      turnId: 'turn-anchor',
      createdAt: '2026-05-05T16:23:21Z',
      status: 'running',
      user: {
        messageId: 'user-anchor',
        content: 'Analyze 553065 campaign performance'
      },
      execution: {
        pages: [
          {
            pageId: 'page-anchor',
            assistantMessageId: 'page-anchor',
            iteration: 1,
            status: 'running',
            narration: 'Reviewing Hierarchy, Targeting profile, Pacing ad order.',
            content: '',
            modelSteps: [{
              assistantMessageId: 'page-anchor',
              provider: 'openai',
              model: 'gpt-5.4',
              status: 'completed',
              startedAt: '2026-05-05T16:23:38Z'
            }],
            toolSteps: []
          }
        ]
      }
    };

    const { rows } = buildCanonicalTranscriptRows([turn]);
    const assistant = rows.find((row) => String(row?.role || '').toLowerCase() === 'assistant');
    expect(assistant?.turnStartedAt).toBe('2026-05-05T16:23:21Z');
  });

  it('moves persisted router assistant JSON into intake execution details instead of a bubble', () => {
    const turn = {
      turnId: 'turn-router',
      createdAt: '2026-05-04T06:23:29Z',
      status: 'succeeded',
      user: {
        messageId: 'user-router',
        content: 'forecast line 7281841'
      },
      messages: [
        {
          messageId: 'router-json',
          role: 'assistant',
          mode: 'router',
          status: '',
          content: '{"appendToolBundles":["analyst-forecasting-tools","analyst-baseline"],"clarificationNeeded":false}'
        },
        {
          messageId: 'assistant-final',
          role: 'assistant',
          content: 'I’m pulling the active setup now.'
        }
      ],
      execution: {
        pages: []
      }
    };

    const { rows } = buildCanonicalTranscriptRows([turn]);
    const executionAssistant = rows.find((row) =>
      String(row?.role || '').toLowerCase() === 'assistant'
      && Array.isArray(row?.executionGroups)
      && row.executionGroups.length > 0
    );

    expect(rows.some((row) => String(row?.content || '').includes('appendToolBundles'))).toBe(false);
    expect(rows.some((row) => String(row?.content || '').includes('I’m pulling the active setup now.'))).toBe(true);
    expect(executionAssistant?.executionGroups?.[0]).toMatchObject({
      phase: 'intake',
      assistantMessageId: 'router-json'
    });
    expect(executionAssistant?.executionGroups?.[0]?.modelSteps?.[0]?.responsePayload)
      .toContain('appendToolBundles');
  });

  it('preserves extra same-turn user and assistant messages as standalone rows', () => {
    const turn = {
      turnId: 'turn-extra',
      createdAt: '2026-04-21T00:00:00Z',
      status: 'completed',
      user: {
        messageId: 'user-1',
        content: 'Initial ask'
      },
      messages: [
        {
          messageId: 'user-2',
          role: 'user',
          content: 'Steer: narrow to ad order scope',
          createdAt: '2026-04-21T00:00:02Z',
          sequence: 2,
          interim: 0
        },
        {
          messageId: 'assistant-note-1',
          role: 'assistant',
          content: 'PRELIMINARY NOTE',
          createdAt: '2026-04-21T00:00:03Z',
          sequence: 3,
          interim: 0
        }
      ],
      execution: {
        pages: [{
          pageId: 'page-final',
          assistantMessageId: 'page-final',
          sequence: 4,
          status: 'completed',
          finalResponse: true,
          content: 'Final answer',
          modelSteps: [{
            modelCallId: 'mc-final',
            assistantMessageId: 'page-final',
            provider: 'openai',
            model: 'gpt-5.4',
            status: 'completed'
          }],
          toolSteps: []
        }]
      }
    };

    const { rows } = buildCanonicalTranscriptRows([turn]);
    expect(rows.find((row) => row?.id === 'user-2')).toMatchObject({
      role: 'user',
      content: 'Steer: narrow to ad order scope'
    });
    expect(rows.find((row) => row?.id === 'assistant-note-1')).toMatchObject({
      role: 'assistant',
      content: 'PRELIMINARY NOTE'
    });
    expect(rows.find((row) => row?.id === 'page-final')).toMatchObject({
      role: 'assistant',
      content: 'Final answer'
    });
    expect(rows.map((row) => row?.id)).toEqual(['user-1', 'user-2', 'assistant-note-1', 'page-final']);
  });

  it('orders same-turn standalone rows by sequence before createdAt drift', () => {
    const turn = {
      turnId: 'turn-seq',
      createdAt: '2026-04-21T00:00:00Z',
      status: 'completed',
      user: {
        messageId: 'user-1',
        content: 'Initial ask',
        sequence: 1
      },
      messages: [
        {
          messageId: 'assistant-note-2',
          role: 'assistant',
          content: 'Second note',
          createdAt: '2026-04-21T00:00:05Z',
          sequence: 3,
          interim: 0
        },
        {
          messageId: 'assistant-note-1',
          role: 'assistant',
          content: 'First note',
          createdAt: '2026-04-21T00:00:06Z',
          sequence: 2,
          interim: 0
        }
      ]
    };

    const { rows } = buildCanonicalTranscriptRows([turn]);
    expect(rows.map((row) => row?.id)).toEqual(['user-1', 'assistant-note-1', 'assistant-note-2']);
  });

  it('orders the final execution bubble after a standalone same-turn note when the page sequence is later', () => {
    const turn = {
      turnId: 'turn-order',
      createdAt: '2026-04-21T00:00:00Z',
      status: 'completed',
      user: {
        messageId: 'user-1',
        content: 'Initial ask',
        sequence: 1,
      },
      messages: [
        {
          messageId: 'assistant-note-1',
          role: 'assistant',
          content: 'PRELIMINARY NOTE',
          createdAt: '2026-04-21T00:00:03Z',
          sequence: 10,
          interim: 0,
        },
      ],
      execution: {
        pages: [{
          pageId: 'page-final',
          assistantMessageId: 'page-final',
          sequence: 11,
          status: 'completed',
          finalResponse: true,
          content: 'Final answer',
          modelSteps: [{
            modelCallId: 'mc-final',
            assistantMessageId: 'page-final',
            provider: 'openai',
            model: 'gpt-5.4',
            status: 'completed'
          }],
          toolSteps: []
        }]
      }
    };

    const { rows } = buildCanonicalTranscriptRows([turn]);
    expect(rows.map((row) => row?.id)).toEqual(['user-1', 'assistant-note-1', 'page-final']);
  });

  it('does not hold a later succeeded turn behind stale live ownership when that later turn already has persisted assistant content', () => {
    const turns = [
      {
        turnId: 'turn-failed-live',
        createdAt: '2026-05-06T17:05:45Z',
        status: 'failed',
        user: {
          messageId: 'user-1',
          content: 'Forecast viant.taxonomy 31312 and location US/CA'
        },
        execution: {
          pages: [
            {
              pageId: 'page-live',
              assistantMessageId: 'page-live',
              iteration: 2,
              status: 'failed',
              content: 'failed forecast details',
              modelSteps: [],
              toolSteps: []
            }
          ]
        }
      },
      {
        turnId: 'turn-succeeded-final',
        createdAt: '2026-05-06T17:09:26Z',
        status: 'succeeded',
        user: {
          messageId: 'user-2',
          content: ''
        },
        assistant: {
          final: {
            messageId: 'assistant-final',
            content: '```forge-data\\n{"version":1}\\n```'
          }
        },
        execution: {
          pages: []
        }
      }
    ];

    const { rows, queuedTurns } = buildCanonicalTranscriptRows(turns, {
      holdAfterTurnId: 'turn-failed-live',
      pendingElicitations: []
    });

    expect(queuedTurns).toHaveLength(0);
    expect(rows.some((row) => row?.turnId === 'turn-succeeded-final' && String(row?.content || '').includes('forge-data'))).toBe(true);
  });
});

describe('buildConversationRenderRows', () => {
  it('drops empty interim assistant placeholders that only carry internal model steps', () => {
    const { mergedRows } = buildConversationRenderRows({
      transcriptRows: [],
      liveRows: [{
        id: 'assistant-empty',
        role: 'assistant',
        turnId: 'turn-1',
        interim: 1,
        content: '',
        narration: '',
        executionGroups: [{
          assistantMessageId: 'assistant-empty',
          status: 'completed',
          narration: '',
          content: '',
          finalResponse: false,
          modelSteps: [{
            modelCallId: 'mc-1',
            provider: 'openai',
            model: 'gpt-5.4',
            status: 'completed'
          }],
          toolSteps: [],
          toolCallsPlanned: []
        }]
      }]
    });

    expect(mergedRows).toEqual([]);
  });
});
