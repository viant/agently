import { describe, expect, it } from 'vitest';
import { resolveLayoutWindowKey } from './WorkspaceSidebar.jsx';

describe('workspace layout window actions', () => {
  it('keeps local and remote window identities distinct', () => {
    expect(resolveLayoutWindowKey({ windowKey: 'overview' })).toBe('overview');
    expect(resolveLayoutWindowKey({ provider: 'workspace', windowKey: 'overview' })).toBe('overview');
    expect(resolveLayoutWindowKey({ provider: 'operations-ui', windowKey: 'overview' })).toBe('provider:operations-ui:overview');
  });
});
