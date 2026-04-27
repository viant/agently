import { describe, expect, it } from 'vitest';

import {
  canonicalExecutionPages,
  extractCanonicalExecutionGroups,
  flattenCanonicalTranscriptSteps,
  transcriptConversationTurns,
} from './canonicalTranscript';

describe('canonicalTranscript', () => {
  it('extracts canonical turns only from the wrapped transcript response', () => {
    const turns = transcriptConversationTurns({
      conversation: {
        turns: [{ turnId: 'turn-1' }]
      }
    });

    expect(turns).toEqual([{ turnId: 'turn-1' }]);
    expect(transcriptConversationTurns({ turns: [{ turnId: 'legacy' }] })).toEqual([]);
  });

  it('flattens canonical model and tool steps from execution pages', () => {
    const turns = [{
      turnId: 'turn-1',
      execution: {
        pages: [{
          pageId: 'page-1',
          assistantMessageId: 'page-1',
          iteration: 1,
          status: 'completed',
          modelSteps: [{
            modelCallId: 'mc-1',
            assistantMessageId: 'page-1',
            executionRole: 'intake',
            provider: 'openai',
            model: 'gpt-5.4',
            status: 'completed',
            requestPayloadId: 'req-1'
          }],
          toolSteps: [{
            toolCallId: 'tc-1',
            toolMessageId: 'tm-1',
            toolName: 'llm/agents-run',
            executionRole: 'worker',
            content: 'Checking blocker diagnostics now.',
            status: 'completed',
            linkedConversationId: 'child-1',
            responsePayloadId: 'resp-1'
          }]
        }]
      }
    }];

    const steps = flattenCanonicalTranscriptSteps(turns);

    expect(steps).toHaveLength(2);
    expect(steps[0]).toMatchObject({
      id: 'mc-1',
      kind: 'model',
      executionRole: 'intake',
      provider: 'openai',
      model: 'gpt-5.4',
      requestPayloadId: 'req-1'
    });
    expect(steps[1]).toMatchObject({
      id: 'tc-1',
      kind: 'tool',
      executionRole: 'worker',
      toolName: 'llm/agents-run',
      content: 'Checking blocker diagnostics now.',
      linkedConversationId: 'child-1',
      responsePayloadId: 'resp-1'
    });
  });

  it('does not inherit page completion status for tool steps with no own status', () => {
    const turns = [{
      turnId: 'turn-1',
      execution: {
        pages: [{
          pageId: 'page-1',
          assistantMessageId: 'page-1',
          status: 'completed',
          toolSteps: [{
            toolCallId: 'tc-1',
            toolMessageId: 'tm-1',
            toolName: 'llm/agents/start'
          }]
        }]
      }
    }];

    const steps = flattenCanonicalTranscriptSteps(turns);

    expect(steps[0]).toMatchObject({
      id: 'tc-1',
      kind: 'tool',
      toolName: 'llm/agents/start',
      status: ''
    });
  });

  it('extracts canonical execution groups from execution pages', () => {
    const turns = [{
      turnId: 'turn-1',
      status: 'completed',
      execution: {
        pages: [{
          pageId: 'page-1',
          assistantMessageId: 'page-1',
          parentMessageId: 'parent-1',
          iteration: 2,
          narration: 'Checking metrics.',
          content: 'Done.',
          status: 'completed',
          finalResponse: true,
          modelSteps: [],
          toolSteps: []
        }]
      }
    }];

    expect(canonicalExecutionPages(turns[0])).toHaveLength(1);
    expect(extractCanonicalExecutionGroups(turns)).toEqual([
      expect.objectContaining({
        turnId: 'turn-1',
        turnStatus: 'completed',
        assistantMessageId: 'page-1',
        parentMessageId: 'parent-1',
        iteration: 2,
        narration: 'Checking metrics.',
        content: 'Done.',
        status: 'completed',
        finalResponse: true,
      })
    ]);
  });
});
