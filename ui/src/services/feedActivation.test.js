import { describe, expect, it } from 'vitest';
import { feedActivationEventType, mergeFeedSnapshotActivation } from './feedActivation';

describe('feed snapshot activation', () => {
  it('does not treat refreshed historical content as a new activation decision', () => {
    const prior = [{ feedId: 'feed', active: null, activationKnown: false }];
    const [restored] = mergeFeedSnapshotActivation([{ feedId: 'feed', data: { rows: [2] } }], prior);
    expect(feedActivationEventType(restored)).toBe('tool_feed_unknown');
    expect(restored.data.rows).toEqual([2]);
  });
  it('honors explicit active, inactive, and unknown snapshot decisions', () => {
    const previous = [{ feedId: 'feed', active: null, activationKnown: false }];
    for (const [active, activationKnown, type] of [[true, true, 'tool_feed_active'], [false, true, 'tool_feed_inactive'], [null, false, 'tool_feed_unknown']]) {
      const [feed] = mergeFeedSnapshotActivation([{ feedId: 'feed', active, activationKnown }], previous);
      expect(feedActivationEventType(feed)).toBe(type);
    }
  });
  it('retains unknown through omitted content before legacy content reappears', () => {
    const prior = [{ feedId: 'feed', active: null, activationKnown: false, data: { rows: [1] } }];
    const omitted = mergeFeedSnapshotActivation([], prior);
    expect(omitted).toEqual([{ feedId: 'feed', active: null, activationKnown: false }]);
    const [returned] = mergeFeedSnapshotActivation([{ feedId: 'feed', data: { rows: [2] } }], omitted);
    expect(feedActivationEventType(returned)).toBe('tool_feed_unknown');
    expect(returned.data.rows).toEqual([2]);
  });
});
