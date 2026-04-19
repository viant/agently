import { describe, expect, it } from 'vitest';

import {
  localOptimisticRunningForPath,
  computeSubmittingWhileProcessing,
  shouldHoldLegacySubmitLatch,
  shouldClearLegacyOptimisticRunning,
  makeLocalQueuedPreview,
  computeLegacyIsProcessing,
  shouldShowStarterTasks,
  effectiveShowAbortWhileRunning,
  retainLocalQueuedTurns,
} from '../../../../forge/src/components/chatLegacySubmitState.js';

describe('chatLegacySubmitState', () => {
  it('disables local optimistic running on the canonical external path', () => {
    expect(localOptimisticRunningForPath(true, true)).toBe(false);
    expect(localOptimisticRunningForPath(false, true)).toBe(true);
  });

  it('computes submitting/processing state for external and legacy paths', () => {
    expect(computeSubmittingWhileProcessing({
      externalStateOwns: true,
      isProcessing: false,
      backendConversationRunning: true,
      localOptimisticRunning: true,
      submitLatch: true,
    })).toBe(false);

    expect(computeSubmittingWhileProcessing({
      externalStateOwns: false,
      isProcessing: false,
      backendConversationRunning: false,
      localOptimisticRunning: true,
      submitLatch: false,
    })).toBe(true);
  });

  it('decides when the legacy submit latch or optimistic running should clear', () => {
    expect(shouldHoldLegacySubmitLatch({
      usesLegacyFeedState: false,
      isProcessing: true,
    })).toBe(false);

    expect(shouldHoldLegacySubmitLatch({
      usesLegacyFeedState: true,
      isProcessing: false,
      backendConversationRunning: false,
      localOptimisticRunning: false,
    })).toBe(false);

    expect(shouldHoldLegacySubmitLatch({
      usesLegacyFeedState: true,
      isProcessing: true,
    })).toBe(true);

    expect(shouldClearLegacyOptimisticRunning({
      usesLegacyFeedState: false,
      turnLifecycleRunning: true,
    })).toBe(false);

    expect(shouldClearLegacyOptimisticRunning({
      usesLegacyFeedState: true,
      turnLifecycleRunning: false,
      legacyActiveExecutions: false,
      legacyLastTurnStatus: '',
    })).toBe(false);

    expect(shouldClearLegacyOptimisticRunning({
      usesLegacyFeedState: true,
      turnLifecycleRunning: true,
      legacyActiveExecutions: false,
      legacyLastTurnStatus: '',
    })).toBe(true);
  });

  it('creates deterministic local queue previews when clock/random are injected', () => {
    expect(makeLocalQueuedPreview(' queued request ', 12345, 0.6789)).toEqual({
      id: 'local:12345:6789',
      preview: 'queued request',
      local: true,
    });
  });

  it('computes legacy processing visibility without leaking path-specific branching', () => {
    expect(computeLegacyIsProcessing({
      usesLegacyFeedState: false,
      turnLifecycleRunning: true,
    })).toBe(true);

    expect(computeLegacyIsProcessing({
      usesLegacyFeedState: true,
      turnLifecycleRunning: false,
      legacyActiveExecutions: false,
      legacyLastTurnStatus: 'completed',
      lastUserIndex: 1,
      lastAssistantIndex: 0,
      isTerminalTurnStatus: (value) => value === 'completed',
    })).toBe(false);

    expect(computeLegacyIsProcessing({
      usesLegacyFeedState: true,
      turnLifecycleRunning: false,
      legacyActiveExecutions: false,
      legacyLastTurnStatus: '',
      lastUserIndex: 2,
      lastAssistantIndex: 1,
      isTerminalTurnStatus: () => false,
    })).toBe(true);
  });

  it('computes starter-task and abort visibility', () => {
    expect(shouldShowStarterTasks({
      starterTaskCount: 1,
      messageCount: 0,
      conversationID: '',
      isProcessing: false,
    })).toBe(true);

    expect(shouldShowStarterTasks({
      starterTaskCount: 1,
      messageCount: 1,
      conversationID: '',
      isProcessing: false,
    })).toBe(false);

    expect(effectiveShowAbortWhileRunning({
      effectiveShowAbort: true,
      turnLifecycleRunning: false,
      abortVisible: true,
    })).toBe(true);

    expect(effectiveShowAbortWhileRunning({
      effectiveShowAbort: false,
      turnLifecycleRunning: true,
      abortVisible: true,
    })).toBe(false);
  });

  it('filters local queued turns through the legacy preview retention contract', () => {
    expect(retainLocalQueuedTurns({
      current: [
        { id: 'local-1', preview: 'keep me', local: true },
        { id: 'local-2', preview: 'drop me', local: true },
      ],
      messages: [],
      runningTurnId: 'turn-1',
      backendQueuedContent: new Set(),
      isConversationStillActive: false,
      shouldKeepLocalQueuedPreview: (_messages, preview) => preview === 'keep me',
    })).toEqual([
      { id: 'local-1', preview: 'keep me', local: true },
    ]);
  });
});
