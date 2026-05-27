import { describe, expect, it } from 'vitest';

import { snapshotConversationId } from './connector';

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
});
