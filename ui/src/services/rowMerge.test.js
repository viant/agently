import { describe, expect, it } from 'vitest';

import { mergeRenderedRows } from './rowMerge';

function findLatestRunningTurnId(rows = []) {
  for (let index = rows.length - 1; index >= 0; index -= 1) {
    const item = rows[index];
    const status = String(item?.turnStatus || item?.status || '').toLowerCase();
    if (['running', 'thinking', 'processing', 'waiting_for_user', 'in_progress'].includes(status)) {
      return String(item?.turnId || '').trim();
    }
  }
  return '';
}

describe('mergeRenderedRows', () => {
  it('merges transcript rows and canonical live rows at one boundary', () => {
    const transcriptRows = [
      { id: 'user-1', role: 'user', turnId: 'turn-1', createdAt: '2026-03-16T01:00:00Z', content: 'hi' }
    ];
    const liveRows = [
      {
        id: 'assistant-1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:01Z',
        interim: 1,
        status: 'running',
        turnStatus: 'running',
        executionGroups: [
          {
            assistantMessageId: 'assistant-1',
            modelMessageId: 'assistant-1',
            sequence: 1,
            preamble: 'Thinking'
          }
        ]
      }
    ];

    const merged = mergeRenderedRows({
      transcriptRows,
      liveRows,
      runningTurnId: 'turn-1',
      hasRunning: true,
      findLatestRunningTurnId
    });

    expect(merged.map((row) => row.id)).toEqual(['user-1', 'assistant-1']);
    expect(merged[1]).toMatchObject({
      id: 'assistant-1',
      turnId: 'turn-1',
      status: 'running'
    });
  });

  it('keeps a stream placeholder while the turn is live and no canonical execution evidence exists', () => {
    const transcriptRows = [
      { id: 'user-1', role: 'user', turnId: 'turn-1', createdAt: '2026-03-16T01:00:00Z', content: 'hi' }
    ];
    const liveRows = [
      {
        id: 'stream:msg-1',
        _type: 'stream',
        _streamMessageId: 'msg-1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:01Z',
        content: 'H'
      }
    ];

    const merged = mergeRenderedRows({
      transcriptRows,
      liveRows,
      runningTurnId: 'turn-1',
      hasRunning: true,
      findLatestRunningTurnId
    });

    expect(merged).toHaveLength(2);
    expect(merged[1]).toMatchObject({
      id: 'stream:msg-1',
      _type: 'stream',
      turnId: 'turn-1'
    });
  });

  it('drops a stream placeholder once transcript has the final assistant response', () => {
    const transcriptRows = [
      { id: 'user-1', role: 'user', turnId: 'turn-1', createdAt: '2026-03-16T01:00:00Z', content: 'hi' },
      { id: 'assistant-1', role: 'assistant', turnId: 'turn-1', createdAt: '2026-03-16T01:00:02Z', content: 'Hi!', interim: 0 }
    ];
    const liveRows = [
      {
        id: 'stream:assistant-1',
        _type: 'stream',
        _streamMessageId: 'assistant-1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:01Z',
        content: 'H'
      }
    ];

    const merged = mergeRenderedRows({
      transcriptRows,
      liveRows,
      runningTurnId: 'turn-1',
      hasRunning: true,
      findLatestRunningTurnId
    });

    expect(merged.map((row) => row.id)).toEqual(['user-1', 'assistant-1']);
  });

  it('prefers live session rows for owned turns in the current conversation', () => {
    const transcriptRows = [
      { id: 'user-1', role: 'user', turnId: 'turn-1', createdAt: '2026-03-16T01:00:00Z', content: 'hi' },
      { id: 'assistant-1', role: 'assistant', turnId: 'turn-1', createdAt: '2026-03-16T01:00:02Z', content: 'Hi!', interim: 0 }
    ];
    const liveRows = [
      {
        id: 'assistant-1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:01Z',
        interim: 0,
        status: 'completed',
        turnStatus: 'completed',
        executionGroups: [
          {
            assistantMessageId: 'assistant-1',
            modelMessageId: 'assistant-1',
            sequence: 1,
            finalResponse: true,
            status: 'completed',
            content: 'Hi!'
          }
        ]
      },
      {
        id: 'stream:assistant-1',
        _type: 'stream',
        _streamMessageId: 'assistant-1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:02Z',
        content: 'Hi!',
        interim: 0,
        isStreaming: false
      }
    ];

    const merged = mergeRenderedRows({
      transcriptRows,
      liveRows,
      runningTurnId: '',
      hasRunning: false,
      findLatestRunningTurnId,
      currentConversationID: 'conv-1',
      liveOwnedConversationID: 'conv-1',
      liveOwnedTurnIds: ['turn-1']
    });

    // Stream row should be dropped when transcript already has a final assistant for that message/turn.
    expect(merged.map((row) => row.id)).toEqual(['assistant-1']);
  });

  it('keeps the active turn entirely on the live side when the turn is owned by SSE', () => {
    const transcriptRows = [
      { id: 'user-db', role: 'user', turnId: 'turn-1', createdAt: '2026-03-16T01:00:00Z', content: 'db hi' },
      { id: 'assistant-db', role: 'assistant', turnId: 'turn-1', createdAt: '2026-03-16T01:00:02Z', content: 'db answer', interim: 0 }
    ];
    const liveRows = [
      { id: 'user:turn-1', role: 'user', turnId: 'turn-1', createdAt: '2026-03-16T01:00:00Z', content: 'live hi' },
      {
        id: 'assistant-live',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:01Z',
        interim: 1,
        status: 'running',
        turnStatus: 'running',
        executionGroups: [{ assistantMessageId: '', status: 'running', modelSteps: [{ status: 'running', startedAt: '2026-03-16T01:00:01Z' }] }]
      }
    ];

    const merged = mergeRenderedRows({
      transcriptRows,
      liveRows,
      runningTurnId: 'turn-1',
      hasRunning: true,
      findLatestRunningTurnId,
      currentConversationID: 'conv-1',
      liveOwnedConversationID: 'conv-1',
      liveOwnedTurnIds: ['turn-1']
    });

    expect(merged.map((row) => row.id)).toEqual(['user:turn-1', 'assistant-live']);
  });

  it('preserves richer transcript execution-group data over sparse live placeholders', () => {
    const transcriptRows = [
      {
        id: 'assistant-1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:02Z',
        executionGroups: [
          {
            assistantMessageId: 'assistant-1',
            finalResponse: true,
            status: 'completed',
            content: 'Hi!',
            modelSteps: [
              {
                provider: 'openai',
                model: 'gpt-4o-mini',
                status: 'completed',
                startedAt: '2026-03-16T01:00:00Z',
                completedAt: '2026-03-16T01:00:02Z'
              }
            ]
          }
        ]
      }
    ];
    const liveRows = [
      {
        id: 'assistant-1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:02Z',
        executionGroups: [
          {
            assistantMessageId: 'assistant-1',
            finalResponse: false,
            status: 'thinking',
            modelSteps: [
              {
                startedAt: '2026-03-16T01:00:01Z',
                status: 'thinking'
              }
            ]
          }
        ]
      }
    ];

    const merged = mergeRenderedRows({
      transcriptRows,
      liveRows,
      runningTurnId: 'turn-1',
      hasRunning: true,
      findLatestRunningTurnId
    });

    expect(merged[0].executionGroups[0]).toMatchObject({
      finalResponse: true,
      status: 'thinking',
      content: 'Hi!'
    });
    const ms = merged[0].executionGroups[0].modelSteps[0];
    expect(ms).toMatchObject({
      provider: 'openai',
      model: 'gpt-4o-mini',
      completedAt: '2026-03-16T01:00:02Z'
    });
    // startedAt comes from whichever entry wins the merge; both are valid
    expect(['2026-03-16T01:00:00Z', '2026-03-16T01:00:01Z']).toContain(ms.startedAt);
  });

  it('collapses assistant rows with different ids when they belong to the same owned turn', () => {
    const transcriptRows = [
      {
        id: 'turn-1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:02Z',
        content: 'Final answer',
        interim: 0,
        executionGroups: [
          {
            assistantMessageId: 'assistant-db',
            finalResponse: true,
            status: 'completed',
            content: 'Final answer'
          }
        ]
      }
    ];
    const liveRows = [
      {
        id: 'assistant:turn-1:1',
        role: 'assistant',
        turnId: 'turn-1',
        createdAt: '2026-03-16T01:00:01Z',
        interim: 1,
        status: 'running',
        turnStatus: 'running',
        executionGroups: [
          {
            assistantMessageId: 'assistant-live',
            status: 'thinking',
            preamble: 'Thinking...'
          }
        ]
      }
    ];

    const merged = mergeRenderedRows({
      transcriptRows,
      liveRows,
      runningTurnId: 'turn-1',
      hasRunning: true,
      findLatestRunningTurnId,
      currentConversationID: 'conv-1',
      liveOwnedConversationID: 'conv-1',
      liveOwnedTurnIds: ['turn-1']
    });

    expect(merged).toHaveLength(1);
    expect(merged[0].turnId).toBe('turn-1');
    expect(merged[0].executionGroups).toHaveLength(1);
    expect(merged[0].executionGroups[0]).toMatchObject({
      preamble: 'Thinking...'
    });
  });
});
