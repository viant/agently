import { describe, expect, it } from 'vitest';
import {
  readConversationProjectionUsage,
  recordConversationProjectionUsage,
} from './usageProjectionStore';

function memoryStorage() {
  const values = new Map();
  return {
    getItem: (key) => values.get(key) || null,
    setItem: (key, value) => values.set(key, String(value)),
  };
}

describe('usage projection store', () => {
  it('accumulates context tokens freed by turn and replaces retries', () => {
    const storage = memoryStorage();
    expect(recordConversationProjectionUsage({
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      projection: { tokensFreed: 40, reason: 'tool call supersession' },
    }, storage)).toBe(true);
    recordConversationProjectionUsage({
      conversationId: 'conversation-1',
      turnId: 'turn-2',
      projection: { tokensFreed: 10 },
    }, storage);
    recordConversationProjectionUsage({
      conversationId: 'conversation-1',
      turnId: 'turn-1',
      projection: { tokensFreed: 45 },
    }, storage);

    const usage = readConversationProjectionUsage('conversation-1', storage);
    expect(usage.entries).toHaveLength(2);
    expect(usage.tokensFreed).toBe(55);
  });
});
