import { describe, expect, it } from 'vitest';

import { shouldAutoScrollStreamingRow } from './BubbleMessage';

describe('BubbleMessage auto-scroll helpers', () => {
  it('auto-scrolls when the streaming row is already near the bottom edge', () => {
    expect(shouldAutoScrollStreamingRow(
      { bottom: 540 },
      { bottom: 500 },
      96
    )).toBe(true);
  });

  it('does not auto-scroll when the user is far away from the streaming row', () => {
    expect(shouldAutoScrollStreamingRow(
      { bottom: 900 },
      { bottom: 500 },
      96
    )).toBe(false);
  });
});
