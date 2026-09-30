import { describe, expect, it } from 'vitest';
import { resolveLayoutWindowKey, resolveLayoutWindowOptions } from './WorkspaceSidebar.jsx';

describe('workspace layout window actions', () => {
  it('keeps local and remote window identities distinct', () => {
    expect(resolveLayoutWindowKey({ windowKey: 'overview' })).toBe('overview');
    expect(resolveLayoutWindowKey({ provider: 'workspace', windowKey: 'overview' })).toBe('overview');
    expect(resolveLayoutWindowKey({ provider: 'operations-ui', windowKey: 'overview' })).toBe('provider:operations-ui:overview');
  });

  it.each(['advertisers', 'campaigns', 'spo'])('opens %s without requiring a chat host on the landing page', (windowKey) => {
    const options = resolveLayoutWindowOptions({windowKey, parameters: {executeOnOpen: true}}, '');
    expect(options).toEqual({parameters: {executeOnOpen: true}, conversationId: undefined,
      parentKey: 'chat/new', presentation: 'hosted', region: 'chat.top'});
  });

  it('hosts menu windows in the active conversation and preserves report parameters', () => {
    expect(resolveLayoutWindowOptions({parameters: {reportStarterId: 'spo', executeOnOpen: true}}, 'conv-1')).toEqual({
      parameters: {reportStarterId: 'spo', executeOnOpen: true}, conversationId: 'conv-1',
      parentKey: 'chat/new', presentation: 'hosted', region: 'chat.top',
    });
  });
});
