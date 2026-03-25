import { describe, expect, it } from 'vitest';

import { resolveConversationSummary, resolveConversationTitle } from './conversationTitle';

describe('conversationTitle', () => {
  it('does not use summary text as the sidebar row title fallback', () => {
    const row = {
      Id: 'conv-1',
      Summary: 'Performance highlights for campaign 547754',
      Prompt: 'Analyze campaign 547754 performance'
    };

    expect(resolveConversationTitle(row)).toBe('Analyze campaign 547754 performance');
  });

  it('exposes summary text separately for sidebar hover metadata', () => {
    const row = {
      Id: 'conv-1',
      Summary: 'Performance highlights for campaign 547754'
    };

    expect(resolveConversationSummary(row)).toBe('Performance highlights for campaign 547754');
  });

  it('strips the persisted Title prefix from the visible sidebar row title', () => {
    const row = {
      Id: 'conv-1',
      Title: 'Title: Campaign 547754 Performance Analysis and Recommended Next Actions'
    };

    expect(resolveConversationTitle(row)).toBe('Campaign 547754 Performance Analysis and Recommended Next Actions');
  });
});
