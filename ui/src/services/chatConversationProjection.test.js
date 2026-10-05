import { describe, expect, it } from 'vitest';
import { projectChatConversation } from './chatConversationProjection';

describe('native chat conversation projection', () => {
  it('keeps app host RPC results out of chat without changing the saved transcript', () => {
    const chat = { turnId: 'chat', origin: 'user', assistant: { content: 'chat answer' } };
    const host = { turnId: 'rpc', origin: 'host_request', assistant: { content: 'private app answer' } };
    const goal = { turnId: 'goal', origin: 'goal' };
    const original = { conversationId: 'native', turns: [chat, host, goal], feeds: [{ feedId: 'report' }] };
    const projected = projectChatConversation(original);
    expect(projected.turns).toEqual([chat, goal]);
    expect(projected.feeds).toBe(original.feeds);
    expect(original.turns).toEqual([chat, host, goal]);
  });
  it('preserves legacy untagged turns and empty host-only snapshots', () => {
    const original = { turns: [{ turnId: 'legacy' }] };
    expect(projectChatConversation(original)).toBe(original);
    expect(projectChatConversation({ turns: [{ origin: 'host_request' }] }).turns).toEqual([]);
    expect(projectChatConversation(null)).toBeNull();
  });
});
