import { describe, expect, it, vi } from 'vitest';

import { syncTranscriptSnapshot, tickTranscript } from './transcriptStore';

describe('syncTranscriptSnapshot', () => {
  it('keeps latest turn live-owned after transcript confirms it is finished', () => {
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
      setStage: vi.fn(),
      liveRows: chatState.liveRows
    });

    expect(result?.shouldFinalizeActiveStream).toBe(false);
    expect(result?.liveRows).toEqual(chatState.liveRows);
    expect(chatState.liveOwnedTurnIds).toEqual(['turn-1']);
    expect(chatState.liveOwnedConversationID).toBe('conv-1');
    expect(chatState.activeStreamTurnId).toBe('turn-1');
    expect(chatState.activeStreamStartedAt).toBe(123);
  });

  it('keeps stale live ownership when the completed turn is still the latest turn', () => {
    const chatState = {
      transcriptRows: [],
      liveRows: [
        {
          id: 'assistant-live',
          role: 'assistant',
          turnId: 'turn-1',
          createdAt: '2026-04-06T19:47:00Z',
          interim: 1,
          status: 'running',
          turnStatus: 'running',
          content: 'The agent wants access to your HOME and PATH environment variables.'
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
          turnId: 'turn-1',
          status: 'completed'
        }
      ],
      ensureContextResources: () => chatState,
      resolveActiveStreamTurnId: () => 'turn-1',
      mapTranscriptToRows: () => ({
        rows: [
          {
            id: 'user-1',
            role: 'user',
            turnId: 'turn-1',
            createdAt: '2026-04-06T19:47:00Z',
            content: 'What are my HOME, SHELL, and PATH environment variables?'
          },
          {
            id: 'assistant-1',
            role: 'assistant',
            turnId: 'turn-1',
            createdAt: '2026-04-06T19:47:05Z',
            interim: 0,
            status: 'completed',
            content: '{\"values\":{\"HOME\":\"/Users/awitas\",\"PATH\":\"/usr/bin\"}}'
          }
        ],
        queuedTurns: [],
        runningTurnId: ''
      }),
      findLatestRunningTurnIdFromTurns: () => '',
      findLatestRunningTurnId: () => '',
      setStage: vi.fn(),
      liveRows: chatState.liveRows
    });

    expect(result?.shouldFinalizeActiveStream).toBe(false);
    expect(result?.liveRows).toEqual(chatState.liveRows);
    expect(chatState.liveOwnedTurnIds).toEqual(['turn-1']);
    expect(chatState.liveOwnedConversationID).toBe('conv-1');
  });

  it('releases live ownership once a newer turn exists in transcript history', () => {
    const chatState = {
      transcriptRows: [],
      liveRows: [
        {
          id: 'assistant-live',
          role: 'assistant',
          turnId: 'turn-1',
          createdAt: '2026-04-06T19:47:00Z',
          interim: 0,
          status: 'completed',
          content: 'done'
        }
      ],
      renderRows: [],
      liveOwnedConversationID: 'conv-1',
      liveOwnedTurnIds: ['turn-1'],
      lastConversationID: 'conv-1',
      lastQueuedTurns: [],
      lastHasRunning: false,
      runningTurnId: '',
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
        { turnId: 'turn-1', status: 'completed' },
        { turnId: 'turn-2', status: 'completed' }
      ],
      ensureContextResources: () => chatState,
      resolveActiveStreamTurnId: () => '',
      mapTranscriptToRows: () => ({
        rows: [
          { id: 'user-1', role: 'user', turnId: 'turn-1', createdAt: '2026-04-06T19:47:00Z', content: 'first' },
          { id: 'assistant-1', role: 'assistant', turnId: 'turn-1', createdAt: '2026-04-06T19:47:05Z', interim: 0, status: 'completed', content: 'done' },
          { id: 'user-2', role: 'user', turnId: 'turn-2', createdAt: '2026-04-06T19:48:00Z', content: 'second' }
        ],
        queuedTurns: [],
        runningTurnId: ''
      }),
      findLatestRunningTurnIdFromTurns: () => '',
      findLatestRunningTurnId: () => '',
      setStage: vi.fn(),
      liveRows: chatState.liveRows
    });

    expect(result?.shouldFinalizeActiveStream).toBe(true);
    expect(chatState.liveRows).toEqual([]);
    expect(chatState.liveOwnedTurnIds).toEqual([]);
    expect(chatState.liveOwnedConversationID).toBe('');
  });

  it('drops previously cached transcript rows for owned turns once live ownership starts', () => {
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
      setStage: vi.fn(),
      liveRows: chatState.liveRows
    });

    expect(result?.transcriptRows).toEqual([]);
    expect(chatState.transcriptRows).toEqual([]);
  });

  it('does not let transcript render a locally active turn before SSE owns it', () => {
    const chatState = {
      transcriptRows: [
        {
          id: 'cached-user-1',
          role: 'user',
          turnId: 'turn-1',
          createdAt: '2026-03-16T09:59:59Z',
          content: 'Analyze performance of ad order 2652066.'
        },
        {
          id: 'cached-assistant-1',
          role: 'assistant',
          turnId: 'turn-1',
          createdAt: '2026-03-16T10:00:01Z',
          interim: 1,
          status: 'running',
          content: 'Thinking...'
        }
      ],
      liveRows: [],
      liveOwnedConversationID: 'conv-1',
      liveOwnedTurnIds: [],
      activeStreamPrompt: 'Analyze performance of ad order 2652066.',
      lastConversationID: 'conv-1',
      lastQueuedTurns: [],
      lastHasRunning: true,
      runningTurnId: ''
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
      resolveActiveStreamTurnId: () => '',
      mapTranscriptToRows: () => ({
        rows: [
          {
            id: 'msg-user-1',
            role: 'user',
            turnId: 'turn-1',
            createdAt: '2026-03-16T09:59:59Z',
            content: 'Analyze performance of ad order 2652066.'
          },
          {
            id: 'msg-assistant-1',
            role: 'assistant',
            turnId: 'turn-1',
            createdAt: '2026-03-16T10:00:01Z',
            interim: 1,
            status: 'running',
            content: 'Thinking...'
          }
        ],
        queuedTurns: [],
        runningTurnId: 'turn-1'
      }),
      findLatestRunningTurnIdFromTurns: () => 'turn-1',
      findLatestRunningTurnId: () => 'turn-1',
      setStage: vi.fn(),
      liveRows: chatState.liveRows
    });

    expect(result?.transcriptRows).toEqual([]);
    expect(chatState.transcriptRows).toEqual([]);
    expect(result?.liveRows).toEqual([]);
    expect(chatState.liveRows).toEqual([]);
    expect(chatState.liveOwnedConversationID).toBe('conv-1');
    expect(chatState.liveOwnedTurnIds).toEqual([]);
  });

  it('reapplies transcript-backed tool feeds for the settled conversation', () => {
    const applyFeedEvent = vi.fn();
    const chatState = {
      transcriptRows: [],
      liveRows: [],
      liveOwnedConversationID: '',
      liveOwnedTurnIds: [],
      lastConversationID: '',
      lastQueuedTurns: [],
      lastHasRunning: false,
      runningTurnId: '',
      lastTranscriptFeedsByConversation: {
        'conv-1': [
          {
            feedId: 'changes',
            title: 'Changes',
            itemCount: 1,
            data: { output: { changes: [{ path: 'sample.txt' }] } }
          }
        ]
      }
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

    syncTranscriptSnapshot({
      context,
      turns: [{ id: 'turn-1', status: 'completed' }],
      ensureContextResources: () => chatState,
      resolveActiveStreamTurnId: () => '',
      mapTranscriptToRows: () => ({
        rows: [
          {
            id: 'assistant-1',
            role: 'assistant',
            turnId: 'turn-1',
            createdAt: '2026-03-16T10:00:00Z',
            content: 'done'
          }
        ],
        queuedTurns: [],
        runningTurnId: ''
      }),
      findLatestRunningTurnIdFromTurns: () => '',
      findLatestRunningTurnId: () => '',
      applyFeedEvent,
      setStage: vi.fn(),
      liveRows: chatState.liveRows
    });

    expect(applyFeedEvent).toHaveBeenCalledWith({
      type: 'tool_feed_active',
      feedId: 'changes',
      feedTitle: 'Changes',
      feedItemCount: 1,
      feedData: { output: { changes: [{ path: 'sample.txt' }] } },
      conversationId: 'conv-1',
    });
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
    expect(fetchTranscript).toHaveBeenNthCalledWith(1, 'conv-1', 'msg-user-1', {});
    expect(fetchTranscript).toHaveBeenNthCalledWith(2, 'conv-1', '', {});
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
    expect(fetchTranscript).toHaveBeenCalledWith('conv-1', 'msg-assistant-1', {});
    expect(syncTranscriptSnapshot).not.toHaveBeenCalled();
    expect(result).toBeUndefined();
  });

  it('forwards transcript options to incremental and recovery fetches', async () => {
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
    const fullTurns = [{ id: 'turn-1', message: [{ id: 'msg-assistant-1', role: 'assistant', interim: 0, content: 'done' }] }];
    const fetchTranscript = vi.fn()
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce(fullTurns);

    await tickTranscript({
      context,
      options: {
        transcript: {
          includeExecutionDetails: false
        }
      },
      ensureContextResources: () => chatState,
      fetchTranscript,
      fetchPendingElicitations: vi.fn().mockResolvedValue([]),
      resolveLastTranscriptCursor: vi.fn(() => 'msg-assistant-1'),
      syncTranscriptSnapshot: vi.fn(() => ({ hasRunning: false, conversationID: 'conv-1' }))
    });

    expect(fetchTranscript).toHaveBeenCalledTimes(2);
    expect(fetchTranscript).toHaveBeenNthCalledWith(1, 'conv-1', 'msg-user-1', { includeExecutionDetails: false });
    expect(fetchTranscript).toHaveBeenNthCalledWith(2, 'conv-1', '', { includeExecutionDetails: false });
  });
});
