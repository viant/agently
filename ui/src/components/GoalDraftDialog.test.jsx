import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';

vi.mock('@blueprintjs/core', () => ({
  Dialog: ({ isOpen, children, title }) => isOpen
    ? React.createElement('div', { 'data-testid': 'bp-dialog' }, title, children)
    : null,
}));

vi.mock('../services/agentlyClient', () => ({
  client: {
    createGoal: vi.fn(),
    updateGoal: vi.fn(),
  },
}));

vi.mock('../services/chatRuntime', () => ({
  refreshGoalFeed: vi.fn(),
}));

vi.mock('../services/httpClient', () => ({
  showToast: vi.fn(),
}));

import GoalDraftDialog from './GoalDraftDialog.jsx';

describe('GoalDraftDialog', () => {
  it('renders the dedicated autonomous goal drafting thin', () => {
    const html = renderToStaticMarkup(
      <GoalDraftDialog isOpen conversationId="conv-1" onClose={() => {}} />
    );
    expect(html).toContain('Autonomous Goal');
    expect(html).toContain('Create A Structured Autonomous Goal');
    expect(html).toContain('Outcome');
    expect(html).toContain('Verification');
    expect(html).toContain('Constraints');
    expect(html).toContain('/goal &lt;desired end state&gt; verified by &lt;specific evidence&gt;');
  });
});
