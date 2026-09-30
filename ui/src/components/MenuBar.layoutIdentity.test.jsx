import { describe, expect, it } from 'vitest';
import { canonicalWindowParameters } from './MenuBar.jsx';

describe('workspace window parameter identity', () => {
  it('ignores object key order while preserving values and array order', () => {
    expect(canonicalWindowParameters({ b: 2, a: { y: 1, x: [1, 2] } }))
      .toBe(canonicalWindowParameters({ a: { x: [1, 2], y: 1 }, b: 2 }));
    expect(canonicalWindowParameters({ id: 1 })).not.toBe(canonicalWindowParameters({ id: '1' }));
    expect(canonicalWindowParameters({ ids: [1, 2] })).not.toBe(canonicalWindowParameters({ ids: [2, 1] }));
  });
});
