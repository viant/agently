import { describe, expect, it } from 'vitest';

import { buildCanonicalTranscriptRows } from './renderRows';

describe('buildCanonicalTranscriptRows', () => {
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
            preamble: '',
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
});
