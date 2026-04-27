import { describe, expect, it } from 'vitest';

import {
  describeTimelineEvent,
  isPresentableGroup,
  mergeLatestTranscriptAndLiveGroups,
  normalizeModelStep,
  normalizeToolStep,
  plannedToolCalls
} from './ExecutionWorkspace';

describe('ExecutionWorkspace helpers', () => {
  it('reads planned tool calls from canonical execution groups', () => {
    const calls = plannedToolCalls({
      toolCallsPlanned: [
        { toolCallId: 'tc1', toolName: 'llm/agents/run' }
      ]
    });

    expect(calls).toHaveLength(1);
    expect(calls[0]).toMatchObject({
      toolCallId: 'tc1',
      toolName: 'llm/agents/run'
    });
  });

  it('treats planned tool calls as presentable latest-page content', () => {
    expect(isPresentableGroup({
      narration: '',
      toolCalls: [],
      toolCallsPlanned: [{ toolName: 'llm/agents/run' }],
      finalResponse: false,
      content: ''
    })).toBe(true);
  });

  it('keeps the latest visible page on the newest presentable group when a newer model-only page is not presentable', () => {
    const transcriptGroups = [
      {
        assistantMessageId: 'm1',
        sequence: 1,
        narration: 'Using llm/agents/run.',
        toolCalls: [{ toolName: 'llm/agents/run' }],
        toolCallsPlanned: [],
        finalResponse: false,
        content: ''
      },
      {
        assistantMessageId: 'm2',
        sequence: 2,
        narration: '',
        toolCalls: [],
        toolCallsPlanned: [],
        finalResponse: false,
        content: ''
      }
    ];

    const visible = mergeLatestTranscriptAndLiveGroups(transcriptGroups, {}, '1');

    expect(visible).toHaveLength(1);
    expect(visible[0].assistantMessageId).toBe('m1');
  });

  it('includes planned tool names in timeline event descriptions', () => {
    const text = describeTimelineEvent({
      type: 'model_completed',
      status: 'thinking',
      toolCallsPlanned: [
        { toolName: 'llm/agents/run' },
        { toolName: 'system/exec/run' }
      ]
    });

    expect(text).toContain('planned llm/agents/run, system/exec/run');
  });

  it('projects streaming model content into the stream payload for details', () => {
    const step = normalizeModelStep({
      assistantMessageId: 'a1',
      status: 'streaming',
      narration: 'Thinking...',
      content: 'partial streamed text',
      finalResponse: false,
      modelSteps: [{
        provider: 'openai',
        model: 'gpt-5.4',
        status: 'streaming'
      }]
    });

    expect(step.status).toBe('streaming');
    expect(step.streamPayload).toEqual({
      status: 'streaming',
      content: 'partial streamed text',
      narration: 'Thinking...'
    });
  });

  it('projects final model content into the response payload when no response payload object exists', () => {
    const step = normalizeModelStep({
      assistantMessageId: 'a1',
      status: 'succeeded',
      content: 'Final answer',
      finalResponse: true,
      modelSteps: [{
        provider: 'openai',
        model: 'gpt-5.4',
        status: 'succeeded'
      }]
    });

    expect(step.responsePayload).toBe('Final answer');
    expect(step.streamPayload).toBeNull();
  });

  it('prefers modelCallId and toolCallId as execution workspace step ids', () => {
    const modelStep = normalizeModelStep({
      assistantMessageId: 'msg-1',
      modelSteps: [{
        modelCallId: 'mc-1',
        assistantMessageId: 'msg-1',
        provider: 'openai',
        model: 'gpt-5.4',
        status: 'completed'
      }]
    });
    const toolStep = normalizeToolStep({
      toolCallId: 'tc-1',
      toolMessageId: 'tm-1',
      toolName: 'resources-list',
      status: 'completed'
    });

    expect(modelStep.id).toBe('mc-1');
    expect(toolStep.id).toBe('tc-1');
  });
});
