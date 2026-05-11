import { describe, expect, it, vi, beforeEach } from 'vitest';

const mocks = vi.hoisted(() => ({
  applyFeedEvent: vi.fn(),
  isFeedInactive: vi.fn(() => false),
}));

vi.mock('./agentlyClient', () => ({
  client: {
    getTranscript: vi.fn(),
  },
}));

vi.mock('./toolFeedBus', () => ({
  applyFeedEvent: mocks.applyFeedEvent,
  clearFeedState: vi.fn(),
  isFeedInactive: mocks.isFeedInactive,
}));

import { client } from './agentlyClient';
import { fetchTranscript } from './chatRuntime';

describe('fetchTranscript', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.isFeedInactive.mockReturnValue(false);
  });

  it('requests transcript feeds for history-mode activation without replaying them directly', async () => {
    client.getTranscript.mockResolvedValue({
      feeds: [
        {
          feedId: 'explorer',
          title: 'Explorer',
          itemCount: 3,
          data: { output: { files: [{ Path: 'bitset.go' }] } },
        },
        {
          feedId: 'terminal',
          title: 'Terminal',
          itemCount: 1,
          data: { output: { commands: [{ input: 'pwd', output: '/tmp' }] } },
        },
        {
          feedId: 'changes',
          title: 'Changes',
          itemCount: 1,
          data: { output: { changes: [{ url: '/tmp/sample_test.go', kind: 'create' }] } },
        },
      ],
      turns: [
        {
          turnId: 'turn-1',
          execution: {
            pages: [],
          },
        },
      ],
    });

    await fetchTranscript('conv-history-1');

    expect(client.getTranscript).toHaveBeenCalledWith(expect.objectContaining({
      conversationId: 'conv-history-1',
      includeFeeds: true,
    }), undefined);

    expect(mocks.applyFeedEvent).not.toHaveBeenCalled();
  });

  it('does not resurrect an inactive feed from transcript hydration', async () => {
    mocks.isFeedInactive.mockImplementation((feedId, conversationId) => (
      String(feedId) === 'plan' && String(conversationId) === 'conv-history-2'
    ));

    client.getTranscript.mockResolvedValue({
      feeds: [
        {
          feedId: 'plan',
          title: 'Plan',
          itemCount: 3,
          data: { output: { rows: [{ step: 'x' }] } },
        },
        {
          feedId: 'terminal',
          title: 'Terminal',
          itemCount: 1,
          data: { output: { commands: [{ input: 'pwd', output: '/tmp' }] } },
        },
      ],
      turns: [
        {
          turnId: 'turn-2',
          execution: {
            pages: [],
          },
        },
      ],
    });

    await fetchTranscript('conv-history-2');

    expect(mocks.applyFeedEvent).not.toHaveBeenCalled();
  });

  it('still requests feeds when execution details are disabled', async () => {
    client.getTranscript.mockResolvedValue({
      feeds: [
        {
          feedId: 'plan',
          title: 'Plan',
          itemCount: 1,
          data: { output: { rows: [{ step: 'Inspect repo' }] } },
        },
      ],
      turns: [
        {
          turnId: 'turn-3',
          execution: {
            pages: [],
          },
        },
      ],
    });

    await fetchTranscript('conv-history-3', '', { includeExecutionDetails: false });

    expect(client.getTranscript).toHaveBeenCalledWith(expect.objectContaining({
      conversationId: 'conv-history-3',
      includeModelCalls: false,
      includeToolCalls: false,
      includeFeeds: true,
    }), undefined);
    expect(mocks.applyFeedEvent).not.toHaveBeenCalled();
  });
});
