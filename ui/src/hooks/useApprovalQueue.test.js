import { describe, expect, it } from 'vitest';

import { shouldPollApprovalQueue } from './useApprovalQueue';

describe('shouldPollApprovalQueue', () => {
  it('polls aggressively only when the queue is open in the visible focused tab', () => {
    expect(shouldPollApprovalQueue(true, 'visible', true, true)).toBe(true);
    expect(shouldPollApprovalQueue(true, 'visible', true, false)).toBe(false);
    expect(shouldPollApprovalQueue(false, 'visible', true, true)).toBe(false);
    expect(shouldPollApprovalQueue(true, 'hidden', true, true)).toBe(false);
    expect(shouldPollApprovalQueue(true, 'visible', false, true)).toBe(false);
  });
});
