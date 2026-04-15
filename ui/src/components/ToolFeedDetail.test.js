import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';

import { resolveRootFeedDataSourceName } from './ToolFeedDetail.jsx';

const { getFeedDataMock, makeFeedKeyMock } = vi.hoisted(() => ({
  getFeedDataMock: vi.fn(() => null)
  ,makeFeedKeyMock: vi.fn((feedId, conversationId = '') => conversationId ? `${conversationId}::${feedId}` : feedId)
}));

vi.mock('../services/toolFeedBus', () => ({
  getFeedData: getFeedDataMock,
  makeFeedKey: makeFeedKeyMock,
  splitFeedKey: vi.fn((feedKey = '') => {
    const raw = String(feedKey || '').trim();
    const idx = raw.indexOf('::');
    if (idx === -1) return { feedId: raw, conversationId: '' };
    return { conversationId: raw.slice(0, idx), feedId: raw.slice(idx + 2) };
  }),
  onFeedDataChange: vi.fn(() => () => {}),
  getActiveFeeds: vi.fn(() => []),
  onFeedChange: vi.fn(() => () => {}),
}));

vi.mock('./ToolFeedBar', () => ({
  isFeedExpanded: vi.fn((feedId) => feedId === 'conv-1::plan'),
  getSelectedFeedId: vi.fn(() => 'conv-1::plan'),
  onSelectedFeedChange: vi.fn(() => () => {}),
}));

vi.mock('../services/planFeedBus', () => ({
  usePlanFeed: vi.fn(() => ({
    conversationId: 'conv-1',
    explanation: 'Inspect package and add a focused test.',
    steps: [
      { id: 's1', step: 'Inspect package', status: 'completed' },
      { id: 's2', step: 'Add test', status: 'in_progress' },
    ],
  })),
}));

vi.mock('../services/queueSyncBus', () => ({
  getQueueSyncSnapshot: vi.fn(() => ({
    conversationId: 'conv-1',
    queuedTurns: [],
    suppressedTurnIds: [],
    updatedAt: Date.now(),
  })),
}));

describe('resolveRootFeedDataSourceName', () => {
  it('prefers an explicit output source over object key order', () => {
    const name = resolveRootFeedDataSourceName({
      planDetail: {
        dataSourceRef: 'planInfo',
        selectors: { data: 'plan' }
      },
      planInfo: {
        source: 'output'
      }
    });

    expect(name).toBe('planInfo');
  });
});

describe('ToolFeedDetail', () => {
  it('renders the plan feed as a visible detail panel', async () => {
    const { default: ToolFeedDetail } = await import('./ToolFeedDetail.jsx');
    const html = renderToStaticMarkup(React.createElement(ToolFeedDetail));

    expect(html).toContain('Inspect package and add a focused test.');
    expect(html).toContain('Inspect package');
    expect(html).toContain('Add test');
  });

  it('falls back to fetched plan feed payload when the plan bus is empty', async () => {
    const { default: ToolFeedDetail } = await import('./ToolFeedDetail.jsx');
    const { usePlanFeed } = await import('../services/planFeedBus');

    usePlanFeed.mockReturnValueOnce({
      conversationId: 'conv-1',
      explanation: '',
      steps: [],
    });
    getFeedDataMock.mockImplementation(() => ({
      data: {
        output: {
          explanation: 'Hierarchy resolved successfully with campaign and agency names.',
          plan: [
            { status: 'completed', step: 'Resolve canonical hierarchy' },
            { status: 'in_progress', step: 'Pull campaign-level pacing metrics' },
          ]
        }
      }
    }));

    const html = renderToStaticMarkup(React.createElement(ToolFeedDetail));

    expect(html).toContain('Hierarchy resolved successfully with campaign and agency names.');
    expect(html).toContain('Resolve canonical hierarchy');
    expect(html).toContain('Pull campaign-level pacing metrics');
  });

  it('renders the queue feed detail when queue feed is expanded', async () => {
    const { default: ToolFeedDetail } = await import('./ToolFeedDetail.jsx');
    const toolFeedBar = await import('./ToolFeedBar');
    const queueSyncBus = await import('../services/queueSyncBus');

    toolFeedBar.isFeedExpanded.mockImplementation((feedId) => feedId === 'conv-1::queue');
    toolFeedBar.getSelectedFeedId.mockImplementation(() => 'conv-1::queue');
    queueSyncBus.getQueueSyncSnapshot.mockReturnValue({
      conversationId: 'conv-1',
      queuedTurns: [
        { id: 'turn-q1', preview: 'queued follow-up one' },
        { id: 'turn-q2', preview: 'queued follow-up two' },
      ],
      suppressedTurnIds: [],
      updatedAt: Date.now(),
    });

    const html = renderToStaticMarkup(React.createElement(ToolFeedDetail, {
      context: {
        Context(name) {
          if (name === 'conversations') {
            return {
              handlers: {
                dataSource: {
                  peekFormData: () => ({ id: 'conv-1' }),
                },
              },
            };
          }
          return null;
        },
      },
    }));

    expect(html).toContain('2 queued requests');
    expect(html).toContain('queued follow-up one');
    expect(html).toContain('queued follow-up two');
  });
});
