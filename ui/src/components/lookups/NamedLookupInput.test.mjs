import { describe, expect, it } from 'vitest';

import { deriveLookupTriggerState } from './NamedLookupInput.jsx';

const registry = [
  { name: 'line', dataSource: 'audience_lookup' },
  { name: 'order', dataSource: 'ad_order_lookup' },
];

describe('deriveLookupTriggerState', () => {
  it('keeps the name picker active while the lookup name is still being typed', () => {
    expect(deriveLookupTriggerState({
      display: '/li',
      caret: 3,
      registry,
    })).toEqual({
      phase: 'namePicker',
      start: 0,
      caret: 3,
      query: 'li',
    });
  });

  it('commits an exact lookup name on whitespace and switches to row picker mode', () => {
    expect(deriveLookupTriggerState({
      display: '/line Preroll',
      caret: 13,
      registry,
    })).toEqual({
      phase: 'rowPicker',
      start: 0,
      caret: 13,
      query: 'Preroll',
      entry: registry[0],
    });
  });

  it('opens the row picker with an empty query right after the committed lookup name', () => {
    expect(deriveLookupTriggerState({
      display: '/line ',
      caret: 6,
      registry,
    })).toEqual({
      phase: 'rowPicker',
      start: 0,
      caret: 6,
      query: '',
      entry: registry[0],
    });
  });

  it('does not guess when whitespace follows a non-existent lookup name', () => {
    expect(deriveLookupTriggerState({
      display: '/unknown value',
      caret: 14,
      registry,
    })).toBeNull();
  });
});
