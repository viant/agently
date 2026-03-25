import { describe, expect, it } from 'vitest';

import { resolveSubmitAgent } from './chatService';

describe('resolveSubmitAgent', () => {
  it('prefers explicit selected auto agent over stale conversation/default agent', () => {
    expect(resolveSubmitAgent({
      selectedAgent: 'auto',
      persistedAgent: '',
      metaForm: { agent: 'chatter', defaults: { agent: 'chatter' } },
      convForm: { agent: 'chatter' }
    })).toBe('auto');
  });

  it('prefers persisted auto agent over stale conversation/default agent', () => {
    expect(resolveSubmitAgent({
      selectedAgent: '',
      persistedAgent: 'auto',
      metaForm: { agent: 'chatter', defaults: { agent: 'chatter' } },
      convForm: { agent: 'chatter' }
    })).toBe('auto');
  });
});
