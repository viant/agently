import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';

const getActiveFeedsMock = vi.hoisted(() => vi.fn(() => []));
const getSelectedFeedIdMock = vi.hoisted(() => vi.fn(() => ''));
const fetchFeedDataNowMock = vi.hoisted(() => vi.fn());

vi.mock('../services/toolFeedBus', () => ({
  getActiveFeeds: getActiveFeedsMock,
  fetchFeedDataNow: fetchFeedDataNowMock,
  onFeedChange: vi.fn(() => () => {}),
  splitFeedKey: vi.fn((feedKey = '') => {
    const raw = String(feedKey || '').trim();
    const idx = raw.indexOf('::');
    if (idx === -1) return { feedId: raw, conversationId: '' };
    return { conversationId: raw.slice(0, idx), feedId: raw.slice(idx + 2) };
  }),
}));

vi.mock('../services/toolFeedSelection', () => ({
  activateExclusiveFeed: vi.fn(),
  getSelectedFeedId: getSelectedFeedIdMock,
  onSelectedFeedChange: vi.fn(() => () => {}),
}));

vi.mock('./ToolFeedDetail.jsx', () => ({
  default: ({ conversationId = '', variant = 'inline' }) => (
    React.createElement(
      'div',
      {
        className: 'tool-feed-detail',
        'data-conversation-id': conversationId,
        'data-variant': variant,
      },
      `detail:${conversationId}:${variant}`
    )
  ),
}));

import ToolFeedWorkspace, { filterWorkspaceFeeds, isStackedToolFeedViewport, sortWorkspaceFeeds } from './ToolFeedWorkspace.jsx';

describe('sortWorkspaceFeeds', () => {
  it('preserves incoming feed order instead of applying hardcoded priorities', () => {
    const result = sortWorkspaceFeeds([
      { feedId: 'conv-1::explorer', rawFeedId: 'explorer', title: 'Explorer' },
      { feedId: 'conv-1::changes', rawFeedId: 'changes', title: 'Changes' },
      { feedId: 'conv-1::terminal', rawFeedId: 'terminal', title: 'Terminal' },
      { feedId: 'conv-1::plan', rawFeedId: 'plan', title: 'Plan' },
    ]);

    expect(result.map((item) => item.rawFeedId)).toEqual(['explorer', 'changes', 'terminal', 'plan']);
  });
});

describe('isStackedToolFeedViewport', () => {
  it('uses a launcher/drawer until chat and a 320px rail both have practical width', () => {
    expect(isStackedToolFeedViewport(820)).toBe(true);
    expect(isStackedToolFeedViewport(1100)).toBe(true);
    expect(isStackedToolFeedViewport(1101)).toBe(false);
  });
});

describe('filterWorkspaceFeeds', () => {
  it('keeps only feeds for the active conversation and sorts them', () => {
    const result = filterWorkspaceFeeds([
      { feedId: 'conv-2::terminal', rawFeedId: 'terminal', title: 'Terminal', conversationId: 'conv-2' },
      { feedId: 'conv-1::changes', rawFeedId: 'changes', title: 'Changes', conversationId: 'conv-1' },
      { feedId: 'conv-1::plan', rawFeedId: 'plan', title: 'Plan', conversationId: 'conv-1' },
    ], 'conv-1');

    expect(result.map((item) => item.rawFeedId)).toEqual(['changes', 'plan']);
  });

  it('shows feeds by default and gates only explicitly developer-only feeds', () => {
    const feeds = [
      { feedId: 'conv-1::plan', title: 'Plan', conversationId: 'conv-1' },
      { feedId: 'conv-1::terminal', title: 'Terminal', conversationId: 'conv-1', developerOnly: true },
    ];
    expect(filterWorkspaceFeeds(feeds, 'conv-1', false).map((feed) => feed.title)).toEqual(['Plan']);
    expect(filterWorkspaceFeeds(feeds, 'conv-1', true).map((feed) => feed.title)).toEqual(['Plan', 'Terminal']);
  });
});

describe('ToolFeedWorkspace', () => {
  it('renders nothing when no active feeds are present', () => {
    getActiveFeedsMock.mockReturnValueOnce([]);
    getSelectedFeedIdMock.mockReturnValueOnce('');

    const html = renderToStaticMarkup(React.createElement(ToolFeedWorkspace, { conversationId: 'conv-1' }));

    expect(html).toBe('');
  });

  it('renders a compact tool-feed strip with tabs for visible workspace feeds', () => {
    getActiveFeedsMock.mockReturnValueOnce([
      { feedId: 'conv-1::terminal', rawFeedId: 'terminal', title: 'Terminal', conversationId: 'conv-1', itemCount: 2 },
      { feedId: 'conv-1::changes', rawFeedId: 'changes', title: 'Changes', conversationId: 'conv-1', itemCount: 1 },
      { feedId: 'conv-1::plan', rawFeedId: 'plan', title: 'Plan', conversationId: 'conv-1', itemCount: 3 },
    ]);
    getSelectedFeedIdMock.mockReturnValueOnce('conv-1::terminal');

    const html = renderToStaticMarkup(React.createElement(ToolFeedWorkspace, { conversationId: 'conv-1' }));

    expect(html).toContain('Tool feeds');
    expect(html).toContain('Plan');
    expect(html).toContain('Terminal');
    expect(html).toContain('Changes');
    expect(html).toContain('Close tool feeds');
    expect(html).toContain('Collapse Tool feeds to header');
    expect(html).toContain('Maximize Tool feeds');
    expect(html).toContain('detail:conv-1:rail');
  });

  it('uses the conversation-scoped selected feed id for the active rail', () => {
    getActiveFeedsMock.mockReturnValueOnce([
      { feedId: 'conv-1::plan', rawFeedId: 'plan', title: 'Plan', conversationId: 'conv-1', itemCount: 1 },
      { feedId: 'conv-1::terminal', rawFeedId: 'terminal', title: 'Terminal', conversationId: 'conv-1', itemCount: 1 },
    ]);
    getSelectedFeedIdMock.mockImplementation((conversationId = '') => (
      conversationId === 'conv-1' ? 'conv-1::terminal' : 'conv-2::changes'
    ));

    const html = renderToStaticMarkup(React.createElement(ToolFeedWorkspace, { conversationId: 'conv-1' }));

    expect(html).toContain('app-tool-workspace-tab is-active');
    expect(html).toContain('Terminal');
  });

  it('renders a compact top launcher without mounting the full feed body', () => {
    getActiveFeedsMock.mockReturnValueOnce([
      { feedId: 'conv-1::plan', rawFeedId: 'plan', title: 'Plan', conversationId: 'conv-1', itemCount: 3 },
    ]);
    getSelectedFeedIdMock.mockReturnValueOnce('conv-1::plan');
    const html = renderToStaticMarkup(React.createElement(ToolFeedWorkspace, {
      conversationId: 'conv-1',
      stackedOverride: true,
    }));
    expect(html).toContain('is-compact-launcher');
    expect(html).toContain('Open Tool feeds drawer');
    expect(html).not.toContain('detail:conv-1:rail');
  });

  it('renders a reopen affordance when feeds are dismissed', () => {
    getActiveFeedsMock.mockReturnValueOnce([
      { feedId: 'conv-1::plan', rawFeedId: 'plan', title: 'Plan', conversationId: 'conv-1', itemCount: 1 },
      { feedId: 'conv-1::terminal', rawFeedId: 'terminal', title: 'Terminal', conversationId: 'conv-1', itemCount: 1 },
    ]);
    getSelectedFeedIdMock.mockReturnValueOnce('conv-1::plan');

    const html = renderToStaticMarkup(React.createElement(ToolFeedWorkspace, {
      conversationId: 'conv-1',
      initialDismissed: true,
    }));

    expect(html).toContain('Reopen Tool feeds');
    expect(html).toContain('2 active');
    expect(html).not.toContain('detail:conv-1:rail');
  });
});
