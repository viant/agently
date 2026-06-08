import { describe, expect, it, vi } from 'vitest';

vi.mock('forge/core', () => ({
  buildUISnapshot: () => ({
    selected: { windowId: 'chat/new', tabId: 'chat/new' },
    windows: [
      {
        windowId: 'chat/new',
        windowKey: 'chat/new',
        parameters: {
          conversations: {
            form: {
              id: 'conv-snapshot'
            }
          }
        }
      }
    ]
  }),
  ensureUIBridgeClientId: () => 'bridge-client-123'
}));

import { connectorConfig, snapshotConversationId } from './connector';

describe('snapshotConversationId', () => {
  it('uses the main chat conversation instead of a selected workspace window conversation', () => {
    const snapshot = {
      selected: {
        windowId: 'orderPerformance_1'
      },
      windows: [
        {
          windowId: 'chat/new',
          windowKey: 'chat/new',
          parameters: {
            conversations: {
              form: {
                id: ''
              }
            }
          }
        },
        {
          windowId: 'orderPerformance_1',
          windowKey: 'orderPerformance',
          parameters: {
            conversations: {
              form: {
                id: 'b370e953-c6a3-4a11-8805-ea4bde1b361f'
              }
            }
          }
        }
      ]
    };

    expect(snapshotConversationId(snapshot)).toBe('');
  });

  it('returns the main chat conversation when present', () => {
    const snapshot = {
      windows: [
        {
          windowId: 'chat/new',
          windowKey: 'chat/new',
          parameters: {
            conversations: {
              form: {
                id: 'conv-123'
              }
            }
          }
        },
        {
          windowId: 'orderPerformance_1',
          windowKey: 'orderPerformance',
          parameters: {
            conversations: {
              form: {
                id: 'other-conv'
              }
            }
          }
        }
      ]
    };

    expect(snapshotConversationId(snapshot)).toBe('conv-123');
  });

  it('snapshot builder falls back to a generated bridge client id', () => {
    const previousWindow = global.window;
    global.window = {};

    try {
      const snapshot = connectorConfig.uiBridge.snapshotBuilder();
      expect(snapshot.clientId).toBe('bridge-client-123');
      expect(snapshot.conversationId).toBe('conv-snapshot');
    } finally {
      global.window = previousWindow;
    }
  });
});
