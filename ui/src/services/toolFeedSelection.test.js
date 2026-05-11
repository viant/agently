import { beforeEach, describe, expect, it } from 'vitest';

import {
  __resetToolFeedSelectionForTest,
  clearFeedSelection,
  clearFeedSelectionForConversation,
  expandFeed,
  getExpandedFeedIds,
  getSelectedFeedId,
  activateExclusiveFeed,
  onFeedExpansionChange,
  onSelectedFeedChange,
  reconcileFeedSelection,
} from './toolFeedSelection';

describe('toolFeedSelection', () => {
  beforeEach(() => {
    __resetToolFeedSelectionForTest();
  });

  it('clears stale selection when feeds are removed', () => {
    expandFeed('conv-a::plan', 'conv-a');

    reconcileFeedSelection([]);

    expect(Array.from(getExpandedFeedIds())).toEqual([]);
    expect(getSelectedFeedId()).toBe('');
  });

  it('retains only feed ids that still exist and falls forward to a valid feed', () => {
    expandFeed('conv-a::plan', 'conv-a');
    expandFeed('conv-a::terminal', 'conv-a');

    reconcileFeedSelection([
      { feedId: 'conv-b::changes' },
      { feedId: 'conv-a::terminal' },
    ]);

    expect(Array.from(getExpandedFeedIds())).toEqual(['conv-a::terminal', 'conv-b::changes']);
    expect(getSelectedFeedId('conv-a')).toBe('conv-a::terminal');
    expect(getSelectedFeedId('conv-b')).toBe('conv-b::changes');
  });

  it('tracks selected feed independently per conversation', () => {
    activateExclusiveFeed('conv-a::plan', 'conv-a');
    activateExclusiveFeed('conv-b::changes', 'conv-b');

    expect(getSelectedFeedId('conv-a')).toBe('conv-a::plan');
    expect(getSelectedFeedId('conv-b')).toBe('conv-b::changes');
    expect(getSelectedFeedId()).toBe('conv-b::changes');
    expect(Array.from(getExpandedFeedIds())).toEqual(['conv-a::plan', 'conv-b::changes']);
  });

  it('notifies listeners when feed selection is fully cleared', () => {
    const selected = [];
    const expanded = [];
    const offSelected = onSelectedFeedChange((value) => selected.push(value));
    const offExpanded = onFeedExpansionChange((value) => expanded.push(Array.from(value)));

    activateExclusiveFeed('conv-a::plan', 'conv-a');
    clearFeedSelection();

    offSelected();
    offExpanded();

    expect(selected).toContain('conv-a::plan');
    expect(selected[selected.length - 1]).toBe('');
    expect(expanded).toContainEqual(['conv-a::plan']);
    expect(expanded[expanded.length - 1]).toEqual([]);
  });

  it('clears selection only for the targeted conversation', () => {
    activateExclusiveFeed('conv-a::plan', 'conv-a');
    activateExclusiveFeed('conv-b::changes', 'conv-b');

    clearFeedSelectionForConversation('conv-a');

    expect(Array.from(getExpandedFeedIds())).toEqual(['conv-b::changes']);
    expect(getSelectedFeedId('conv-a')).toBe('');
    expect(getSelectedFeedId('conv-b')).toBe('conv-b::changes');
  });
});
