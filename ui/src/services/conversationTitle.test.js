import { describe, expect, it } from 'vitest';

import { resolveConversationSummary, resolveConversationTitle } from './conversationTitle';

describe('conversationTitle', () => {
  it('does not use summary text as the sidebar row title fallback', () => {
    const row = {
      Id: 'conv-1',
      Summary: 'Performance highlights for project 547754',
      Prompt: 'Analyze project 547754 performance'
    };

    expect(resolveConversationTitle(row)).toBe('Analyze project 547754 performance');
  });

  it('exposes summary text separately for sidebar hover metadata', () => {
    const row = {
      Id: 'conv-1',
      Summary: 'Performance highlights for project 547754'
    };

    expect(resolveConversationSummary(row)).toBe('Performance highlights for project 547754');
  });

  it('strips the persisted Title prefix from the visible sidebar row title', () => {
    const row = {
      Id: 'conv-1',
      Title: 'Title: Project 547754 Performance Analysis and Recommended Next Actions'
    };

    expect(resolveConversationTitle(row)).toBe('Project 547754 Performance Analysis and Recommended Next Actions');
  });
});
