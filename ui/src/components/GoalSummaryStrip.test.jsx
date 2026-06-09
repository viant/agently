import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';

const getFeedDataMock = vi.hoisted(() => vi.fn(() => null));

vi.mock('../services/toolFeedBus', () => ({
  getFeedData: getFeedDataMock,
  onFeedDataChange: vi.fn(() => () => {}),
}));

vi.mock('../services/agentlyClient', () => ({
  client: {
    updateGoal: vi.fn(),
  },
}));

vi.mock('../services/chatRuntime', () => ({
  refreshGoalFeed: vi.fn(),
}));

vi.mock('../services/httpClient', () => ({
  showToast: vi.fn(),
}));

import GoalSummaryStrip from './GoalSummaryStrip.jsx';

describe('GoalSummaryStrip', () => {
  it('renders nothing without a conversation', () => {
    const html = renderToStaticMarkup(<GoalSummaryStrip conversationId="" />);
    expect(html).toBe('');
  });

  it('renders a summary for the active goal', () => {
    getFeedDataMock.mockReturnValueOnce({
      data: {
        goal: {
          objective: 'Ship the release',
          status: 'active',
        },
      },
    });
    const html = renderToStaticMarkup(<GoalSummaryStrip conversationId="conv-1" />);
    expect(html).toContain('Ship the release');
    expect(html).toContain('Manage goal');
  });

  it('renders a no-goal prompt when the feed is present but empty', () => {
    getFeedDataMock.mockReturnValueOnce({
      data: { goal: null },
    });
    const html = renderToStaticMarkup(<GoalSummaryStrip conversationId="conv-1" />);
    expect(html).toContain('No goal set');
    expect(html).toContain('Set goal');
  });

  it('renders wakeup schedule details when controller schedule metadata is present', () => {
    getFeedDataMock.mockReturnValueOnce({
      data: {
        goal: {
          objective: 'Ship the release',
          status: 'active',
        },
        controllerSchedule: {
          mode: 'wakeup',
          wakeAt: '2026-06-08T19:30:00Z',
        },
      },
    });
    const html = renderToStaticMarkup(<GoalSummaryStrip conversationId="conv-1" />);
    expect(html).toContain('Scheduled to resume');
  });
});
