import { describe, expect, it } from 'vitest';

import {
  applyElicitationRequestedEvent,
  applyExecutionStreamEvent,
  applyAssistantFinalEvent,
  applyMessagePatchEvent,
  applyPreambleEvent,
  applyToolStreamEvent,
  applyTurnStartedEvent,
  applyStreamChunk,
  finalizeStreamTurn,
  normalizeStreamingMarkdown
} from './liveStreamStore';
import { markLiveOwnedTurn } from './liveStreamStore';

describe('normalizeStreamingMarkdown', () => {
  it('strips leading and trailing markdown fences from streamed content', () => {
    expect(normalizeStreamingMarkdown("```markdown\nhello\n```")).toMatchObject({
      content: 'hello',
      hadLeadingFence: true,
      hadTrailingFence: true,
      language: 'markdown',
    });
  });

  it('keeps plain text unchanged when no fences are present', () => {
    expect(normalizeStreamingMarkdown('hello')).toMatchObject({
      content: 'hello',
      hadLeadingFence: false,
      hadTrailingFence: false,
      language: '',
    });
  });
});

describe('applyStreamChunk', () => {
  it('updates the active execution group content while streaming', () => {
    const chatState = {
      activeStreamTurnId: 'turn-1',
      liveRows: [{
        id: 'assistant:turn-1:1',
        role: 'assistant',
        turnId: 'turn-1',
        interim: 1,
        isStreaming: true,
        content: '',
        executionGroups: [{
          assistantMessageId: 'msg-1',
          content: '',
          finalResponse: false,
          status: 'thinking',
          modelSteps: [{
            modelCallId: 'msg-1',
            assistantMessageId: 'msg-1',
            status: 'thinking'
          }],
          toolSteps: [],
          toolCallsPlanned: []
        }],
        createdAt: '2026-03-16T10:00:01Z'
      }]
    };

    applyStreamChunk(chatState, {
      id: 'msg-1',
      streamId: 'conv-1',
      content: 'Hello'
    }, 'conv-1');

    expect(chatState.liveRows[0].content).toBe('Hello');
    expect(chatState.liveRows[0].executionGroups[0].content).toBe('Hello');
    expect(chatState.liveRows[0].executionGroups[0].status).toBe('streaming');
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].status).toBe('streaming');
  });

  it('creates a streaming execution group when text arrives before model_started', () => {
    const chatState = {
      activeStreamTurnId: 'turn-1',
      liveRows: []
    };

    applyStreamChunk(chatState, {
      id: 'msg-1',
      streamId: 'conv-1',
      content: 'Hello'
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups[0].content).toBe('Hello');
    expect(chatState.liveRows[0].executionGroups[0].status).toBe('streaming');
    expect(chatState.liveRows[0].createdAt).toBe('');
  });

  it('matches streamed chunks by canonical messageId when assistantMessageId is absent', () => {
    const chatState = {
      activeStreamTurnId: 'turn-1',
      liveRows: [{
        id: 'assistant:turn-1:1',
        role: 'assistant',
        turnId: 'turn-1',
        interim: 1,
        isStreaming: true,
        content: '',
        executionGroups: [{
          assistantMessageId: 'msg-1',
          content: '',
          finalResponse: false,
          status: 'thinking',
          modelSteps: [{
            modelCallId: 'msg-1',
            assistantMessageId: 'msg-1',
            status: 'thinking'
          }],
          toolSteps: [],
          toolCallsPlanned: []
        }],
        createdAt: '2026-03-16T10:00:01Z'
      }]
    };

    applyStreamChunk(chatState, {
      messageId: 'msg-1',
      streamId: 'conv-1',
      content: 'Hello'
    }, 'conv-1');

    expect(chatState.liveRows[0].content).toBe('Hello');
    expect(chatState.liveRows[0].executionGroups[0].content).toBe('Hello');
  });
});

describe('applyExecutionStreamEvent', () => {
  it('records turn_started as an execution-details lifecycle entry even before model_started', () => {
    const chatState = { liveRows: [], activeStreamPrompt: 'hi' };

    applyTurnStartedEvent(chatState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      status: 'running'
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(2);
    expect(chatState.liveRows[0]).toMatchObject({
      id: 'user:turn-1',
      role: 'user',
      turnId: 'turn-1',
      content: 'hi'
    });
    expect(chatState.liveRows[1]).toMatchObject({
      id: 'turn:turn-1',
      role: 'assistant',
      turnId: 'turn-1',
      status: 'running',
      turnStatus: 'running',
    });
    expect(chatState.liveRows[1].executionGroups[0].toolSteps[0]).toMatchObject({
      kind: 'turn',
      reason: 'turn_started',
      toolName: 'turn_started',
      status: 'running',
    });
  });

  it('reuses an existing user row for the same turn instead of adding a synthetic duplicate', () => {
    const chatState = {
      liveRows: [
        {
          id: 'msg-user-1',
          role: 'user',
          turnId: 'turn-1',
          conversationId: 'conv-1',
          createdAt: '2026-03-16T10:00:00Z',
          content: 'hi',
          rawContent: 'hi'
        }
      ],
      activeStreamPrompt: 'hi'
    };

    applyTurnStartedEvent(chatState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      status: 'running',
      createdAt: '2026-03-16T10:00:00Z'
    }, 'conv-1');

    expect(chatState.liveRows.filter((row) => String(row?.role || '').toLowerCase() === 'user')).toHaveLength(1);
    expect(chatState.liveRows[0].id).toBe('msg-user-1');
    expect(chatState.liveRows[0].content).toBe('hi');
  });

  it('creates a synthetic user row from activeStreamPrompt when turn_started lacks user ids', () => {
    const chatState = {
      liveRows: [],
      activeStreamPrompt: 'check order pacing and deliver 2660140, troubleshoot it'
    };

    applyTurnStartedEvent(chatState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      status: 'running',
      createdAt: '2026-03-16T10:00:00Z'
    }, 'conv-1');

    expect(chatState.liveRows.map((row) => row.id)).toEqual(['user:turn-1', 'turn:turn-1']);
    expect(chatState.liveRows[0]).toMatchObject({
      role: 'user',
      turnId: 'turn-1',
      content: 'check order pacing and deliver 2660140, troubleshoot it'
    });
  });

  it('keeps execution row timestamps deterministic when events omit createdAt', () => {
    const chatState = { liveRows: [] };

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].createdAt).toBe('');
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].startedAt).toBeUndefined();
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].completedAt).toBeUndefined();
  });

  it('keeps the turn-level row running when an intermediate model step completes', () => {
    const chatState = { liveRows: [] };

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'streaming',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'completed',
      createdAt: '2026-03-16T10:00:04Z',
      responsePayloadId: 'resp-1'
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].status).toBe('streaming');
    expect(chatState.liveRows[0].turnStatus).toBe('streaming');
    expect(chatState.liveRows[0].executionGroups[0].status).toBe('completed');
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].status).toBe('completed');
  });

  it('preserves canonical thinking status from model_started rows', () => {
    const chatState = { liveRows: [] };

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      type: 'model_started',
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.4' }
    }, 'conv-1');

    expect(chatState.liveRows[0].status).toBe('thinking');
    expect(chatState.liveRows[0].turnStatus).toBe('thinking');
  });

  it('keeps row startedAt when model_started merges into an existing turn row', () => {
    const chatState = { liveRows: [], activeStreamPrompt: 'Recommend sitelists for audience 7180287' };

    applyTurnStartedEvent(chatState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      status: 'running',
      createdAt: '2026-03-16T10:00:00Z'
    }, 'conv-1');

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      type: 'model_started',
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.4' }
    }, 'conv-1');

    const assistantRow = chatState.liveRows.find((row) => row.role === 'assistant');
    expect(assistantRow.id).toBe('mc-1');
    expect(assistantRow.startedAt).toBe('2026-03-16T10:00:00Z');
    expect(assistantRow.executionGroups[0].startedAt).toBe('2026-03-16T10:00:00Z');
    const modelGroup = assistantRow.executionGroups.find((group) => Array.isArray(group?.modelSteps) && group.modelSteps.length > 0);
    expect(modelGroup?.modelSteps?.[0]?.startedAt).toBe('2026-03-16T10:00:01Z');
    expect(assistantRow.status).toBe('thinking');
  });

  it('preserves turn agent identity on execution rows', () => {
    const chatState = { liveRows: [] };

    applyExecutionStreamEvent(chatState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      agentIdUsed: 'steward-performance',
      agentName: 'Steward-performance-Analyzer',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0]).toMatchObject({
      turnId: 'turn-1',
      agentIdUsed: 'steward-performance',
      agentName: 'Steward-performance-Analyzer'
    });
  });

  it('does not overwrite the user row when model_started uses the turn id', () => {
    const chatState = {
      liveRows: [
        {
          id: 'turn-1',
          role: 'user',
          turnId: 'turn-1',
          createdAt: '2026-03-16T10:00:00Z',
          content: 'hi'
        }
      ]
    };

    applyExecutionStreamEvent(chatState, {
      id: 'turn-1',
      type: 'model_started',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(2);
    expect(chatState.liveRows[0]).toMatchObject({
      id: 'turn-1',
      role: 'user',
      content: 'hi'
    });
    expect(chatState.liveRows[1]).toMatchObject({
      id: 'assistant:turn-1:1',
      role: 'assistant',
      turnId: 'turn-1'
    });
  });

  it('consolidates multiple execution pages into one row per turn', () => {
    // Bug: 4 execution detail blocks appeared instead of 1 row per turn.
    // Root cause: matching by assistantMessageId created separate rows per page.
    // Fix: match by turnId + role=assistant so all pages merge into one row.
    const chatState = { liveRows: [] };

    // Page 1: model_started (preamble)
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'msg-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      pageIndex: 1,
      pageCount: 1,
      status: 'thinking',
      preamble: 'Thinking…',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-4o' }
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);

    // Page 2: different assistantMessageId, same turn — tool call iteration
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'msg-2',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 2,
      pageIndex: 2,
      pageCount: 2,
      status: 'thinking',
      preamble: 'Running tool…',
      createdAt: '2026-03-16T10:00:03Z',
      model: { provider: 'openai', model: 'gpt-4o' }
    }, 'conv-1');

    // Should still be 1 row, with 2 execution groups
    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups).toHaveLength(2);
    expect(chatState.liveRows[0].executionGroups[0].assistantMessageId).toBe('msg-1');
    expect(chatState.liveRows[0].executionGroups[1].assistantMessageId).toBe('msg-2');
  });

  it('assistant_final updates content without creating a second execution group', () => {
    // Bug: assistant_final has a different assistantMessageId than model_started,
    // so applyExecutionStreamEvent created a second execution group with empty
    // model info — showing "Execution details (2)" with a phantom model entry.
    // Fix: use applyAssistantFinalEvent which updates row content and the last
    // execution group's content/finalResponse without adding new groups.
    const chatState = { liveRows: [] };

    // Step 1: model_started creates execution row with model info
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'model-call-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      preamble: 'Thinking about your request…',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups).toHaveLength(1);
    expect(chatState.liveRows[0].content).toBe('Thinking about your request…');

    // Step 2: assistant_final arrives with DIFFERENT assistantMessageId
    applyAssistantFinalEvent(chatState, {
      assistantMessageId: 'assistant-msg-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      content: '{"HOME": "/Users/awitas"}',
      finalResponse: true,
      createdAt: '2026-03-16T10:00:05Z'
    });

    // Must still have exactly 1 execution group — no phantom entry
    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups).toHaveLength(1);
    // Row content must be the final response
    expect(chatState.liveRows[0].content).toBe('{"HOME": "/Users/awitas"}');
    expect(chatState.liveRows[0].interim).toBe(1);
    // The existing execution group gets the final content
    expect(chatState.liveRows[0].executionGroups[0].content).toBe('{"HOME": "/Users/awitas"}');
    expect(chatState.liveRows[0].executionGroups[0].finalResponse).toBe(true);
    // Model info from model_started is preserved
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].provider).toBe('openai');
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].model).toBe('gpt-5.2');
  });

  it('does not create phantom execution group from model_completed without model info', () => {
    // Bug: "Execution details (3)" for a 2-iteration turn.
    // The conv service emits model_completed with same assistantMessageId but
    // no model info. mergeCanonicalExecutionGroups overwrites the modelSteps
    // from model_started with empty data from model_completed.
    // Additionally, if the model_completed for the final iteration has
    // status "succeeded", it must merge into the existing group, not create
    // a phantom third entry.
    const chatState = { liveRows: [] };

    // 1. model_started iter 1 (conv service, has model info)
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      pageIndex: 1,
      pageCount: 1,
      status: 'thinking',
      preamble: 'Let me check…',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' },
      requestPayloadId: 'req-1'
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups).toHaveLength(1);

    // 2. model_completed iter 1 from conv service (same assistantMessageId, NO model info)
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'completed',
      createdAt: '2026-03-16T10:00:05Z',
      responsePayloadId: 'resp-1'
      // Note: no model field
    }, 'conv-1');

    // Should merge into group 1, preserving model info
    expect(chatState.liveRows[0].executionGroups).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].provider).toBe('openai');
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].model).toBe('gpt-5.2');

    // 3. model_started iter 2 (conv service, has model info)
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-2',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 2,
      pageIndex: 2,
      pageCount: 2,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:06Z',
      model: { provider: 'openai', model: 'gpt-5.2' },
      requestPayloadId: 'req-2'
    }, 'conv-1');

    expect(chatState.liveRows[0].executionGroups).toHaveLength(2);

    // 4. model_completed iter 2 from conv service (same assistantMessageId, NO model info)
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-2',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 2,
      status: 'succeeded',
      createdAt: '2026-03-16T10:00:10Z',
      responsePayloadId: 'resp-2'
      // Note: no model field
    }, 'conv-1');

    // Must still be 2 groups, NOT 3
    expect(chatState.liveRows[0].executionGroups).toHaveLength(2);
    // Model info from model_started must be preserved
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].provider).toBe('openai');
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].model).toBe('gpt-5.2');
    expect(chatState.liveRows[0].executionGroups[1].modelSteps[0].provider).toBe('openai');
    expect(chatState.liveRows[0].executionGroups[1].modelSteps[0].model).toBe('gpt-5.2');
    // Payload IDs from model_completed must be added
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].responsePayloadId).toBe('resp-1');
    expect(chatState.liveRows[0].executionGroups[1].modelSteps[0].responsePayloadId).toBe('resp-2');
  });

  it('tool_calls_planned creates preliminary tool steps immediately', () => {
    // Bug: tool call line missing in execution details during streaming.
    // Root cause: tool_calls_planned stored planned tools in toolCallsPlanned
    // array, but the UI renders toolSteps as timeline entries.
    // Fix: tool_calls_planned should also create preliminary toolSteps with
    // status "planned" so they render immediately. Later tool_call_started/
    // tool_call_completed events merge by toolCallId to update status.
    const chatState = { liveRows: [] };

    // 1. model_started iter 1
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      preamble: 'Let me check…',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    // 2. tool_calls_planned arrives with planned tool calls
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'tool_calls',
      content: "I'm going to retrieve the HOME env var.",
      toolCallsPlanned: [
        { toolCallId: 'call-1', toolName: 'system_os/getEnv' }
      ],
      createdAt: '2026-03-16T10:00:02Z'
    }, 'conv-1');

    // The execution group should have BOTH toolCallsPlanned AND toolSteps
    const group = chatState.liveRows[0].executionGroups[0];
    expect(group.toolCallsPlanned).toHaveLength(1);
    // Preliminary tool steps must exist for immediate rendering
    expect(group.toolSteps).toHaveLength(1);
    expect(group.toolSteps[0]).toMatchObject({
      toolCallId: 'call-1',
      toolName: 'system_os/getEnv',
      status: 'planned'
    });

    // 3. tool_call_started arrives (from conv service DB patch)
    applyToolStreamEvent(chatState, {
      type: 'tool_call_started',
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      toolCallId: 'call-1',
      toolMessageId: 'tool-msg-1',
      toolName: 'system_os/getEnv',
      status: 'running',
      createdAt: '2026-03-16T10:00:03Z'
    }, 'conv-1');

    // Should merge, not duplicate — still 1 tool step, now "running"
    expect(chatState.liveRows[0].executionGroups[0].toolSteps).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups[0].toolSteps[0].status).toBe('running');
  });

  it('tool_call events add tool steps when row id differs from assistantMessageId', () => {
    // Bug: tool call line missing in execution details.
    // Root cause: applyToolStreamEventToRows finds the row by exact
    // row.id === assistantMessageId. But after turn-based consolidation,
    // the row's id is from the FIRST event (e.g., model_started without
    // assistantMessageId → id = "assistant:turn-1:1"). Later tool_call
    // events have a different assistantMessageId (e.g., "mc-1"), so the
    // row lookup fails silently.
    // Fix: find the row by turnId + role=assistant, not by exact id.
    const chatState = { liveRows: [] };

    // 1. model_started creates row — id becomes "assistant:turn-1:1"
    //    (no assistantMessageId in this event)
    applyExecutionStreamEvent(chatState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      preamble: 'Let me check…',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    // Row id is generated, NOT 'mc-1'
    expect(chatState.liveRows[0].id).toBe('assistant:turn-1:1');

    // 2. tool_call_started with assistantMessageId 'mc-1' — different from row id
    applyToolStreamEvent(chatState, {
      type: 'tool_call_started',
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      toolCallId: 'call-1',
      toolMessageId: 'tool-msg-1',
      toolName: 'system_os/getEnv',
      status: 'running',
      createdAt: '2026-03-16T10:00:02Z'
    }, 'conv-1');

    // Must find the row by turn, add tool step
    expect(chatState.liveRows).toHaveLength(1);
    const groups = chatState.liveRows[0].executionGroups;
    const allToolSteps = groups.flatMap((g) => g.toolSteps || []);
    expect(allToolSteps).toHaveLength(1);
    expect(allToolSteps[0].toolName).toBe('system_os/getEnv');
    expect(allToolSteps[0].toolCallId).toBe('call-1');
  });

  it('finalizeStreamTurn propagates _streamContent to content when payload has no content', () => {
    // When turn_completed arrives with no content, finalizeStreamTurn should
    // use the accumulated _streamContent from text_delta events.
    const chatState = {
      liveRows: [
        {
          id: 'assistant:turn-1:1',
          role: 'assistant',
          turnId: 'turn-1',
          createdAt: '2026-03-16T10:00:01Z',
          interim: 1,
          content: 'Thinking…',
          _streamContent: 'Here is the final response.',
          preamble: 'Thinking…',
          status: 'thinking',
          isStreaming: true,
          executionGroups: [{
            assistantMessageId: 'msg-1',
            preamble: 'Thinking…',
            content: '',
            finalResponse: false,
            status: 'thinking',
            modelSteps: [],
            toolSteps: [],
            toolCallsPlanned: []
          }]
        }
      ],
      activeStreamTurnId: 'turn-1',
      activeStreamStartedAt: Date.now(),
      activeStreamPrompt: 'test'
    };

    finalizeStreamTurn(chatState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      status: 'completed'
    }, 'conv-1');

  expect(chatState.liveRows).toHaveLength(1);
  expect(chatState.liveRows[0].content).toBe('Here is the final response.');
  expect(chatState.liveRows[0].interim).toBe(0);
  expect(chatState.liveRows[0].isStreaming).toBe(false);
  expect(chatState.liveRows[0].turnStatus).toBe('completed');
  expect(chatState.liveRows[0].executionGroups[1].content).toBe('Here is the final response.');
  });

  it('finalizeStreamTurn keeps model step completedAt deterministic when the terminal payload omits timestamps', () => {
    const chatState = {
      liveRows: [
        {
          id: 'assistant:turn-1:1',
          role: 'assistant',
          turnId: 'turn-1',
          createdAt: '2026-03-16T10:00:01Z',
          interim: 1,
          content: 'Thinking…',
          _streamContent: 'Final answer.',
          status: 'thinking',
          isStreaming: true,
          executionGroups: [{
            assistantMessageId: 'msg-1',
            preamble: 'Thinking…',
            content: '',
            finalResponse: false,
            status: 'thinking',
            modelSteps: [{
              modelCallId: 'msg-1',
              startedAt: '2026-03-16T10:00:01Z',
              status: 'thinking'
            }],
            toolSteps: [],
            toolCallsPlanned: []
          }]
        }
      ],
      activeStreamTurnId: 'turn-1',
      activeStreamPrompt: 'test'
    };

    finalizeStreamTurn(chatState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      status: 'completed'
    }, 'conv-1');

    expect(chatState.liveRows[0].executionGroups[1].modelSteps[0]).toMatchObject({
      startedAt: '2026-03-16T10:00:01Z',
      status: 'completed'
    });
    expect(chatState.liveRows[0].executionGroups[1].modelSteps[0].completedAt).toBeUndefined();
  });

  it('suppresses mode=summary message_patch rows from live chat state', () => {
    const chatState = { liveRows: [] };

    applyMessagePatchEvent(chatState, {
      id: 'summary-msg-1',
      patch: {
        role: 'assistant',
        mode: 'summary',
        turnId: 'turn-1',
        createdAt: '2026-03-16T10:00:02Z',
        content: 'Title: Campaign summary'
      }
    });

    expect(chatState.liveRows).toEqual([]);
  });

  it('creates deterministic preamble and elicitation rows when payload timestamps are missing', () => {
    const preambleState = { liveRows: [] };
    applyPreambleEvent(preambleState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      assistantMessageId: 'msg-1',
      content: 'Calling updatePlan.'
    }, 'conv-1');

    expect(preambleState.liveRows).toHaveLength(1);
    expect(preambleState.liveRows[0].createdAt).toBe('');

    const elicitationState = { liveRows: [] };
    applyElicitationRequestedEvent(elicitationState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      assistantMessageId: 'msg-2',
      elicitationId: 'elic-1',
      content: 'Need input',
      elicitationData: { requestedSchema: { type: 'object' } }
    });

    expect(elicitationState.liveRows).toHaveLength(1);
    expect(elicitationState.liveRows[0].createdAt).toBe('');
  });

  it('ignores later model and patch events once a summary message id has been identified', () => {
    const chatState = {
      liveRows: [{
        id: 'msg-main',
        role: 'assistant',
        turnId: 'turn-1',
        content: 'Hi! How can I help with your campaigns today?',
        interim: 0,
        executionGroups: [{
          assistantMessageId: 'msg-main',
          content: 'Hi! How can I help with your campaigns today?',
          finalResponse: true,
          status: 'completed',
          modelSteps: [{
            modelCallId: 'msg-main',
            status: 'completed',
            provider: 'openai',
            model: 'gpt-5.4'
          }],
          toolSteps: [],
          toolCallsPlanned: []
        }],
        createdAt: '2026-03-16T10:00:01Z'
      }]
    };

    applyMessagePatchEvent(chatState, {
      id: 'summary-msg-1',
      patch: {
        role: 'assistant',
        mode: 'summary',
        turnId: 'turn-1',
        interim: 1,
        createdAt: '2026-03-16T10:00:02Z'
      }
    });

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'summary-msg-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      status: 'thinking',
      createdAt: '2026-03-16T10:00:03Z',
      model: { provider: 'openai', model: 'gpt-5.4' }
    }, 'conv-1');

    applyMessagePatchEvent(chatState, {
      id: 'summary-msg-1',
      patch: {
        turnId: 'turn-1',
        interim: 0,
        createdAt: '2026-03-16T10:00:04Z',
        content: 'Title: Initial Greeting'
      }
    });

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].content).toBe('Hi! How can I help with your campaigns today?');
    expect(chatState.liveRows[0].executionGroups).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups[0].assistantMessageId).toBe('msg-main');
  });

  it('message_patch merges into existing execution row for the same turn', () => {
    // Bug: message_patch creates a SEPARATE assistant row from the execution row
    // because mergeRowSnapshots matches by id only. This causes:
    // 1. Preamble bubble stays even after assistant_final updates the execution row
    // 2. Tool steps on the execution row don't appear in rendering
    // Fix: applyMessagePatchToRows should find existing assistant rows by turnId.
    const chatState = { liveRows: [] };

    // 1. model_started creates execution row
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      preamble: 'Let me check…',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].id).toBe('mc-1');
    expect(chatState.liveRows[0].executionGroups).toHaveLength(1);

    // 2. message_patch with DIFFERENT id for same turn
    applyMessagePatchEvent(chatState, {
      id: 'msg-456',
      patch: {
        role: 'assistant',
        turnId: 'turn-1',
        content: 'I am going to look up HOME env var.',
        interim: 1,
        createdAt: '2026-03-16T10:00:02Z'
      }
    });

    // Must merge into the EXISTING execution row, not create a second row
    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].content).toBe('I am going to look up HOME env var.');
    // Execution groups must be preserved
    expect(chatState.liveRows[0].executionGroups).toHaveLength(1);

    // 3. assistant_final updates the same row
    applyAssistantFinalEvent(chatState, {
      assistantMessageId: 'msg-456',
      turnId: 'turn-1',
      content: '{"HOME": "/Users/awitas"}',
      finalResponse: true
    });

  // Still 1 row, content replaced
  expect(chatState.liveRows).toHaveLength(1);
  expect(chatState.liveRows[0].content).toBe('{"HOME": "/Users/awitas"}');
  expect(chatState.liveRows[0].interim).toBe(1);
  });

  it('handles 3 parallel tool calls in a single iteration', () => {
    const chatState = { liveRows: [] };

    // model_started iter 1
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      preamble: 'I will check 3 things…',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    // tool_calls_planned with 3 tool calls
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'tool_calls',
      toolCallsPlanned: [
        { toolCallId: 'call-1', toolName: 'system_os/getEnv' },
        { toolCallId: 'call-2', toolName: 'system_os/exec' },
        { toolCallId: 'call-3', toolName: 'system_fs/readFile' }
      ],
      createdAt: '2026-03-16T10:00:02Z'
    }, 'conv-1');

    // 3 preliminary tool steps
    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups[0].toolSteps).toHaveLength(3);
    expect(chatState.liveRows[0].executionGroups[0].toolSteps.map((s) => s.toolName))
      .toEqual(['system_os/getEnv', 'system_os/exec', 'system_fs/readFile']);
    expect(chatState.liveRows[0].executionGroups[0].toolSteps.every((s) => s.status === 'planned')).toBe(true);

    // tool_call_started for all 3
    for (const [i, name] of ['system_os/getEnv', 'system_os/exec', 'system_fs/readFile'].entries()) {
      applyToolStreamEvent(chatState, {
        type: 'tool_call_started',
        assistantMessageId: 'mc-1',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        toolCallId: `call-${i + 1}`,
        toolMessageId: `tool-msg-${i + 1}`,
        toolName: name,
        status: 'running',
        createdAt: `2026-03-16T10:00:0${3 + i}Z`
      }, 'conv-1');
    }

    // Still 3 tool steps, all running now
    expect(chatState.liveRows[0].executionGroups[0].toolSteps).toHaveLength(3);
    expect(chatState.liveRows[0].executionGroups[0].toolSteps.every((s) => s.status === 'running')).toBe(true);

    // tool_call_completed for all 3
    for (const [i, name] of ['system_os/getEnv', 'system_os/exec', 'system_fs/readFile'].entries()) {
      applyToolStreamEvent(chatState, {
        type: 'tool_call_completed',
        assistantMessageId: 'mc-1',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        toolCallId: `call-${i + 1}`,
        toolMessageId: `tool-msg-${i + 1}`,
        toolName: name,
        status: 'completed',
        responsePayloadId: `resp-tool-${i + 1}`,
        createdAt: `2026-03-16T10:00:0${6 + i}Z`
      }, 'conv-1');
    }

    // Still 3 tool steps, all completed with payload IDs
    const steps = chatState.liveRows[0].executionGroups[0].toolSteps;
    expect(steps).toHaveLength(3);
    expect(steps.every((s) => s.status === 'completed')).toBe(true);
    expect(steps.map((s) => s.responsePayloadId)).toEqual(['resp-tool-1', 'resp-tool-2', 'resp-tool-3']);

    // model_started iter 2 (final response)
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-2',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 2,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:10Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    // 1 row, 2 execution groups
    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups).toHaveLength(2);
    // First group: 1 model step + 3 tool steps
    expect(chatState.liveRows[0].executionGroups[0].modelSteps).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups[0].toolSteps).toHaveLength(3);

    // assistant_final
    applyAssistantFinalEvent(chatState, {
      assistantMessageId: 'msg-final',
      turnId: 'turn-1',
      content: 'Here are the results.',
      finalResponse: true
    });

  expect(chatState.liveRows[0].content).toBe('Here are the results.');
  expect(chatState.liveRows[0].interim).toBe(1);
  });

  it('handles tool call failure without breaking the row', () => {
    const chatState = { liveRows: [] };

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    // tool_call_started
    applyToolStreamEvent(chatState, {
      type: 'tool_call_started',
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      toolCallId: 'call-1',
      toolMessageId: 'tool-msg-1',
      toolName: 'system_os/exec',
      status: 'running',
      createdAt: '2026-03-16T10:00:02Z'
    }, 'conv-1');

    // tool_call_completed with FAILED status
    applyToolStreamEvent(chatState, {
      type: 'tool_call_completed',
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      toolCallId: 'call-1',
      toolMessageId: 'tool-msg-1',
      toolName: 'system_os/exec',
      status: 'failed',
      createdAt: '2026-03-16T10:00:03Z'
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    const step = chatState.liveRows[0].executionGroups[0].toolSteps[0];
    expect(step.status).toBe('failed');
    expect(step.toolName).toBe('system_os/exec');

    // Turn can still get a final response after tool failure
    applyAssistantFinalEvent(chatState, {
      turnId: 'turn-1',
      content: 'The command failed. Let me try another approach.',
      finalResponse: true
    });

    expect(chatState.liveRows[0].content).toBe('The command failed. Let me try another approach.');
    expect(chatState.liveRows[0].interim).toBe(1);
    // Tool step preserved
    expect(chatState.liveRows[0].executionGroups[0].toolSteps[0].status).toBe('failed');
  });

  it('handles linked conversation tool call', () => {
    const chatState = { liveRows: [] };

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    // tool_call_completed with linked conversation
    applyToolStreamEvent(chatState, {
      type: 'tool_call_completed',
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      toolCallId: 'call-1',
      toolMessageId: 'tool-msg-1',
      toolName: 'llm/agents/run',
      status: 'completed',
      linkedConversationId: 'child-conv-1',
      responsePayloadId: 'resp-linked-1',
      createdAt: '2026-03-16T10:00:05Z'
    }, 'conv-1');

    const step = chatState.liveRows[0].executionGroups[0].toolSteps[0];
    expect(step.toolName).toBe('llm/agents/run');
    expect(step.linkedConversationId).toBe('child-conv-1');
    expect(step.responsePayloadId).toBe('resp-linked-1');
  });

  it('preserves tool_call_completed request and response payloads from SSE events', () => {
    const chatState = { liveRows: [] };

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    applyToolStreamEvent(chatState, {
      type: 'tool_call_completed',
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      toolCallId: 'call-1',
      toolMessageId: 'tool-msg-1',
      toolName: 'llm/agents:start',
      status: 'completed',
      arguments: { agentId: 'guardian', streaming: true },
      responsePayload: {
        conversationId: 'child-conv-1',
        status: 'completed',
        assistantResponse: 'Guardian started and returned first diagnostics.'
      },
      createdAt: '2026-03-16T10:00:05Z'
    }, 'conv-1');

    const step = chatState.liveRows[0].executionGroups[0].toolSteps[0];
    expect(step.requestPayload).toEqual({ agentId: 'guardian', streaming: true });
    expect(step.responsePayload).toEqual({
      conversationId: 'child-conv-1',
      status: 'completed',
      assistantResponse: 'Guardian started and returned first diagnostics.'
    });
  });

  it('carries phase through live model execution rows', () => {
    const chatState = { liveRows: [] };

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-intake-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 0,
      phase: 'intake',
      mode: 'router',
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    expect(chatState.liveRows[0].executionGroups[0].phase).toBe('intake');
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].phase).toBe('intake');
  });

  it('carries phase through live tool execution steps', () => {
    const chatState = { liveRows: [] };

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-sidecar-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    applyToolStreamEvent(chatState, {
      type: 'tool_call_completed',
      assistantMessageId: 'mc-sidecar-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      toolCallId: 'call-1',
      toolMessageId: 'tool-msg-1',
      toolName: 'llm/agents:start',
      phase: 'sidecar',
      status: 'completed',
      createdAt: '2026-03-16T10:00:05Z'
    }, 'conv-1');

    expect(chatState.liveRows[0].executionGroups[0].toolSteps[0].phase).toBe('sidecar');
  });

  it('keeps turn_started in a stable dedicated lifecycle group instead of attaching it to later model groups', () => {
    const chatState = { liveRows: [] };

    applyTurnStartedEvent(chatState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      status: 'running',
      createdAt: '2026-03-16T10:00:00Z'
    }, 'conv-1');

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'msg-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.4' }
    }, 'conv-1');

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'msg-2',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 2,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:02Z',
      model: { provider: 'openai', model: 'gpt-5.4' }
    }, 'conv-1');

    const groups = chatState.liveRows[0].executionGroups;
    expect(groups[0].pageId).toBe('turn:turn-1:lifecycle');
    expect(groups[0].toolSteps[0]).toMatchObject({
      kind: 'turn',
      reason: 'turn_started'
    });
    expect(groups.slice(1).every((group) =>
      !(Array.isArray(group?.toolSteps) && group.toolSteps.some((step) => String(step?.reason || '') === 'turn_started'))
    )).toBe(true);
  });

  it('handles multi-turn: second turn does not corrupt first turn', () => {
    const chatState = { liveRows: [] };

    // Turn 1: simple response
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-t1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    applyAssistantFinalEvent(chatState, {
      turnId: 'turn-1',
      content: 'Hi! How can I help?',
      finalResponse: true
    });

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].turnId).toBe('turn-1');
    expect(chatState.liveRows[0].content).toBe('Hi! How can I help?');

    // Turn 2: with tool call
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-t2',
      conversationId: 'conv-1',
      turnId: 'turn-2',
      iteration: 1,
      status: 'thinking',
      preamble: 'Let me check…',
      createdAt: '2026-03-16T10:01:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    applyToolStreamEvent(chatState, {
      type: 'tool_call_started',
      assistantMessageId: 'mc-t2',
      conversationId: 'conv-1',
      turnId: 'turn-2',
      toolCallId: 'call-t2',
      toolName: 'system_os/getEnv',
      status: 'running',
      createdAt: '2026-03-16T10:01:02Z'
    }, 'conv-1');

    // 2 separate rows — one per turn
    expect(chatState.liveRows).toHaveLength(2);
    // Turn 1 untouched
    expect(chatState.liveRows[0].turnId).toBe('turn-1');
    expect(chatState.liveRows[0].content).toBe('Hi! How can I help?');
    expect(chatState.liveRows[0].interim).toBe(1);
    // Turn 2 has tool step
    expect(chatState.liveRows[1].turnId).toBe('turn-2');
    expect(chatState.liveRows[1].executionGroups[0].toolSteps).toHaveLength(1);
    expect(chatState.liveRows[1].executionGroups[0].toolSteps[0].toolName).toBe('system_os/getEnv');
  });

  it('message_patch for user role does not merge into assistant row', () => {
    const chatState = { liveRows: [] };

    // Execution row for assistant
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    // message_patch for USER message (same turn)
    applyMessagePatchEvent(chatState, {
      id: 'user-msg-1',
      patch: {
        role: 'user',
        turnId: 'turn-1',
        content: 'show me HOME env var',
        rawContent: 'show me HOME env var',
        interim: 0,
        createdAt: '2026-03-16T10:00:00Z'
      }
    });

    // User row must be SEPARATE from assistant row
    expect(chatState.liveRows).toHaveLength(2);
    const userRow = chatState.liveRows.find((r) => r.role === 'user');
    const assistantRow = chatState.liveRows.find((r) => r.role === 'assistant');
    expect(userRow.content).toBe('show me HOME env var');
    expect(assistantRow.executionGroups).toHaveLength(1);
  });

  it('message_patch for user role merges into an existing synthetic user row for the same turn', () => {
    const chatState = {
      liveRows: [{
        id: 'user:turn-1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-03-16T10:00:00Z',
        content: 'Forecast inventory and uniques for this targeting set: deal 106171723',
        rawContent: 'Forecast inventory and uniques for this targeting set: deal 106171723',
        interim: 0,
        status: 'completed',
        turnStatus: 'running',
      }]
    };

    applyMessagePatchEvent(chatState, {
      id: 'turn-1',
      patch: {
        role: 'user',
        turnId: 'turn-1',
        content: 'Forecast inventory and uniques for this targeting set: deal 106171723',
        rawContent: 'Forecast inventory and uniques for this targeting set: deal 106171723',
        interim: 0,
        createdAt: '2026-03-16T10:00:00Z'
      }
    });

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].id).toBe('user:turn-1');
    expect(chatState.liveRows[0].role).toBe('user');
    expect(chatState.liveRows[0].content).toBe('Forecast inventory and uniques for this targeting set: deal 106171723');
  });

  it('message_patch keeps assistant createdAt deterministic when the patch omits timestamps', () => {
    const chatState = { liveRows: [] };

    applyMessagePatchEvent(chatState, {
      id: 'assistant-msg-1',
      patch: {
        role: 'assistant',
        turnId: 'turn-1',
        content: 'Calling updatePlan.',
        interim: 1
      }
    });

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].id).toBe('assistant-msg-1');
    expect(chatState.liveRows[0].createdAt).toBe('');
  });

  it('message_patch for user role keeps createdAt deterministic when the patch omits createdAt', () => {
    const chatState = { liveRows: [] };

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    applyMessagePatchEvent(chatState, {
      id: 'user-msg-1',
      patch: {
        role: 'user',
        turnId: 'turn-1',
        rawContent: 'Forecast inventory and uniques for this targeting set: deal 106171723'
      }
    });

    expect(chatState.liveRows).toHaveLength(2);
    const userRow = chatState.liveRows.find((row) => row?.role === 'user');
    expect(userRow).toMatchObject({
      role: 'user',
      turnId: 'turn-1',
      createdAt: ''
    });
  });

  it('full 2-iteration turn: preamble replaced by final, tool call visible', () => {
    // Reproduce the exact real-world event sequence for "show me HOME env var":
    // model_started → message_patch(preamble) → tool_calls_planned →
    // tool_call_started → tool_call_completed → model_started(iter2) →
    // message_patch(final) → assistant_final → model_completed(iter1) →
    // model_completed(iter2) → turn_completed
    const chatState = {
      liveRows: [],
      activeStreamTurnId: 'turn-1',
      runningTurnId: 'turn-1',
      activeStreamStartedAt: Date.now(),
      activeStreamPrompt: 'show me HOME env var'
    };

    // 1. model_started iter 1
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);

    // 2. message_patch for preamble (interim=1, different ID from model call)
    applyMessagePatchEvent(chatState, {
      id: 'msg-preamble',
      patch: {
        role: 'assistant',
        turnId: 'turn-1',
        content: "I'm going to use functions.system_os-getEnv tool.",
        preamble: "I'm going to use functions.system_os-getEnv tool.",
        interim: 1,
        createdAt: '2026-03-16T10:00:02Z'
      }
    });

    // Must merge into execution row, not create a separate row
    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].id).toBe('mc-1');
    expect(chatState.liveRows[0].content).toBe("I'm going to use functions.system_os-getEnv tool.");

    // 3. tool_calls_planned (from reactor)
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'tool_calls',
      toolCallsPlanned: [
        { toolCallId: 'call-1', toolName: 'system_os/getEnv' }
      ],
      createdAt: '2026-03-16T10:00:03Z'
    }, 'conv-1');

    expect(chatState.liveRows[0].executionGroups[0].toolSteps).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups[0].toolSteps[0].status).toBe('planned');

    // 4. tool_call_started
    applyToolStreamEvent(chatState, {
      type: 'tool_call_started',
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      toolCallId: 'call-1',
      toolMessageId: 'tool-msg-1',
      toolName: 'system_os/getEnv',
      status: 'running',
      createdAt: '2026-03-16T10:00:04Z'
    }, 'conv-1');

    expect(chatState.liveRows[0].executionGroups[0].toolSteps[0].status).toBe('running');

    // 5. tool_call_completed
    applyToolStreamEvent(chatState, {
      type: 'tool_call_completed',
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      toolCallId: 'call-1',
      toolMessageId: 'tool-msg-1',
      toolName: 'system_os/getEnv',
      status: 'completed',
      responsePayloadId: 'resp-tool-1',
      createdAt: '2026-03-16T10:00:05Z'
    }, 'conv-1');

    expect(chatState.liveRows[0].executionGroups[0].toolSteps[0].status).toBe('completed');

    // 6. model_started iter 2
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-2',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 2,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:06Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    expect(chatState.liveRows[0].executionGroups).toHaveLength(2);

    // 7. message_patch for FINAL assistant message (interim=0, different msg ID)
    applyMessagePatchEvent(chatState, {
      id: 'msg-final',
      patch: {
        role: 'assistant',
        turnId: 'turn-1',
        content: '```json\n{"HOME": "/Users/awitas"}\n```',
        interim: 0,
        createdAt: '2026-03-16T10:00:10Z'
      }
    });

    // Content must be replaced with final response.
    // interim stays 1 — only assistant_final/turn_completed sets it to 0.
    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].content).toBe('{"HOME": "/Users/awitas"}');

    // 8. assistant_final
    applyAssistantFinalEvent(chatState, {
      assistantMessageId: 'msg-final',
      turnId: 'turn-1',
      content: '```json\n{"HOME": "/Users/awitas"}\n```',
      finalResponse: true
    });

    expect(chatState.liveRows[0].content).toBe('{"HOME": "/Users/awitas"}');
    expect(chatState.liveRows[0].id).toBe('msg-final');
    expect(chatState.liveRows[0].interim).toBe(1);

    // 9. model_completed iter 1 (conv service, no model info)
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'completed',
      createdAt: '2026-03-16T10:00:11Z',
      responsePayloadId: 'resp-mc-1'
    }, 'conv-1');

    // 10. model_completed iter 2 with final content
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-2',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 2,
      status: 'completed',
      content: '```json\n{"HOME": "/Users/awitas"}\n```',
      finalResponse: true,
      createdAt: '2026-03-16T10:00:12Z',
      responsePayloadId: 'resp-mc-2'
    }, 'conv-1');

    // Final state assertions:
    // 1 row
    expect(chatState.liveRows).toHaveLength(1);
    // 2 execution groups (iter 1 + iter 2)
    expect(chatState.liveRows[0].executionGroups).toHaveLength(2);
    // Tool step visible on iter 1
    expect(chatState.liveRows[0].executionGroups[0].toolSteps).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups[0].toolSteps[0].toolName).toBe('system_os/getEnv');
  // Content is final response, NOT preamble
  expect(chatState.liveRows[0].content).toBe('{"HOME": "/Users/awitas"}');
  expect(chatState.liveRows[0].interim).toBe(0);
    // Model info preserved
    expect(chatState.liveRows[0].executionGroups[0].modelSteps[0].provider).toBe('openai');
    expect(chatState.liveRows[0].executionGroups[1].modelSteps[0].provider).toBe('openai');
  });

  it('text_delta appends to execution row instead of creating a separate stream row', () => {
    // Bug: applyStreamChunk creates a separate _type:'stream' row that renders
    // as a duplicate assistant bubble alongside the execution row.
    // Fix: text_delta should append content to the existing execution row.
    const chatState = {
      liveRows: [],
      activeStreamTurnId: 'turn-1',
      runningTurnId: 'turn-1'
    };

    // 1. model_started creates execution row
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-16T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);

    // 2. text_delta arrives
    applyStreamChunk(chatState, {
      id: 'mc-1',
      content: 'Hello',
      createdAt: '2026-03-16T10:00:02Z'
    }, 'conv-1');

    // Must NOT create a second row — should update the execution row
    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].role).toBe('assistant');
    expect(chatState.liveRows[0]._streamContent).toBe('Hello');

    // 3. More text_delta
    applyStreamChunk(chatState, {
      id: 'mc-1',
      content: ' world',
      createdAt: '2026-03-16T10:00:02Z'
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0]._streamContent).toBe('Hello world');

    // 4. assistant_final replaces content
    applyAssistantFinalEvent(chatState, {
      turnId: 'turn-1',
      content: 'Hello world!',
      finalResponse: true
    });

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].content).toBe('Hello world!');
    expect(chatState.liveRows[0].interim).toBe(1);
  });

  it('text_delta creates execution row when model_started has not arrived yet', () => {
    const chatState = {
      liveRows: [],
      activeStreamTurnId: 'turn-1',
      runningTurnId: 'turn-1'
    };

    // text_delta arrives before model_started
    applyStreamChunk(chatState, {
      id: 'mc-1',
      content: 'Hi',
      createdAt: '2026-03-16T10:00:01Z'
    }, 'conv-1');

    // Should create an assistant row (not a stream row)
    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].role).toBe('assistant');
    expect(chatState.liveRows[0]._streamContent).toBe('Hi');
    expect(chatState.liveRows[0]._type).not.toBe('stream');
  });

  it('does not overwrite the user row when finalizing a turn', () => {
    const chatState = {
      liveRows: [
        {
          id: 'turn-1',
          role: 'user',
          turnId: 'turn-1',
          createdAt: '2026-03-16T10:00:00Z',
          content: 'hi'
        },
        {
          id: 'assistant:turn-1:1',
          role: 'assistant',
          turnId: 'turn-1',
          createdAt: '2026-03-16T10:00:01Z',
          interim: 1,
          status: 'running',
          _streamContent: 'Hi!',
          executionGroups: [
            {
              assistantMessageId: 'assistant:turn-1:1',
              modelMessageId: 'assistant:turn-1:1',
              iteration: 1,
              status: 'running'
            }
          ]
        }
      ]
    };

    finalizeStreamTurn(chatState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      status: 'succeeded',
      content: 'Hi!'
    }, 'conv-1');

    expect(chatState.liveRows[0]).toMatchObject({
      id: 'turn-1',
      role: 'user',
      content: 'hi'
    });
    expect(chatState.liveRows[1]).toMatchObject({
      id: 'assistant:turn-1:1',
      role: 'assistant',
      content: 'Hi!',
      turnStatus: 'succeeded'
    });
  });
});

describe('markLiveOwnedTurn', () => {
  it('keeps only the latest owned turn and prunes older live rows', () => {
    const chatState = {
      liveOwnedConversationID: 'conv-1',
      liveOwnedTurnIds: ['turn-1'],
      liveRows: [
        { id: 'assistant-1', role: 'assistant', turnId: 'turn-1', content: 'old' },
        { id: 'assistant-2', role: 'assistant', turnId: 'turn-2', content: 'new' },
        { id: 'stream:1', _type: 'stream', role: 'assistant', content: 'stream' }
      ]
    };

    const owned = markLiveOwnedTurn(chatState, 'conv-1', 'turn-2');

    expect(owned).toEqual(['turn-2']);
    expect(chatState.liveOwnedTurnIds).toEqual(['turn-2']);
    expect(chatState.liveRows.map((row) => row.id)).toEqual(['assistant-2', 'stream:1']);
  });
});

describe('applyPreambleEvent', () => {
  it('updates the visible assistant bubble when preamble arrives after model_started', () => {
    const chatState = {
      liveRows: [{
        id: 'assistant:turn-1:1',
        role: 'assistant',
        turnId: 'turn-1',
        interim: 1,
        content: '',
        executionGroups: [{
          assistantMessageId: 'msg-1',
          status: 'thinking',
          modelSteps: [{
            modelCallId: 'msg-1',
            assistantMessageId: 'msg-1',
            status: 'thinking'
          }]
        }],
        createdAt: '2026-03-16T10:00:01Z'
      }]
    };

    applyPreambleEvent(chatState, {
      assistantMessageId: 'msg-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      content: 'Let me analyze this request…'
    }, 'conv-1');

    expect(chatState.liveRows[0].content).toBe('Let me analyze this request…');
    expect(chatState.liveRows[0].preamble).toBe('Let me analyze this request…');
    expect(chatState.liveRows[0].executionGroups[0].preamble).toBe('Let me analyze this request…');
  });

  it('replaces an older interim preamble with the latest one for the same live turn', () => {
    const chatState = {
      liveRows: [{
        id: 'assistant:turn-1:1',
        role: 'assistant',
        turnId: 'turn-1',
        interim: 1,
        content: 'Calling updatePlan.',
        preamble: 'Calling updatePlan.',
        executionGroups: [{
          assistantMessageId: 'msg-1',
          status: 'completed',
          preamble: 'Calling updatePlan.'
        }, {
          assistantMessageId: 'msg-2',
          status: 'thinking',
          preamble: ''
        }],
        createdAt: '2026-03-16T10:00:01Z'
      }]
    };

    applyPreambleEvent(chatState, {
      assistantMessageId: 'msg-2',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      content: 'Checking the hierarchy before forecasting.'
    }, 'conv-1');

    expect(chatState.liveRows[0].content).toBe('Checking the hierarchy before forecasting.');
    expect(chatState.liveRows[0].preamble).toBe('Checking the hierarchy before forecasting.');
  });
});

describe('applyPreambleEvent', () => {
  it('carries turn agent identity into a new assistant row before model_started arrives', () => {
    const chatState = { liveRows: [] };

    applyPreambleEvent(chatState, {
      conversationId: 'conv-1',
      turnId: 'turn-1',
      content: 'Thinking…',
      createdAt: '2026-03-16T10:00:01Z',
      agentIdUsed: 'coder',
      agentName: 'Coder'
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0]).toMatchObject({
      turnId: 'turn-1',
      agentIdUsed: 'coder',
      agentName: 'Coder'
    });
  });
});

describe('applyElicitationRequestedEvent', () => {
  it('attaches elicitation data to existing assistant row', () => {
    const chatState = { liveRows: [] };

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-17T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    applyElicitationRequestedEvent(chatState, {
      turnId: 'turn-1',
      assistantMessageId: 'mc-1',
      elicitationId: 'elic-1',
      content: 'Please confirm:',
      callbackUrl: '/v1/elicitations/elic-1/resolve',
      elicitationData: {
        requestedSchema: {
          type: 'object',
          properties: { confirm: { type: 'boolean' } }
        }
      }
    });

    expect(chatState.liveRows).toHaveLength(1);
    const row = chatState.liveRows[0];
    expect(row.elicitationId).toBe('elic-1');
    expect(row.elicitation).toMatchObject({
      elicitationId: 'elic-1',
      message: 'Please confirm:',
      callbackURL: '/v1/elicitations/elic-1/resolve'
    });
    expect(row.elicitation.requestedSchema).toMatchObject({
      type: 'object',
      properties: { confirm: { type: 'boolean' } }
    });
  });

  it('creates a new row when no assistant row exists for the turn', () => {
    const chatState = { liveRows: [] };

    applyElicitationRequestedEvent(chatState, {
      turnId: 'turn-1',
      assistantMessageId: 'msg-elic',
      elicitationId: 'elic-1',
      content: 'Pick one:',
      elicitationData: {
        requestedSchema: { type: 'string', enum: ['a', 'b'] }
      }
    });

    expect(chatState.liveRows).toHaveLength(1);
    const row = chatState.liveRows[0];
    expect(row.role).toBe('elicition');
    expect(row.turnId).toBe('turn-1');
    expect(row.elicitationId).toBe('elic-1');
    expect(row.elicitation.requestedSchema).toMatchObject({
      type: 'string',
      enum: ['a', 'b']
    });
  });

  it('returns early when elicitationId is missing', () => {
    const chatState = { liveRows: [] };
    const result = applyElicitationRequestedEvent(chatState, {
      turnId: 'turn-1',
      content: 'no id'
    });
    expect(result).toEqual([]);
    expect(chatState.liveRows).toHaveLength(0);
  });
});

describe('applyPreambleEvent', () => {
  it('sets preamble on existing assistant row and its last execution group', () => {
    const chatState = { liveRows: [] };

    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-03-17T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    applyPreambleEvent(chatState, {
      turnId: 'turn-1',
      assistantMessageId: 'mc-1',
      content: 'Let me analyze the code...'
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].preamble).toBe('Let me analyze the code...');
    expect(chatState.liveRows[0].executionGroups[0].preamble).toBe('Let me analyze the code...');
  });

  it('creates a new row when no assistant row exists', () => {
    const chatState = { liveRows: [] };

    applyPreambleEvent(chatState, {
      turnId: 'turn-1',
      assistantMessageId: 'mc-1',
      content: 'Thinking about it...',
      conversationId: 'conv-1',
      iteration: 1
    }, 'conv-1');

    expect(chatState.liveRows).toHaveLength(1);
    expect(chatState.liveRows[0].role).toBe('assistant');
    expect(chatState.liveRows[0].turnId).toBe('turn-1');
    expect(chatState.liveRows[0].preamble).toBe('Thinking about it...');
    expect(chatState.liveRows[0].executionGroups).toHaveLength(1);
    expect(chatState.liveRows[0].executionGroups[0].preamble).toBe('Thinking about it...');
  });

  it('returns early when content is empty', () => {
    const chatState = { liveRows: [] };
    const result = applyPreambleEvent(chatState, {
      turnId: 'turn-1',
      content: ''
    }, 'conv-1');
    expect(result).toEqual([]);
    expect(chatState.liveRows).toHaveLength(0);
  });

  it('updates preamble on the last execution group when multiple groups exist', () => {
    const chatState = { liveRows: [] };

    // Iteration 1
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      preamble: 'First iteration',
      createdAt: '2026-03-17T10:00:01Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    // Iteration 2
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-2',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 2,
      status: 'thinking',
      createdAt: '2026-03-17T10:00:05Z',
      model: { provider: 'openai', model: 'gpt-5.2' }
    }, 'conv-1');

    expect(chatState.liveRows[0].executionGroups).toHaveLength(2);

    // Preamble for iteration 2
    applyPreambleEvent(chatState, {
      turnId: 'turn-1',
      assistantMessageId: 'mc-2',
      content: 'Second iteration thinking...'
    }, 'conv-1');

    // Should update the LAST group only
    expect(chatState.liveRows[0].executionGroups[0].preamble).toBe('First iteration');
    expect(chatState.liveRows[0].executionGroups[1].preamble).toBe('Second iteration thinking...');
    expect(chatState.liveRows[0].preamble).toBe('Second iteration thinking...');
  });
});

describe('applyToolStreamEvent — terminal timestamp coverage (P2 fix)', () => {
  // Use applyExecutionStreamEvent to create the assistant row (matching existing
  // test patterns), then applyToolStreamEvent to add the tool call step.
  function makeBaseState() {
    const chatState = { liveRows: [] };
    applyExecutionStreamEvent(chatState, {
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      iteration: 1,
      status: 'thinking',
      createdAt: '2026-04-13T10:00:00Z',
      model: { provider: 'test', model: 'test-model' }
    }, 'conv-1');
    applyToolStreamEvent(chatState, {
      type: 'tool_call_started',
      assistantMessageId: 'mc-1',
      conversationId: 'conv-1',
      turnId: 'turn-1',
      toolCallId: 'tc-1',
      toolMessageId: 'tm-1',
      toolName: 'system/exec:start',
      status: 'running',
      createdAt: '2026-04-13T10:00:01Z',
    }, 'conv-1');
    return chatState;
  }

  it('stamps completedAt on tool_call_completed', () => {
    const chatState = makeBaseState();
    applyToolStreamEvent(chatState, {
      type: 'tool_call_completed',
      assistantMessageId: 'mc-1',
      toolCallId: 'tc-1',
      toolMessageId: 'tm-1',
      toolName: 'system/exec:start',
      status: 'completed',
      createdAt: '2026-04-13T10:00:05Z',
    }, 'conv-1');
    const step = chatState.liveRows[0].executionGroups[0].toolSteps[0];
    expect(step.completedAt).toBe('2026-04-13T10:00:05Z');
    expect(step.status).toBe('completed');
  });

  it('stamps completedAt on tool_call_failed', () => {
    const chatState = makeBaseState();
    applyToolStreamEvent(chatState, {
      type: 'tool_call_failed',
      assistantMessageId: 'mc-1',
      toolCallId: 'tc-1',
      toolMessageId: 'tm-1',
      toolName: 'system/exec:start',
      status: 'failed',
      createdAt: '2026-04-13T10:00:06Z',
    }, 'conv-1');
    const step = chatState.liveRows[0].executionGroups[0].toolSteps[0];
    expect(step.completedAt).toBe('2026-04-13T10:00:06Z');
    expect(step.status).toBe('failed');
  });

  it('stamps completedAt on tool_call_canceled', () => {
    const chatState = makeBaseState();
    applyToolStreamEvent(chatState, {
      type: 'tool_call_canceled',
      assistantMessageId: 'mc-1',
      toolCallId: 'tc-1',
      toolMessageId: 'tm-1',
      toolName: 'system/exec:start',
      status: 'canceled',
      createdAt: '2026-04-13T10:00:07Z',
    }, 'conv-1');
    const step = chatState.liveRows[0].executionGroups[0].toolSteps[0];
    expect(step.completedAt).toBe('2026-04-13T10:00:07Z');
    expect(step.status).toBe('canceled');
  });

  it('does not stamp completedAt on tool_call_waiting (non-terminal)', () => {
    const chatState = makeBaseState();
    applyToolStreamEvent(chatState, {
      type: 'tool_call_waiting',
      assistantMessageId: 'mc-1',
      toolCallId: 'tc-1',
      toolMessageId: 'tm-1',
      toolName: 'system/exec:start',
      status: 'waiting',
      createdAt: '2026-04-13T10:00:03Z',
    }, 'conv-1');
    const step = chatState.liveRows[0].executionGroups[0].toolSteps[0];
    expect(step.completedAt).toBeUndefined();
    expect(step.status).toBe('waiting');
  });
});
