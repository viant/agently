import { describe, expect, it } from 'vitest';

import { shouldPollApprovalQueue } from './useApprovalQueue';

describe('shouldPollApprovalQueue', () => {
  it('polls only when the queue is enabled in the visible focused tab', () => {
    expect(shouldPollApprovalQueue(true, 'visible', true)).toBe(true);
    expect(shouldPollApprovalQueue(false, 'visible', true)).toBe(false);
    expect(shouldPollApprovalQueue(true, 'hidden', true)).toBe(false);
    expect(shouldPollApprovalQueue(true, 'visible', false)).toBe(false);
  });
});
