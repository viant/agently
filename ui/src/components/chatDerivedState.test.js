import { describe, expect, it } from 'vitest';

import { computeChatDerivedState, computeEffectiveQueuedTurns, effectiveBackendConversationRunning } from '../../../../forge/src/components/chatDerivedState.js';

describe('chatDerivedState', () => {
  it('does not let a stale running flag override explicit terminal conversation state', () => {
    expect(effectiveBackendConversationRunning({ running: true, status: 'canceled', stage: 'thinking' })).toBe(false);
    expect(effectiveBackendConversationRunning({ running: true, status: 'running', stage: 'executing' })).toBe(true);
    expect(effectiveBackendConversationRunning({ running: true })).toBe(true);
    expect(effectiveBackendConversationRunning({ running: false, status: 'running' })).toBe(false);
  });

  it('merges queued turns on the legacy path and passes through on the external path', () => {
    expect(computeEffectiveQueuedTurns({
      usesExternalFeedState: false,
      queuedTurns: [{ id: 'server-1', preview: 'server' }],
      localQueuedTurns: [{ id: 'local-1', preview: 'local' }],
    })).toEqual([
      { id: 'local-1', preview: 'local' },
      { id: 'server-1', preview: 'server' },
    ]);

    expect(computeEffectiveQueuedTurns({
      usesExternalFeedState: true,
      queuedTurns: [{ id: 'server-1', preview: 'server' }],
      localQueuedTurns: [{ id: 'local-1', preview: 'local' }],
    })).toEqual([{ id: 'server-1', preview: 'server' }]);
  });

  it('computes canonical-path running state without legacy branching', () => {
    const state = computeChatDerivedState({
      usesExternalFeedState: true,
      usesLegacyFeedState: false,
      backendConversationRunning: false,
      hasActiveTurnLifecycle: true,
      optimisticRunning: true,
      messages: [],
      legacyActiveExecutions: false,
      legacyLastTurnStatus: '',
      starterTaskCount: 1,
      conversationID: '',
      effectiveShowAbort: true,
      abortVisible: false,
      queuedTurns: [],
      localQueuedTurns: [],
      queuedCountValue: 0,
    });

    expect(state.localOptimisticRunning).toBe(false);
    expect(state.turnLifecycleRunning).toBe(true);
    expect(state.isProcessing).toBe(true);
    expect(state.showStarterTasks).toBe(false);
    expect(state.effectiveShowAbortWhileRunning).toBe(true);
    expect(state.queuedCount).toBe(0);
  });

  it('computes legacy-path processing, starter task, and abort visibility', () => {
    const state = computeChatDerivedState({
      usesExternalFeedState: false,
      usesLegacyFeedState: true,
      backendConversationRunning: false,
      hasActiveTurnLifecycle: false,
      optimisticRunning: false,
      messages: [],
      legacyActiveExecutions: false,
      legacyLastTurnStatus: '',
      lastUserIndex: -1,
      lastAssistantIndex: -1,
      isTerminalTurnStatus: () => false,
      starterTaskCount: 2,
      conversationID: '',
      effectiveShowAbort: true,
      abortVisible: false,
      queuedTurns: [{ id: 'server-1', preview: 'server' }],
      localQueuedTurns: [],
      queuedCountValue: '1',
    });

    expect(state.isProcessing).toBe(true);
    expect(state.showStarterTasks).toBe(false);
    expect(state.effectiveShowAbortWhileRunning).toBe(false);
    expect(state.queuedCount).toBe(1);
  });
});
