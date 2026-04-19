import { describe, expect, it, vi, beforeEach } from 'vitest';

const mocks = vi.hoisted(() => ({
  applyFeedEvent: vi.fn(),
}));

vi.mock('./agentlyClient', () => ({
  client: {
    getTranscript: vi.fn(),
  },
}));

vi.mock('./toolFeedBus', () => ({
  applyFeedEvent: mocks.applyFeedEvent,
  clearFeedState: vi.fn(),
}));

import { client } from './agentlyClient';
import { fetchTranscript } from './chatRuntime';

describe('fetchTranscript', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('rehydrates tool feeds from transcript data for history-mode chat activation', async () => {
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

    expect(mocks.applyFeedEvent).toHaveBeenCalledTimes(3);
    expect(mocks.applyFeedEvent).toHaveBeenNthCalledWith(1, expect.objectContaining({
      type: 'tool_feed_active',
      feedId: 'explorer',
      feedTitle: 'Explorer',
      feedItemCount: 3,
      conversationId: 'conv-history-1',
    }));
    expect(mocks.applyFeedEvent).toHaveBeenNthCalledWith(2, expect.objectContaining({
      type: 'tool_feed_active',
      feedId: 'terminal',
      feedTitle: 'Terminal',
      feedItemCount: 1,
      conversationId: 'conv-history-1',
    }));
    expect(mocks.applyFeedEvent).toHaveBeenNthCalledWith(3, expect.objectContaining({
      type: 'tool_feed_active',
      feedId: 'changes',
      feedTitle: 'Changes',
      feedItemCount: 1,
      conversationId: 'conv-history-1',
    }));
  });
});
