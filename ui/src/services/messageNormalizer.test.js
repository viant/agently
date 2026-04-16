import { describe, expect, it } from 'vitest';

import { classifyMessage, normalizeMessages } from './messageNormalizer';

describe('classifyMessage', () => {
  it('keeps pending schema-based elicitations as read-only bubbles for the global overlay flow', () => {
    expect(classifyMessage({
      role: 'elicition',
      status: 'open',
      elicitation: {
        requestedSchema: {
          type: 'object',
          properties: {
            desiredFormat: { type: 'string' }
          }
        }
      }
    })).toBe('bubble');
  });
});

describe('normalizeMessages', () => {
  it('drops assistant summary artifacts from rendered chat rows', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'analyze campaign'
      },
      {
        id: 'sum-1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:01Z',
        status: 'summary',
        content: 'Highlights: pacing healthy, delivery soft.'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:02Z',
        interim: 0,
        content: 'Campaign 547754 is pacing slightly behind target.'
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: 3 });

    expect(normalized.some((entry) => String(entry?.id || '') === 'sum-1')).toBe(false);
    expect(normalized.some((entry) => String(entry?.content || '').includes('Highlights: pacing healthy'))).toBe(false);
    expect(normalized.some((entry) => String(entry?.content || '').includes('Campaign 547754 is pacing slightly behind target.'))).toBe(true);
  });

  it('keeps mode=summary content off the chat bubble and attaches it to the iteration', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'analyze campaign'
      },
      {
        id: 'a0',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        createdAt: '2026-01-01T10:00:00.500Z',
        interim: 1,
        content: 'Calling updatePlan.'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:01Z',
        interim: 0,
        content: '## Highlights\n- Campaign pacing is slightly behind target.'
      },
      {
        id: 'a2',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:02Z',
        mode: 'summary',
        interim: 0,
        content: 'Title: Campaign 547754 Performance Analysis and Recommended Next Actions\n\n- Saved 3 actionable recommendations'
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: 3 });
    const iteration = normalized.find((entry) => entry?._type === 'iteration');

    expect(normalized.some((entry) => String(entry?.content || '').includes('Saved 3 actionable recommendations'))).toBe(false);
    expect(iteration?._iterationData?.summary).toMatchObject({
      id: 'a2',
      mode: 'summary'
    });
    expect(iteration?._iterationData?.response?.content).toContain('Campaign pacing is slightly behind target');
  });

  it('prefers turn agentIdUsed over createdByUserId when synthesizing iteration data', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'run performance diagnostics'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 1,
        createdAt: '2026-01-01T10:00:01Z',
        content: 'Resolving hierarchy first.',
        createdByUserId: 'steward',
        agentIdUsed: 'steward-performance'
      },
      {
        id: 'a2',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 0,
        createdAt: '2026-01-01T10:00:02Z',
        content: 'done',
        createdByUserId: 'steward',
        agentIdUsed: 'steward-performance'
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: 3 });
    const iteration = normalized.find((entry) => entry?._type === 'iteration');

    expect(iteration?._iterationData?.agentId).toBe('steward-performance');
  });

  it('keeps the backend final model step without fabricating a synthetic final step', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'hello'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 1,
        createdAt: '2026-01-01T10:00:01Z',
        content: 'Using system/os/getEnv.',
        executions: [{
          steps: [{
            id: 'model-1',
            kind: 'model',
            reason: 'thinking',
            toolName: 'openai/gpt-5.2',
            status: 'completed'
          }]
        }]
      },
      {
        id: 'tool-1',
        role: 'tool',
        turnId: 'turn-1',
        iteration: 1,
        createdAt: '2026-01-01T10:00:02Z',
        content: '',
        executions: [{
          steps: [{
            id: 'tool-step-1',
            kind: 'tool',
            reason: 'tool_call',
            toolName: 'system/os/getEnv',
            status: 'completed'
          }]
        }]
      },
      {
        id: 'a2',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 0,
        createdAt: '2026-01-01T10:00:03Z',
        content: 'done',
        executions: [{
          steps: [{
            id: 'model-2',
            kind: 'model',
            reason: 'final_response',
            toolName: 'openai/gpt-5.2',
            status: 'completed'
          }]
        }]
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: 3 });
    const iteration = normalized.find((entry) => entry?._type === 'iteration');

    expect(iteration?._iterationData?.toolCalls?.map((step) => step.id)).toEqual(['model-1', 'tool-step-1', 'model-2']);
    expect(iteration?._iterationData?.toolCalls?.some((step) => String(step?.id || '').endsWith(':final'))).toBe(false);
  });

  it('keeps later non-interim assistant messages in the same turn as separate bubbles', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'analyze repo'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 1,
        createdAt: '2026-01-01T10:00:01Z',
        content: 'Using resources-list.',
        executions: [{
          steps: [{
            id: 'model-1',
            kind: 'model',
            reason: 'thinking',
            toolName: 'openai/gpt-5.2',
            status: 'completed'
          }]
        }]
      },
      {
        id: 'tool-1',
        role: 'tool',
        turnId: 'turn-1',
        iteration: 1,
        createdAt: '2026-01-01T10:00:02Z',
        content: '',
        executions: [{
          steps: [{
            id: 'tool-step-1',
            kind: 'tool',
            reason: 'tool_call',
            toolName: 'resources-list',
            status: 'completed'
          }]
        }]
      },
      {
        id: 'a1-final',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 0,
        createdAt: '2026-01-01T10:00:02.500Z',
        content: 'Here is the first answer.'
      },
      {
        id: 'a2',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 2,
        interim: 1,
        createdAt: '2026-01-01T10:00:03Z',
        content: 'Using resources-list and resources-grepFiles.',
        executions: [{
          steps: [{
            id: 'model-2',
            kind: 'model',
            reason: 'thinking',
            toolName: 'openai/gpt-5.2',
            status: 'completed'
          }]
        }]
      },
      {
        id: 'tool-2',
        role: 'tool',
        turnId: 'turn-1',
        iteration: 2,
        createdAt: '2026-01-01T10:00:04Z',
        content: '',
        executions: [{
          steps: [{
            id: 'tool-step-2',
            kind: 'tool',
            reason: 'tool_call',
            toolName: 'resources-grepFiles',
            status: 'completed'
          }]
        }]
      },
      {
        id: 'a2-final',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 2,
        interim: 0,
        createdAt: '2026-01-01T10:00:05Z',
        content: 'Here is the revised answer.'
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: Number.MAX_SAFE_INTEGER });
    const iterations = normalized.filter((entry) => entry?._type === 'iteration');
    expect(iterations).toHaveLength(2);
    expect(iterations[0]?._iterationData?.response?.content).toBe('Here is the first answer.');
    expect(iterations[1]?._iterationData?.response?.content).toBe('Here is the revised answer.');
    expect(iterations[1]?._iterationData?.preamble?.content).toBe('Using resources-list and resources-grepFiles.');
  });

  it('keeps synthesized iteration createdAt deterministic when source timestamps are missing', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'analyze repo'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 1,
        content: 'Calling updatePlan.'
      },
      {
        id: 'a2',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 0,
        content: 'Done.'
      }
    ];

    const first = normalizeMessages(messages, { visibleCount: 3 });
    const second = normalizeMessages(messages, { visibleCount: 3 });
    const firstIteration = first.find((entry) => entry?._type === 'iteration');
    const secondIteration = second.find((entry) => entry?._type === 'iteration');

    expect(firstIteration?.createdAt).toBe('');
    expect(secondIteration?.createdAt).toBe(firstIteration?.createdAt);
  });

  it('preserves user steering as its own bubble within a turn before the next assistant response', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'forecast inventory'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 1,
        createdAt: '2026-01-01T10:00:01Z',
        content: 'Checking hierarchy.'
      },
      {
        id: 'a1-final',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 0,
        createdAt: '2026-01-01T10:00:02Z',
        content: 'First forecast.'
      },
      {
        id: 'u2',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:03Z',
        content: 'narrow to this targeting set'
      },
      {
        id: 'a2',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 2,
        interim: 1,
        createdAt: '2026-01-01T10:00:04Z',
        content: 'Re-running on the narrowed set.'
      },
      {
        id: 'a2-final',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 2,
        interim: 0,
        createdAt: '2026-01-01T10:00:05Z',
        content: 'Refined forecast.'
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: Number.MAX_SAFE_INTEGER });
    const userBubbles = normalized.filter((entry) => String(entry?.role || '').toLowerCase() === 'user');
    const iterations = normalized.filter((entry) => entry?._type === 'iteration');

    expect(userBubbles.map((entry) => entry.content)).toEqual([
      'forecast inventory',
      'narrow to this targeting set'
    ]);
    expect(iterations).toHaveLength(2);
    expect(iterations[0]?._iterationData?.response?.content).toBe('First forecast.');
    expect(iterations[1]?._iterationData?.response?.content).toBe('Refined forecast.');
  });

  it('renders a parent linked tool call as an execution block with linked conversation metadata', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-parent',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'analyze repo'
      },
      {
        id: 'tool-parent',
        role: 'tool',
        type: 'tool_op',
        turnId: 'turn-parent',
        createdAt: '2026-01-01T10:00:01Z',
        content: '',
        status: 'running',
        toolName: 'llm/agents/run',
        linkedConversationId: 'child-123',
        executions: [{
          steps: [{
            id: 'tool-parent-step',
            kind: 'tool',
            reason: 'tool_call',
            toolName: 'llm/agents/run',
            status: 'running',
            linkedConversationId: 'child-123'
          }]
        }]
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: 1 });
    const iteration = normalized.find((entry) => entry?._type === 'iteration');

    expect(iteration).toBeTruthy();
    expect(iteration?._iterationData?.toolCalls).toHaveLength(1);
    expect(iteration?._iterationData?.toolCalls?.[0]).toMatchObject({
      toolName: 'llm/agents/run',
      linkedConversationId: 'child-123',
      status: 'running'
    });
    expect(normalized.some((entry) => entry?._type === 'paginator')).toBe(false);
  });

  it('prefers live stream content over preamble content for the active iteration bubble', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'say hi'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:01Z',
        interim: 1,
        content: 'Let me think about that...',
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:02Z',
        interim: 1,
        content: 'Hello there',
        _bubbleSource: 'stream'
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: 1 });
    const iteration = normalized.find((entry) => entry?._type === 'iteration');

    expect(iteration).toBeTruthy();
    expect(iteration.content).toBe('Hello there');
    expect(iteration._iterationData?.preamble?.content).toBe('Let me think about that...');
    expect(iteration._iterationData?.streamContent).toBe('Hello there');
  });

  it('preserves canonical executionGroups on a tool-only row so execution details can render the parent model call', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-parent',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'analyze repo'
      },
      {
        id: 'tool-parent',
        role: 'tool',
        type: 'tool_op',
        turnId: 'turn-parent',
        createdAt: '2026-01-01T10:00:01Z',
        content: '',
        status: 'running',
        toolName: 'llm/agents/run',
        linkedConversationId: 'child-123',
        executionGroups: [
          {
            parentMessageId: 'model-1',
            modelMessageId: 'model-1',
            sequence: 1,
            status: 'running',
            preamble: 'Inspecting the repository.',
            modelCall: {
              provider: 'openai',
              model: 'gpt-5.2',
              status: 'running'
            },
            toolMessages: [
              {
                id: 'tool-parent',
                linkedConversationId: 'child-123'
              }
            ],
            toolCalls: [
              {
                messageId: 'tool-parent',
                toolName: 'llm/agents:run',
                status: 'running'
              }
            ]
          }
        ],
        executions: [{
          steps: [{
            id: 'tool-parent-step',
            kind: 'tool',
            reason: 'tool_call',
            toolName: 'llm/agents/run',
            status: 'running',
            linkedConversationId: 'child-123'
          }]
        }]
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: 1 });
    const iteration = normalized.find((entry) => entry?._type === 'iteration');

    expect(iteration?._iterationData?.executionGroups).toHaveLength(1);
    expect(iteration?._iterationData?.executionGroups?.[0]).toMatchObject({
      parentMessageId: 'model-1'
    });
  });

  it('creates an iteration block from a final assistant row when canonical executionGroups arrive without an earlier interim row', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-final',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'hi'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-final',
        createdAt: '2026-01-01T10:00:05Z',
        interim: 0,
        content: 'Hi! How can I help you today?',
        executionGroups: [
          {
            parentMessageId: 'a1',
            modelMessageId: 'a1',
            sequence: 1,
            status: 'completed',
            finalResponse: true,
            content: 'Hi! How can I help you today?',
            modelCall: {
              provider: 'openai',
              model: 'gpt-4o-mini',
              status: 'completed'
            },
            toolCalls: []
          }
        ]
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: 1 });
    const iteration = normalized.find((entry) => entry?._type === 'iteration');
    const responses = normalized.filter((entry) => String(entry?.role || '').toLowerCase() === 'assistant');

    expect(iteration).toBeTruthy();
    expect(iteration?._iterationData?.response?.content).toBe('Hi! How can I help you today?');
    expect(iteration?._iterationData?.executionGroups?.[0]).toMatchObject({
      finalResponse: true,
      content: 'Hi! How can I help you today?'
    });
    expect(responses.some((entry) => String(entry?.content || '').includes('Hi! How can I help you today?'))).toBe(true);
  });

  it('keeps the standalone stream bubble when an iteration uses stream-owned execution rows', () => {
    const messages = [
      {
        id: 'stream:m1',
        _type: 'stream',
        _rowSource: 'stream',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:01Z',
        content: 'I am going to inspect the repository.',
        interim: 1
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 1,
        _rowSource: 'live',
        _bubbleSource: 'stream',
        createdAt: '2026-01-01T10:00:02Z',
        content: 'I am going to inspect the repository.',
        executions: [{
          steps: [{
            id: 'model-1',
            kind: 'model',
            reason: 'thinking',
            toolName: 'openai/gpt-5.2',
            status: 'running'
          }]
        }]
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: 1 });
    expect(normalized.filter((entry) => entry?._type === 'stream')).toHaveLength(1);
    expect(normalized.filter((entry) => entry?._type === 'iteration')).toHaveLength(1);
  });

  it('preserves a synthetic queue row when iteration rows are present', () => {
    const normalized = normalizeMessages([
      {
        id: 'iteration:turn-1:1',
        _type: 'iteration',
        role: 'assistant',
        createdAt: '2026-01-01T10:00:01Z',
        _iterationData: {
          turnId: 'turn-1',
          status: 'running'
        }
      },
      {
        id: 'queue:conv-1:turn-q1',
        _type: 'queue',
        createdAt: '2026-01-01T10:00:02Z',
        queuedTurns: [{ id: 'turn-q1', preview: 'queued follow-up' }]
      }
    ], { visibleCount: 1 });

    expect(normalized.some((entry) => entry?._type === 'queue')).toBe(true);
    expect(normalized.find((entry) => entry?._type === 'queue')).toMatchObject({
      queuedTurns: [{ id: 'turn-q1', preview: 'queued follow-up' }]
    });
  });

  it('keeps stream-owned text on the iteration when no standalone stream row exists yet', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'write story about bear and dog'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 1,
        _rowSource: 'live',
        _bubbleSource: 'stream',
        createdAt: '2026-01-01T10:00:01Z',
        content: 'Once upon a time, a bear met a dog in the woods.',
        executionGroups: [
          {
            parentMessageId: 'a1',
            modelMessageId: 'a1',
            sequence: 1,
            status: 'streaming',
            finalResponse: false,
            content: '',
            modelCall: {
              provider: 'openai',
              model: 'gpt-5.2',
              status: 'streaming'
            },
            toolCalls: []
          }
        ]
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: 1 });
    const iteration = normalized.find((entry) => entry?._type === 'iteration');

    expect(normalized.filter((entry) => entry?._type === 'stream')).toHaveLength(0);
    expect(iteration).toBeTruthy();
    expect(iteration?.content).toContain('bear met a dog');
    expect(iteration?._iterationData?.streamContent).toContain('bear met a dog');
  });

  it('does not create a duplicate final response bubble from stream-owned canonical rows', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'hi'
      },
      {
        id: 'stream:m1',
        _type: 'stream',
        _rowSource: 'stream',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:01Z',
        content: 'Hi! How can I help you today?',
        interim: 0,
        isStreaming: false
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:01Z',
        interim: 0,
        _rowSource: 'live',
        _bubbleSource: 'stream',
        content: 'Hi! How can I help you today?',
        executionGroups: [
          {
            parentMessageId: 'a1',
            modelMessageId: 'a1',
            sequence: 1,
            status: 'completed',
            finalResponse: true,
            content: 'Hi! How can I help you today?',
            modelCall: {
              provider: 'openai',
              model: 'gpt-5.2',
              status: 'completed'
            }
          }
        ]
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: 1 });
    const streams = normalized.filter((entry) => entry?._type === 'stream');
    const assistantBubbles = normalized.filter((entry) => String(entry?.role || '').toLowerCase() === 'assistant' && entry?._type !== 'iteration');

    expect(normalized.filter((entry) => entry?._type === 'iteration')).toHaveLength(1);
    expect(streams).toHaveLength(1);
    expect(assistantBubbles).toHaveLength(1);
    expect(String(assistantBubbles[0]?.content || '')).toContain('Hi! How can I help you today?');
  });

  it('keeps an interim assistant row even when its content matches the latest user message', () => {
    const normalized = normalizeMessages([
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        content: 'Analyze order 2652536 performance for pacing, spend, delivery, and KPI health, and recommend the top next actions.',
        createdAt: '2026-04-06T19:20:00Z'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        interim: 1,
        content: 'Analyze order 2652536 performance for pacing, spend, delivery, and KPI health, and recommend the top next actions.',
        createdAt: '2026-04-06T19:20:01Z'
      }
    ], { visibleCount: Number.MAX_SAFE_INTEGER });

    expect(normalized).toHaveLength(2);
    expect(normalized[0]).toMatchObject({
      id: 'u1',
      role: 'user'
    });
    expect(normalized[1]).toMatchObject({
      _type: 'iteration'
    });
  });

  it('prefers rawContent for the visible user task when expanded prompt content is also present', () => {
    const normalized = normalizeMessages([
      {
        id: 'u1',
        role: 'user',
        mode: 'task',
        turnId: 'turn-1',
        content: 'User Query:\nforecast deal 141952\nContext:\nmap[Projection:map[hiddenMessageIds:[] hiddenTurnIds:[]]]',
        rawContent: 'forecast deal 141952',
        createdAt: '2026-04-09T17:00:00Z'
      }
    ], { visibleCount: Number.MAX_SAFE_INTEGER });

    expect(normalized).toHaveLength(1);
    expect(normalized[0]).toMatchObject({
      id: 'u1',
      role: 'user',
      content: 'forecast deal 141952'
    });
  });

  it('keeps the original user task visible even when mode is missing if rawContent is present', () => {
    const normalized = normalizeMessages([
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        content: 'User Query:\nRecommend sitelists for audience 7180287\nContext:\nmap[Projection:map[hiddenMessageIds:[] hiddenTurnIds:[] reason: scope: tokensFreed: 0] client:map[capabilities:[markdown chart upload code diff] platform:web surface:browser]]\n\nEND_OF_USER_PROMPT',
        rawContent: 'Recommend sitelists for audience 7180287',
        createdAt: '2026-04-13T20:00:00Z'
      }
    ], { visibleCount: Number.MAX_SAFE_INTEGER });

    expect(normalized).toHaveLength(1);
    expect(normalized[0]).toMatchObject({
      id: 'u1',
      role: 'user',
      content: 'Recommend sitelists for audience 7180287'
    });
  });

  it('preserves distinct same-turn user rows when they have different ids', () => {
    const normalized = normalizeMessages([
      {
        id: 'u1-db',
        role: 'user',
        turnId: 'turn-1',
        content: 'what iris targeting do we have ?',
        createdAt: '2026-04-09T18:05:18Z'
      },
      {
        id: 'u1-live',
        role: 'user',
        mode: 'task',
        turnId: 'turn-1',
        content: 'User Query:\nwhat iris targeting do we have ?\nContext:\nmap[Projection:map[hiddenMessageIds:[] hiddenTurnIds:[] reason: scope:conversation tokensFreed:0]]',
        rawContent: 'what iris targeting do we have ?',
        createdAt: '2026-04-09T18:05:23Z'
      }
    ], { visibleCount: Number.MAX_SAFE_INTEGER });

    const userRows = normalized.filter((entry) => String(entry?.role || '').toLowerCase() === 'user');
    expect(userRows).toHaveLength(2);
    expect(userRows[0]).toMatchObject({
      role: 'user',
      turnId: 'turn-1',
      content: 'what iris targeting do we have ?'
    });
    expect(userRows[1]).toMatchObject({
      role: 'user',
      turnId: 'turn-1',
      rawContent: 'what iris targeting do we have ?'
    });
  });

  it('keeps an interim assistant preamble even when it matches the user prompt and execution details are present', () => {
    const prompt = 'What are my HOME, SHELL, and PATH environment variables?';
    const normalized = normalizeMessages([
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        content: prompt,
        createdAt: '2026-04-06T19:47:00Z'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        interim: 1,
        content: prompt,
        createdAt: '2026-04-06T19:47:01Z',
        executions: [{
          steps: [{
            id: 'tool-step-1',
            kind: 'tool',
            toolName: 'system/os/getEnv',
            status: 'running'
          }]
        }]
      }
    ], { visibleCount: Number.MAX_SAFE_INTEGER });

    expect(normalized.filter((entry) => String(entry?.content || '').trim() === prompt)).toHaveLength(2);
    const iteration = normalized.find((entry) => entry?._type === 'iteration');
    expect(iteration?._iterationData?.toolCalls?.map((step) => step.id)).toContain('tool-step-1');
  });

  it('keeps a non-interim assistant response even when it matches the user prompt and execution steps are attached', () => {
    const prompt = 'What are my HOME, SHELL, and PATH environment variables?';
    const normalized = normalizeMessages([
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        content: prompt,
        createdAt: '2026-04-06T19:47:00Z'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        interim: 0,
        content: prompt,
        createdAt: '2026-04-06T19:47:01Z',
        executions: [{
          steps: [
            {
              id: 'model-step-1',
              kind: 'model',
              toolName: 'openai/gpt-5-mini',
              status: 'completed'
            },
            {
              id: 'tool-step-1',
              kind: 'tool',
              toolName: 'system/os/getEnv',
              status: 'running'
            }
          ]
        }]
      }
    ], { visibleCount: Number.MAX_SAFE_INTEGER });

    expect(normalized.filter((entry) => String(entry?.content || '').trim() === prompt)).toHaveLength(2);
    const iteration = normalized.find((entry) => entry?._type === 'iteration');
    expect(iteration?._iterationData?.toolCalls?.map((step) => step.id)).toEqual(['model-step-1', 'tool-step-1']);
  });

  it('hides internal user elicitation_response rows from the rendered transcript', () => {
    const normalized = normalizeMessages([
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-04-06T19:47:00Z',
        content: 'What are my HOME, SHELL, and PATH environment variables?'
      },
      {
        id: 'u2',
        role: 'user',
        type: 'elicitation_response',
        turnId: 'turn-1',
        createdAt: '2026-04-06T19:47:05Z',
        content: '{"editedFields":{"names":["HOME","PATH"]}}'
      },
      {
        id: 'a2',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-04-06T19:47:06Z',
        interim: 0,
        content: '{"values":{"HOME":"/Users/awitas","PATH":"/usr/bin"}}'
      }
    ], { visibleCount: Number.MAX_SAFE_INTEGER });

    expect(normalized.map((entry) => entry.id)).toEqual(['u1', 'a2']);
  });

  it('keeps an elicitation response without explicit iteration on the current turn iteration', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'analyze repo'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 1,
        createdAt: '2026-01-01T10:00:01Z',
        content: '',
        executions: [{
          steps: [{
            id: 'model-1',
            kind: 'model',
            reason: 'thinking',
            toolName: 'openai/gpt-5.2',
            status: 'completed'
          }]
        }]
      },
      {
        id: 'a2',
        role: 'assistant',
        turnId: 'turn-1',
        interim: 0,
        createdAt: '2026-01-01T10:00:02Z',
        content: 'Need repo contents',
        elicitation: {
          elicitationId: 'elic-1',
          message: 'Need repo contents',
          requestedSchema: {
            type: 'object',
            properties: {
              inputMethod: { type: 'string' }
            }
          }
        }
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: Number.MAX_SAFE_INTEGER });
    expect(normalized.filter((entry) => entry?._type === 'iteration')).toHaveLength(1);
    expect(normalized.filter((entry) => entry?.elicitation?.requestedSchema)).toHaveLength(1);
  });

  it('does not render literal undefined when an iteration has no visible response yet', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'forecast inventory'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 1,
        createdAt: '2026-01-01T10:00:01Z',
        content: '',
        executions: [{
          steps: [{
            id: 'model-1',
            kind: 'model',
            reason: 'thinking',
            toolName: 'openai/gpt-5.4',
            status: 'thinking'
          }]
        }]
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: Number.MAX_SAFE_INTEGER });
    const iteration = normalized.find((entry) => entry?._type === 'iteration');

    expect(iteration).toBeTruthy();
    expect(String(iteration?.content || '')).toBe('');
  });

  it('treats iteration 0 as unset so same-turn elicitation stays on the active iteration', () => {
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-01-01T10:00:00Z',
        content: 'analyze repo'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 1,
        interim: 1,
        createdAt: '2026-01-01T10:00:01Z',
        content: '',
        executions: [{
          steps: [{
            id: 'model-1',
            kind: 'model',
            reason: 'thinking',
            toolName: 'openai/gpt-5.2',
            status: 'completed'
          }]
        }]
      },
      {
        id: 'a2',
        role: 'assistant',
        turnId: 'turn-1',
        iteration: 0,
        interim: 0,
        createdAt: '2026-01-01T10:00:02Z',
        content: 'Need repo contents',
        elicitation: {
          elicitationId: 'elic-1',
          message: 'Need repo contents',
          requestedSchema: {
            type: 'object',
            properties: {
              inputMethod: { type: 'string' }
            }
          }
        }
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: Number.MAX_SAFE_INTEGER });
    expect(normalized.filter((entry) => entry?._type === 'iteration')).toHaveLength(1);
    expect(normalized.filter((entry) => entry?.elicitation?.requestedSchema)).toHaveLength(1);
  });

  it('preserves generatedFiles on synthesized iteration rows', () => {
    const generatedFiles = [{ id: 'gf-1', filename: 'mouse_story.pdf', status: 'ready' }];
    const messages = [
      {
        id: 'u1',
        role: 'user',
        turnId: 'turn-1',
        createdAt: '2026-04-01T18:38:40Z',
        content: 'make a pdf'
      },
      {
        id: 'a1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-04-01T18:38:47Z',
        content: 'Created [mouse_story.pdf](sandbox:/mnt/data/mouse_story.pdf).',
        generatedFiles,
        executionGroups: [
          {
            assistantMessageId: 'a1',
            content: 'Created [mouse_story.pdf](sandbox:/mnt/data/mouse_story.pdf).'
          }
        ]
      }
    ];

    const normalized = normalizeMessages(messages, { visibleCount: Number.MAX_SAFE_INTEGER });
    const iteration = normalized.find((entry) => entry?._type === 'iteration');

    expect(iteration).toBeTruthy();
    expect(iteration.generatedFiles).toEqual(generatedFiles);
  });
});
