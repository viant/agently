import { describe, expect, it } from 'vitest';

import { buildCanonicalTranscriptRows } from './renderRows';

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
});
