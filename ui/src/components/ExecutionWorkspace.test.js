import { describe, expect, it } from 'vitest';

import {
  describeTimelineEvent,
  isPresentableGroup,
  mergeLatestTranscriptAndLiveGroups,
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
      preamble: '',
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
        preamble: 'Using llm/agents/run.',
        toolCalls: [{ toolName: 'llm/agents/run' }],
        toolCallsPlanned: [],
        finalResponse: false,
        content: ''
      },
      {
        assistantMessageId: 'm2',
        sequence: 2,
        preamble: '',
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
});
