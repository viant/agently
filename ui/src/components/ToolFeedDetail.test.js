import React from 'react';
import fs from 'node:fs';
import { describe, expect, it, vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';

import { resolveRootFeedDataSourceName } from './ToolFeedDetail.jsx';

const { getFeedDataMock, makeFeedKeyMock } = vi.hoisted(() => ({
  getFeedDataMock: vi.fn(() => null)
  ,makeFeedKeyMock: vi.fn((feedId, conversationId = '') => conversationId ? `${conversationId}::${feedId}` : feedId)
}));

const getActiveFeedsMock = vi.hoisted(() => vi.fn(() => []));

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
  getActiveFeeds: getActiveFeedsMock,
  onFeedChange: vi.fn(() => () => {}),
}));

vi.mock('./ToolFeedBar', () => ({
  isFeedExpanded: vi.fn((feedId) => feedId === 'conv-1::plan'),
  getSelectedFeedId: vi.fn(() => 'conv-1::plan'),
  onSelectedFeedChange: vi.fn(() => () => {}),
}));

vi.mock('forge/components', () => ({
  Container: ({ container }) => React.createElement(
    'div',
    { 'data-testid': 'forge-container' },
    JSON.stringify(container || {})
  ),
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
    getActiveFeedsMock.mockReturnValueOnce([
      {
        feedId: 'conv-1::plan',
        rawFeedId: 'plan',
        title: 'Plan',
        conversationId: 'conv-1',
      },
    ]);
    getFeedDataMock.mockImplementation(() => ({
      data: {
        output: {
          explanation: 'Inspect package and add a focused test.',
          plan: [
            { id: 's1', step: 'Inspect package', status: 'completed' },
            { id: 's2', step: 'Add test', status: 'in_progress' },
          ],
        },
      },
      ui: {
        dataSources: {
          planInfo: { source: 'output' },
          planDetail: { dataSourceRef: 'planInfo', selectors: { data: 'plan' } },
        },
        containers: [
          {
            id: 'header',
            dataSourceRef: 'planInfo',
            items: [
              { id: 'explanation', type: 'label', dataBind: 'explanation' },
            ],
          },
          {
            id: 'planTable',
            type: 'table',
            dataSourceRef: 'planDetail',
            table: {
              columns: [
                { id: 'status', name: 'Status', width: 30 },
                { id: 'step', name: 'Step', width: 200 },
              ],
            },
          },
        ],
      },
      dataSources: {
        planInfo: { source: 'output' },
        planDetail: { dataSourceRef: 'planInfo', selectors: { data: 'plan' } },
      },
    }));
    const html = renderToStaticMarkup(React.createElement(ToolFeedDetail));

    expect(html).toContain('data-testid="forge-container"');
    expect(html).toContain('planInfo');
    expect(html).toContain('planDetail');
    expect(html).toContain('planTable');
    expect(html).not.toContain('explanation');
    expect(html).not.toContain('"title":"Plan"');
  });

  it('renders nothing when feeds exist but none are expanded', async () => {
    const { default: ToolFeedDetail } = await import('./ToolFeedDetail.jsx');
    const toolFeedBus = await import('../services/toolFeedBus');
    const toolFeedBar = await import('./ToolFeedBar');

    toolFeedBus.getActiveFeeds.mockReturnValueOnce([
      {
        feedId: 'conv-1::plan',
        rawFeedId: 'plan',
        conversationId: 'conv-1',
        title: 'Plan',
      },
    ]);
    toolFeedBus.getFeedData.mockReturnValueOnce({
      data: { output: { rows: [{ status: 'completed', step: 'one' }] } },
      ui: { title: 'Plan' },
      dataSources: { output: { source: 'output' } },
      _conversationId: 'conv-1',
    });
    toolFeedBar.isFeedExpanded.mockImplementation(() => false);
    toolFeedBar.getSelectedFeedId.mockImplementation(() => '');

    const html = renderToStaticMarkup(React.createElement(ToolFeedDetail));
    expect(html).toBe('');
  });

  it('falls back to fetched plan feed payload when the plan bus is empty', async () => {
    const { default: ToolFeedDetail } = await import('./ToolFeedDetail.jsx');
    const toolFeedBar = await import('./ToolFeedBar');
    getActiveFeedsMock.mockReturnValueOnce([
      {
        feedId: 'conv-1::plan',
        rawFeedId: 'plan',
        title: 'Plan',
        conversationId: 'conv-1',
      },
    ]);
    getFeedDataMock.mockImplementation(() => ({
      data: {
        output: {
          explanation: 'Hierarchy resolved successfully with campaign and agency names.',
          plan: [
            { status: 'completed', step: 'Resolve canonical hierarchy' },
            { status: 'in_progress', step: 'Pull campaign-level pacing metrics' },
          ]
        }
      },
      ui: {
        dataSources: {
          planInfo: { source: 'output' },
          planDetail: { dataSourceRef: 'planInfo', selectors: { data: 'plan' } },
        },
        containers: [
          {
            id: 'header',
            dataSourceRef: 'planInfo',
            items: [
              { id: 'explanation', type: 'label', dataBind: 'explanation' },
            ],
          },
          {
            id: 'planTable',
            type: 'table',
            dataSourceRef: 'planDetail',
            table: {
              columns: [
                { id: 'status', name: 'Status', width: 30 },
                { id: 'step', name: 'Step', width: 200 },
              ],
            },
          },
        ],
      },
      dataSources: {
        planInfo: { source: 'output' },
        planDetail: { dataSourceRef: 'planInfo', selectors: { data: 'plan' } },
      },
    }));
    toolFeedBar.isFeedExpanded.mockImplementation((feedId) => feedId === 'conv-1::plan');
    toolFeedBar.getSelectedFeedId.mockImplementation(() => 'conv-1::plan');

    const html = renderToStaticMarkup(React.createElement(ToolFeedDetail));

    expect(html).toContain('data-testid="forge-container"');
    expect(html).toContain('planInfo');
    expect(html).toContain('planDetail');
    expect(html).toContain('planTable');
  });

  it('renders inline fallback for data-only feeds instead of a loading placeholder', async () => {
    const { default: ToolFeedDetail } = await import('./ToolFeedDetail.jsx');
    const toolFeedBar = await import('./ToolFeedBar');

    getActiveFeedsMock.mockReturnValueOnce([
      {
        feedId: 'conv-1::explorer',
        rawFeedId: 'explorer',
        title: 'Explorer',
        conversationId: 'conv-1',
      },
    ]);
    toolFeedBar.isFeedExpanded.mockImplementation((feedId) => feedId === 'conv-1::explorer');
    toolFeedBar.getSelectedFeedId.mockImplementation(() => 'conv-1::explorer');
    getFeedDataMock.mockImplementation(() => ({
      data: {
        output: {
          items: [
            { path: '/tmp/project/service.go', matches: 4 },
          ],
        },
      },
      _conversationId: 'conv-1',
    }));

    const html = renderToStaticMarkup(React.createElement(ToolFeedDetail));

    expect(html).not.toContain('Loading feed…');
    expect(html).toContain('/tmp/project/service.go');
  });

  it('renders nothing for feeds with no spec and no data instead of a loading placeholder', async () => {
    const { default: ToolFeedDetail } = await import('./ToolFeedDetail.jsx');
    const toolFeedBar = await import('./ToolFeedBar');

    getActiveFeedsMock.mockReturnValueOnce([
      {
        feedId: 'conv-1::explorer',
        rawFeedId: 'explorer',
        title: 'Explorer',
        conversationId: 'conv-1',
      },
    ]);
    toolFeedBar.isFeedExpanded.mockImplementation((feedId) => feedId === 'conv-1::explorer');
    toolFeedBar.getSelectedFeedId.mockImplementation(() => 'conv-1::explorer');
    getFeedDataMock.mockImplementation(() => ({
      data: {},
      _conversationId: 'conv-1',
    }));

    const html = renderToStaticMarkup(React.createElement(ToolFeedDetail));

    expect(html).toBe('');
  });

  it('renders the queue feed detail when queue feed is expanded', async () => {
    const { default: ToolFeedDetail } = await import('./ToolFeedDetail.jsx');
    const toolFeedBus = await import('../services/toolFeedBus');
    const toolFeedBar = await import('./ToolFeedBar');

    toolFeedBus.getActiveFeeds.mockReturnValueOnce([
      {
        feedId: 'conv-1::queue',
        rawFeedId: 'queue',
        title: 'Queue',
        conversationId: 'conv-1',
      },
    ]);
    toolFeedBar.isFeedExpanded.mockImplementation((feedId) => feedId === 'conv-1::queue');
    toolFeedBar.getSelectedFeedId.mockImplementation(() => 'conv-1::queue');
    getFeedDataMock.mockImplementation((feedId, conversationId) => {
      if (feedId === 'conv-1::queue' && conversationId === 'conv-1') {
        return {
          data: {
            output: {
              queuedTurns: [
                { id: 'turn-q1', preview: 'queued follow-up one', status: 'queued' },
                { id: 'turn-q2', preview: 'queued follow-up two', status: 'queued' },
              ],
            },
          },
          ui: {
            dataSources: {
              queueTurns: { source: 'output.queuedTurns' },
            },
            containers: [
              {
                id: 'queueTable',
                type: 'table',
                dataSourceRef: 'queueTurns',
                table: {
                  columns: [
                    { id: 'preview', name: 'Preview', width: 420 },
                    { id: 'status', name: 'Status', width: 120 },
                  ],
                },
                toolbar: {
                  items: [
                    { id: 'save', label: 'Save edit', icon: 'floppy-disk', align: 'right' },
                  ],
                },
              },
            ],
          },
          dataSources: {
            queueTurns: { source: 'output.queuedTurns' },
          },
        };
      }
      return null;
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

    expect(html).toContain('data-testid="forge-container"');
    expect(html).toContain('queueTable');
    expect(html).toContain('queueTurns');
    expect(html).toContain('Save edit');
  });

  it('uses an already-scoped feed id without double-scoping generic feed lookups', async () => {
    const { default: ToolFeedDetail } = await import('./ToolFeedDetail.jsx');
    const toolFeedBus = await import('../services/toolFeedBus');
    const toolFeedBar = await import('./ToolFeedBar');
    toolFeedBus.getActiveFeeds.mockReturnValueOnce([
      {
        feedId: 'conv-1::terminal',
        rawFeedId: 'terminal',
        title: 'Terminal',
        conversationId: 'conv-1',
      },
    ]);
    toolFeedBar.isFeedExpanded.mockImplementation((feedId) => feedId === 'conv-1::terminal');
    toolFeedBar.getSelectedFeedId.mockImplementation(() => 'conv-1::terminal');
    getFeedDataMock.mockImplementation((feedId, conversationId) => {
      if (feedId === 'conv-1::terminal' && conversationId === 'conv-1') {
        return {
          data: {
            output: {
              lines: ['pwd', '/Users/awitas/go/src/github.com/viant/xdatly'],
            },
          },
          ui: {
            dataSources: {
              output: { source: 'output' },
            },
          },
          dataSources: {
            output: { source: 'output' },
          },
        };
      }
      return null;
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

    expect(html).toContain('app-tool-feed-detail');
    expect(getFeedDataMock).toHaveBeenCalledWith('conv-1::terminal', 'conv-1');
  });

  it('renders the changes feed panel from transcript-backed feed data', async () => {
    const { default: ToolFeedDetail } = await import('./ToolFeedDetail.jsx');
    const toolFeedBus = await import('../services/toolFeedBus');
    const toolFeedBar = await import('./ToolFeedBar');

    toolFeedBus.getActiveFeeds.mockReturnValueOnce([
      {
        feedId: 'conv-1::changes',
        rawFeedId: 'changes',
        title: 'Changes',
        conversationId: 'conv-1',
      },
    ]);
    toolFeedBar.isFeedExpanded.mockImplementation((feedId) => feedId === 'conv-1::changes');
    toolFeedBar.getSelectedFeedId.mockImplementation(() => 'conv-1::changes');
    getFeedDataMock.mockImplementation((feedId, conversationId) => {
      if (feedId === 'conv-1::changes' && conversationId === 'conv-1') {
        return {
          data: {
            output: {
              changes: [
                { url: '/Users/awitas/go/src/github.com/viant/sample_test.go', kind: 'create' },
              ],
            },
          },
          ui: {
            dataSources: {
              snapshot: { source: 'output' },
              changes: { dataSourceRef: 'snapshot', selectors: { data: 'changes' } },
            },
          },
          dataSources: {
            snapshot: { source: 'output' },
            changes: { dataSourceRef: 'snapshot', selectors: { data: 'changes' } },
          },
        };
      }
      return null;
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

    expect(html).toContain('app-tool-feed-detail');
    expect(getFeedDataMock).toHaveBeenCalledWith('conv-1::changes', 'conv-1');
  });

  it('renders multiple expanded feeds together instead of tabbing them', async () => {
    const { default: ToolFeedDetail } = await import('./ToolFeedDetail.jsx');
    const toolFeedBar = await import('./ToolFeedBar');

    getActiveFeedsMock.mockReturnValueOnce([
      {
        feedId: 'conv-1::plan',
        rawFeedId: 'plan',
        title: 'Plan',
        conversationId: 'conv-1',
        itemCount: 2,
      },
      {
        feedId: 'conv-1::changes',
        rawFeedId: 'changes',
        title: 'Changes',
        conversationId: 'conv-1',
        itemCount: 1,
      },
    ]);
    toolFeedBar.isFeedExpanded.mockImplementation((feedId) => feedId === 'conv-1::plan' || feedId === 'conv-1::changes');
    toolFeedBar.getSelectedFeedId.mockImplementation(() => 'conv-1::changes');
    getFeedDataMock.mockImplementation((feedId) => {
      if (feedId === 'conv-1::plan') {
        return {
          data: { output: { plan: [{ step: 'Inspect package', status: 'completed' }] } },
          _conversationId: 'conv-1',
        };
      }
      if (feedId === 'conv-1::changes') {
        return {
          data: { output: { changes: [{ url: '/tmp/a.go', kind: 'create' }] } },
          _conversationId: 'conv-1',
        };
      }
      return null;
    });

    const html = renderToStaticMarkup(React.createElement(ToolFeedDetail));

    expect(html).toContain('app-tool-feed-detail-section-title');
    expect(html).toContain('Plan');
    expect(html).toContain('Changes');
    expect(html).not.toContain('tool-feed-tabs');
  });

  it('keeps changes and explorer feeds visually compact at the spec level', () => {
    const changesYaml = fs.readFileSync('/Users/awitas/go/src/github.com/viant/agently/bootstrap/defaults/feeds/changes.yaml', 'utf8');
    const explorerYaml = fs.readFileSync('/Users/awitas/go/src/github.com/viant/agently/bootstrap/defaults/feeds/explorer.yaml', 'utf8');

    expect(changesYaml).toContain('height: min(18vh, 180px)');
    expect(changesYaml).toContain('borderRadius: 10px');
    expect(explorerYaml).toContain('height: min(20vh, 220px)');
    expect(explorerYaml).toContain('borderRadius: 10px');
  });
});
