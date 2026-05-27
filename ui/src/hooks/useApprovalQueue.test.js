import { describe, expect, it, vi } from 'vitest';

import {
  dispatchApprovalDecisionOutcomes,
  pickNextOutcomeCursor,
  resolveApprovalDecisionOutcome,
  shouldPollApprovalQueue,
} from './useApprovalQueue';

describe('shouldPollApprovalQueue', () => {
  it('polls aggressively only when the queue is open in the visible focused tab', () => {
    expect(shouldPollApprovalQueue(true, 'visible', true, true)).toBe(true);
    expect(shouldPollApprovalQueue(true, 'visible', true, false, true)).toBe(true);
    expect(shouldPollApprovalQueue(true, 'visible', true, false)).toBe(false);
    expect(shouldPollApprovalQueue(false, 'visible', true, true)).toBe(false);
    expect(shouldPollApprovalQueue(true, 'hidden', true, true)).toBe(false);
    expect(shouldPollApprovalQueue(true, 'visible', false, true)).toBe(false);
  });

  it('extracts canonical approval outcomes from decide responses', () => {
    expect(resolveApprovalDecisionOutcome({
      outcome: {
        approvalId: 'approval-1',
        action: 'approve',
        status: 'executed',
        toolName: 'system/os/getEnv',
        result: '{"values":{"HOME":"/tmp"}}',
      },
    })).toEqual({
      approvalId: 'approval-1',
      action: 'approve',
      status: 'executed',
      decision: '',
      conversationId: '',
      turnId: '',
      messageId: '',
      toolName: 'system/os/getEnv',
      result: '{"values":{"HOME":"/tmp"}}',
      errorMessage: '',
    });
  });

  it('dispatches timeout outcomes from pending-approval polling results', () => {
    const dispatch = vi.fn();
    dispatchApprovalDecisionOutcomes({
      outcomes: [{
        approvalId: 'approval-timeout',
        action: 'timeout',
        status: 'timed_out',
        toolName: 'system/os/getEnv',
        errorMessage: 'approval request timed out',
      }],
    }, dispatch);
    expect(dispatch).toHaveBeenCalledWith({
      approvalId: 'approval-timeout',
      action: 'timeout',
      status: 'timed_out',
      decision: '',
      conversationId: '',
      turnId: '',
      messageId: '',
      toolName: 'system/os/getEnv',
      result: '',
      errorMessage: 'approval request timed out',
    });
  });

  // pickNextOutcomeCursor must never invent or interpret the cursor
  // value — it just stores and echoes the backend-issued opaque
  // string. The durable transport contract relies on the UI not
  // dropping the cursor when the backend echoes an empty value,
  // because empty means "no advance" rather than "reset".
  it('keeps the previous cursor when the backend returns no new cursor', () => {
    expect(pickNextOutcomeCursor({}, 'prev')).toBe('prev');
    expect(pickNextOutcomeCursor({ outcomeCursor: '' }, 'prev')).toBe('prev');
    expect(pickNextOutcomeCursor({ outcomeCursor: '   ' }, 'prev')).toBe('prev');
    expect(pickNextOutcomeCursor(null, 'prev')).toBe('prev');
  });

  it('adopts the backend cursor verbatim when present', () => {
    expect(pickNextOutcomeCursor({ outcomeCursor: '2026-05-26T12:00:00Z' }, '')).toBe('2026-05-26T12:00:00Z');
    expect(pickNextOutcomeCursor({ outcomeCursor: '  2026-05-26T12:01:00Z  ' }, 'prev')).toBe('2026-05-26T12:01:00Z');
  });

  // The durability proof at the UI layer is: when the backend
  // re-emits the same canonical outcome on the poll that carries the
  // prior cursor (i.e. the client polled after the transition
  // moment, not exactly during it), the dispatcher still fires the
  // approval event bus with the canonical payload. This mirrors the
  // backend test TestEmbeddedClient_ListPendingToolApprovals_DurableTimeoutOutcomeViaCursor
  // and proves the UI consumes outcomes from the durable poll path,
  // not just from the one-shot decide response.
  it('re-dispatches the same canonical outcome on a subsequent poll that carries the prior cursor', () => {
    const dispatch = vi.fn();
    const outcome = {
      approvalId: 'approval-durable',
      action: 'timeout',
      status: 'timed_out',
      toolName: 'system/os/getEnv',
      errorMessage: 'approval request timed out',
    };
    const firstPoll = { outcomes: [outcome], outcomeCursor: 'C1' };
    const secondPoll = { outcomes: [outcome], outcomeCursor: 'C2' };
    dispatchApprovalDecisionOutcomes(firstPoll, dispatch);
    dispatchApprovalDecisionOutcomes(secondPoll, dispatch);
    expect(dispatch).toHaveBeenCalledTimes(2);
    expect(dispatch.mock.calls[0][0]).toMatchObject({ approvalId: 'approval-durable', action: 'timeout', status: 'timed_out' });
    expect(dispatch.mock.calls[1][0]).toMatchObject({ approvalId: 'approval-durable', action: 'timeout', status: 'timed_out' });
    expect(pickNextOutcomeCursor(secondPoll, pickNextOutcomeCursor(firstPoll, ''))).toBe('C2');
  });
});
