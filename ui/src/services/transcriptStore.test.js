import { describe, expect, it, vi } from 'vitest';

import { syncTranscriptSnapshot, tickTranscript } from './transcriptStore';

describe('syncTranscriptSnapshot', () => {
  it('clears completed live-session ownership once transcript confirms the turn is finished', () => {
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
      runningTurnId: 'turn-1',
      activeStreamTurnId: 'turn-1',
      activeStreamStartedAt: 123
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

    expect(result?.shouldFinalizeActiveStream).toBe(true);
    expect(result?.liveRows).toEqual([]);
    expect(chatState.liveRows).toEqual([]);
    expect(chatState.liveOwnedTurnIds).toEqual([]);
    expect(chatState.liveOwnedConversationID).toBe('');
    expect(chatState.activeStreamTurnId).toBe('');
    expect(chatState.activeStreamStartedAt).toBe(0);
  });

  it('drops previously cached transcript rows for owned turns once live ownership starts', () => {
    const publishChangeFeed = vi.fn();
    const publishPlanFeed = vi.fn();
    const chatState = {
      transcriptRows: [
        {
          id: 'msg-user-1',
          role: 'user',
          turnId: 'turn-1',
          createdAt: '2026-03-16T09:59:59Z',
          content: 'Analyze performance of ad order 2652066.'
        }
      ],
      liveRows: [
        {
          id: 'user:turn-1',
          role: 'user',
          turnId: 'turn-1',
          createdAt: '2026-03-16T09:59:58Z',
          content: 'Analyze performance of ad order 2652066.'
        }
      ],
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
          status: 'running'
        }
      ],
      ensureContextResources: () => chatState,
      resolveActiveStreamTurnId: () => 'turn-1',
      mapTranscriptToRows: () => ({
        rows: [
          {
            id: 'msg-user-1',
            role: 'user',
            turnId: 'turn-1',
            createdAt: '2026-03-16T09:59:59Z',
            content: 'Analyze performance of ad order 2652066.'
          }
        ],
        queuedTurns: [],
        runningTurnId: 'turn-1'
      }),
      findLatestRunningTurnIdFromTurns: () => 'turn-1',
      findLatestRunningTurnId: () => 'turn-1',
      publishChangeFeed,
      publishPlanFeed,
      setStage: vi.fn(),
      liveRows: chatState.liveRows
    });

    expect(result?.transcriptRows).toEqual([]);
    expect(chatState.transcriptRows).toEqual([]);
    expect(publishChangeFeed).toHaveBeenCalledWith({ conversationId: 'conv-1', rows: [] });
    expect(publishPlanFeed).toHaveBeenCalledWith({ conversationId: 'conv-1', rows: [] });
  });
});

describe('tickTranscript', () => {
  it('falls back to a full transcript fetch when incremental polling stalls on a user-only snapshot', async () => {
    const chatState = {
      lastSinceCursor: 'msg-user-1',
      transcriptRows: [
        {
          id: 'msg-user-1',
          role: 'user',
          turnId: 'turn-1',
          createdAt: '2026-03-20T10:00:00Z',
          content: 'run schedule'
        }
      ],
      lastHasRunning: false
    };
    const conversationsDS = {
      peekFormData: () => ({ id: 'conv-1' })
    };
    const context = {
      Context: (name) => {
        if (name === 'conversations') {
          return { handlers: { dataSource: conversationsDS } };
        }
        return null;
      }
    };
    const fullTurns = [
      {
        id: 'turn-1',
        message: [
          { id: 'msg-user-1', role: 'user', content: 'run schedule' },
          { id: 'msg-assistant-1', role: 'assistant', interim: 0, content: 'done' }
        ]
      }
    ];
    const fetchTranscript = vi.fn()
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce(fullTurns);
    const fetchPendingElicitations = vi.fn().mockResolvedValue([]);
    const resolveLastTranscriptCursor = vi.fn(() => 'msg-assistant-1');
    const syncTranscriptSnapshot = vi.fn(() => ({ hasRunning: false, conversationID: 'conv-1' }));

    const result = await tickTranscript({
      context,
      ensureContextResources: () => chatState,
      fetchTranscript,
      fetchPendingElicitations,
      resolveLastTranscriptCursor,
      syncTranscriptSnapshot
    });

    expect(fetchTranscript).toHaveBeenCalledTimes(2);
    expect(fetchTranscript).toHaveBeenNthCalledWith(1, 'conv-1', 'msg-user-1');
    expect(fetchTranscript).toHaveBeenNthCalledWith(2, 'conv-1', '');
    expect(fetchPendingElicitations).toHaveBeenCalledWith('conv-1');
    expect(resolveLastTranscriptCursor).toHaveBeenCalledWith(fullTurns);
    expect(syncTranscriptSnapshot).toHaveBeenCalledWith({
      context,
      turns: fullTurns,
      pendingElicitations: [],
      reason: 'poll'
    });
    expect(chatState.lastSinceCursor).toBe('msg-assistant-1');
    expect(result).toEqual({ hasRunning: false, conversationID: 'conv-1' });
  });

  it('does not refetch the full transcript when the current snapshot already has an assistant reply', async () => {
    const chatState = {
      lastSinceCursor: 'msg-assistant-1',
      transcriptRows: [
        {
          id: 'msg-user-1',
          role: 'user',
          turnId: 'turn-1',
          createdAt: '2026-03-20T10:00:00Z',
          content: 'run schedule'
        },
        {
          id: 'msg-assistant-1',
          role: 'assistant',
          turnId: 'turn-1',
          createdAt: '2026-03-20T10:00:01Z',
          interim: 0,
          content: 'done'
        }
      ],
      lastHasRunning: false
    };
    const conversationsDS = {
      peekFormData: () => ({ id: 'conv-1' })
    };
    const context = {
      Context: (name) => {
        if (name === 'conversations') {
          return { handlers: { dataSource: conversationsDS } };
        }
        return null;
      }
    };
    const fetchTranscript = vi.fn().mockResolvedValue([]);
    const syncTranscriptSnapshot = vi.fn();

    const result = await tickTranscript({
      context,
      ensureContextResources: () => chatState,
      fetchTranscript,
      fetchPendingElicitations: vi.fn(),
      resolveLastTranscriptCursor: vi.fn(),
      syncTranscriptSnapshot
    });

    expect(fetchTranscript).toHaveBeenCalledTimes(1);
    expect(fetchTranscript).toHaveBeenCalledWith('conv-1', 'msg-assistant-1');
    expect(syncTranscriptSnapshot).not.toHaveBeenCalled();
    expect(result).toBeUndefined();
  });
});
