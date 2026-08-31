import { afterEach, describe, expect, it } from 'vitest';
import { clearFeedEntityAliases, registerFeedEntityAlias, rewriteFeedEntityAliases } from './feedEntityAliases';

describe('feed entity aliases', () => {
  afterEach(() => clearFeedEntityAliases());

  it('rewrites a configured canonical id into a labeled generic entity token', () => {
    expect(registerFeedEntityAlias({
      ui: { entity: { type: 'thing', idPath: 'output.id', labelPath: 'output.name' } },
      data: { output: { id: 'opaque_123', name: 'Business name' } },
    }, 'conv-1')).toBe(true);
    expect(rewriteFeedEntityAliases('Updated `opaque_123` version 2.', 'conv-1'))
      .toBe('Updated @{thing:opaque_123 "Business name"} version 2.');
  });
});
