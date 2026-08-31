import { describe, expect, it } from 'vitest';

import { normalizeToolFeedTarget, toolFeedTargetsPlacement } from './toolFeedTarget';

describe('tool feed presentation target', () => {
  it('normalizes known values and defaults unknown values to auto', () => {
    expect(normalizeToolFeedTarget(' INLINE ')).toBe('inline');
    expect(normalizeToolFeedTarget('workspace')).toBe('workspace');
    expect(normalizeToolFeedTarget('detached')).toBe('detached');
    expect(normalizeToolFeedTarget('future-target')).toBe('auto');
    expect(normalizeToolFeedTarget()).toBe('auto');
  });

  it('routes explicit feeds only to their declared placement', () => {
    const feed = { presentation: { target: 'inline' } };
    expect(toolFeedTargetsPlacement(feed, 'inline')).toBe(true);
    expect(toolFeedTargetsPlacement(feed, 'workspace')).toBe(false);
  });

  it('includes auto feeds only when the placement owns legacy feeds', () => {
    expect(toolFeedTargetsPlacement({}, 'workspace', true)).toBe(true);
    expect(toolFeedTargetsPlacement({}, 'inline', false)).toBe(false);
  });
});
