import { beforeEach, describe, expect, it } from 'vitest';

import { __resetAll, getProjection, getState, onSSE, onTranscript, steer, submit } from './chatStore.js';

const CONVERSATION = 'canonical-e2e';

beforeEach(() => __resetAll());

describe('canonical chat pipeline', () => {
  it('keeps render identity stable from local submit through SSE and transcript refinement', () => {
    submit({ conversationId: CONVERSATION, clientRequestId: 'request-1', content: 'hello', createdAt: '2026-01-01T00:00:00Z' });
    const initial = getProjection(CONVERSATION);
    const userKey = initial.find((row) => row.kind === 'user').renderKey;
    const iterationKey = initial.find((row) => row.kind === 'iteration').renderKey;

    onSSE(CONVERSATION, {
      type: 'turn_started', conversationId: CONVERSATION, turnId: 'turn-1',
      userMessageId: 'user-1', clientRequestId: 'request-1', createdAt: '2026-01-01T00:00:01Z',
    });
    onTranscript(CONVERSATION, {
      conversationId: CONVERSATION,
      turns: [{ turnId: 'turn-1', status: 'running', user: { messageId: 'user-1', clientRequestId: 'request-1', content: 'hello' } }],
    });

    const refined = getProjection(CONVERSATION);
    expect(refined.filter((row) => row.kind === 'user')).toHaveLength(1);
    expect(refined.find((row) => row.kind === 'user').renderKey).toBe(userKey);
    expect(refined.find((row) => row.kind === 'iteration').renderKey).toBe(iterationKey);
  });

  it('preserves streaming report fences exactly while text deltas accumulate', () => {
    onSSE(CONVERSATION, { type: 'turn_started', conversationId: CONVERSATION, turnId: 'turn-report' });
    onSSE(CONVERSATION, {
      type: 'text_delta', conversationId: CONVERSATION, turnId: 'turn-report', assistantMessageId: 'assistant-report',
      content: '```forge-report\n{"version":1',
    });
    onSSE(CONVERSATION, {
      type: 'text_delta', conversationId: CONVERSATION, turnId: 'turn-report', assistantMessageId: 'assistant-report',
      content: ',"id":"brief"}\n```',
    });
    const content = getState(CONVERSATION).turns[0].pages[0].content;
    expect(content).toContain('```forge-report');
    expect(content).toContain('"id":"brief"');
  });

  it('settles lifecycle only on terminal turn events', () => {
    onSSE(CONVERSATION, { type: 'turn_started', conversationId: CONVERSATION, turnId: 'turn-1' });
    onSSE(CONVERSATION, { type: 'model_completed', conversationId: CONVERSATION, turnId: 'turn-1', status: 'completed' });
    expect(getState(CONVERSATION).turns[0].lifecycle).toBe('running');
    onSSE(CONVERSATION, { type: 'turn_completed', conversationId: CONVERSATION, turnId: 'turn-1', status: 'completed' });
    expect(getState(CONVERSATION).turns[0].lifecycle).toBe('completed');
  });

  it('keeps steering messages in the active turn instead of creating a queued turn', () => {
    submit({ conversationId: CONVERSATION, clientRequestId: 'request-1', content: 'initial', createdAt: '2026-01-01T00:00:00Z' });
    onSSE(CONVERSATION, { type: 'turn_started', conversationId: CONVERSATION, turnId: 'turn-1', clientRequestId: 'request-1' });
    steer({ conversationId: CONVERSATION, clientRequestId: 'request-2', content: 'narrow scope', createdAt: '2026-01-01T00:00:02Z' });
    expect(getState(CONVERSATION).turns).toHaveLength(1);
    expect(getProjection(CONVERSATION).filter((row) => row.kind === 'user').map((row) => row.content)).toEqual(['initial', 'narrow scope']);
  });

  it('isolates simultaneous conversation projections', () => {
    submit({ conversationId: 'conversation-a', clientRequestId: 'a', content: 'alpha', createdAt: '2026-01-01T00:00:00Z' });
    submit({ conversationId: 'conversation-b', clientRequestId: 'b', content: 'beta', createdAt: '2026-01-01T00:00:00Z' });
    expect(JSON.stringify(getProjection('conversation-a'))).toContain('alpha');
    expect(JSON.stringify(getProjection('conversation-a'))).not.toContain('beta');
    expect(JSON.stringify(getProjection('conversation-b'))).toContain('beta');
  });
});
