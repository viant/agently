import { describe, expect, it, vi } from 'vitest';

import { syncTranscriptSnapshot } from './transcriptStore';

describe('syncTranscriptSnapshot', () => {
  it('does not clear completed live-session rows after a finished turn', () => {
    const chatState = {
      transcriptRows: [],
      liveRows: [
        {
          id: 'assistant-1',
          role: 'assistant',
          turnId: 'turn-1',
          createdAt: '2026-03-16T10:00:00Z',
          interim: 0,
          status: 'completed'
        },
        {
          id: 'stream:assistant-1',
          _type: 'stream',
          role: 'assistant',
          turnId: 'turn-1',
          createdAt: '2026-03-16T10:00:01Z',
          interim: 0,
          isStreaming: false,
          content: 'Hi!'
        }
      ],
      renderRows: [],
      liveOwnedConversationID: 'conv-1',
      liveOwnedTurnIds: ['turn-1'],
      lastConversationID: 'conv-1',
      lastQueuedTurns: [],
      lastHasRunning: true,
      runningTurnId: 'turn-1'
    };
    const conversationsDS = {
      peekFormData: () => ({ id: 'conv-1' }),
      setFormData: vi.fn()
    };
    const context = {
      Context: (name) => {
        if (name === 'conversations') {
          return { handlers: { dataSource: conversationsDS } };
        }
        return null;
      }
    };
    const result = syncTranscriptSnapshot({
      context,
      turns: [
        {
          id: 'turn-1',
          status: 'succeeded'
        }
      ],
      ensureContextResources: () => chatState,
      resolveActiveStreamTurnId: () => '',
      mapTranscriptToRows: () => ({
        rows: [
          {
            id: 'user-1',
            role: 'user',
            turnId: 'turn-1',
            createdAt: '2026-03-16T09:59:59Z',
            content: 'hi'
          }
        ],
        queuedTurns: [],
        runningTurnId: ''
      }),
      findLatestRunningTurnIdFromTurns: () => '',
      findLatestRunningTurnId: () => '',
      publishChangeFeed: vi.fn(),
      publishPlanFeed: vi.fn(),
      setStage: vi.fn(),
      liveRows: chatState.liveRows
    });

    expect(result?.shouldFinalizeActiveStream).toBe(false);
    expect(chatState.liveRows).toHaveLength(2);
    expect(chatState.liveOwnedTurnIds).toEqual(['turn-1']);
    expect(chatState.activeStreamTurnId).toBeUndefined();
  });
});
