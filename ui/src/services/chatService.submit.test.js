import { beforeEach, describe, expect, it, vi } from 'vitest';

const queryDeferred = () => {
  let resolve;
  let reject;
  const promise = new Promise((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
};

vi.mock('./agentlyClient', () => ({
  client: {
    query: vi.fn(),
  },
}));

vi.mock('./chatStore', () => ({
  submit: vi.fn(),
}));

vi.mock('./chatRuntime', () => ({
  applyIterationVisibility: vi.fn(),
  bindConversationWindowEvents: vi.fn(),
  bootstrapConversationSelection: vi.fn(),
  createNewConversation: vi.fn(),
  dsTick: vi.fn(),
  disconnectStream: vi.fn(),
  ensureContextResources: vi.fn(),
  ensureConversation: vi.fn(),
  fetchConversation: vi.fn(),
  fetchPendingElicitations: vi.fn(),
  getVisibleIterations: vi.fn(),
  hydrateMeta: vi.fn(),
  isConversationLiveish: vi.fn(),
  logStreamDebug: vi.fn(),
  mapTranscriptToRows: vi.fn(() => ({ queuedTurns: [] })),
  normalizeMetaResponse: vi.fn(),
  publishActiveConversation: vi.fn(),
  renderMergedRowsForContext: vi.fn(),
  rememberSeedTitle: vi.fn(),
  resolveUserID: vi.fn(),
  sanitizeAutoSelection: vi.fn((value) => value),
  syncConversationTransport: vi.fn(),
  startPolling: vi.fn(),
  stopPolling: vi.fn(),
  syncMessagesSnapshot: vi.fn(),
  unbindConversationWindowEvents: vi.fn(),
}));

vi.mock('./stageBus', () => ({
  setStage: vi.fn(),
}));

vi.mock('./httpClient', () => ({
  showToast: vi.fn(),
}));

vi.mock('../components/lookups/client.js', () => ({
  listLookupRegistry: vi.fn(),
}));

vi.mock('./toolFeedBus', () => ({
  getFeedData: vi.fn(),
  updateFeedData: vi.fn(),
}));

vi.mock('../utils/dialogBus', () => ({
  openCodeDiffDialog: vi.fn(),
  openFileViewDialog: vi.fn(),
  updateCodeDiffDialog: vi.fn(),
  updateFileViewDialog: vi.fn(),
}));

import { client } from './agentlyClient';
import { submit as submitToChatStore } from './chatStore';
import { onInit, submitMessage } from './chatService';
import { listLookupRegistry } from '../components/lookups/client.js';
import {
  dsTick,
  ensureContextResources,
  ensureConversation,
  fetchConversation,
  isConversationLiveish,
  publishActiveConversation,
  rememberSeedTitle,
  resolveUserID,
  disconnectStream,
  syncConversationTransport,
} from './chatRuntime';

describe('submitMessage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('attaches SSE before the query promise resolves for a new active turn', async () => {
    const deferred = queryDeferred();
    client.query.mockReturnValue(deferred.promise);
    ensureConversation.mockResolvedValue('conv-1');
    resolveUserID.mockReturnValue('');
    ensureContextResources.mockReturnValue({
      runningTurnId: '',
      lastHasRunning: false,
      activeConversationID: '',
      liveOwnedConversationID: '',
      activeStreamPrompt: '',
      activeStreamTurnId: '',
      activeStreamStartedAt: 0,
    });
    dsTick.mockResolvedValue({
      conversationID: 'conv-1',
      hasRunning: true,
    });

    const convForm = {};
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => convForm,
                setFormData: vi.fn(({ values }) => Object.assign(convForm, values)),
              },
            },
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({
                  defaults: { model: 'openai_gpt-5_4' },
                }),
              },
            },
          };
        }
        return null;
      },
    };

    const submitPromise = submitMessage({
      context,
      message: 'Forecast inventory and uniques for this targeting set: ad deal 147540',
      model: 'openai_gpt-5_4',
      agent: 'steward',
    });

    await Promise.resolve();
    expect(ensureConversation).toHaveBeenCalled();
    expect(submitToChatStore).toHaveBeenCalledWith(expect.objectContaining({
      conversationId: 'conv-1',
      content: 'Forecast inventory and uniques for this targeting set: ad deal 147540',
    }));
    expect(publishActiveConversation).toHaveBeenCalledWith('conv-1', context);
    expect(rememberSeedTitle).toHaveBeenCalledWith('conv-1', 'Forecast inventory and uniques for this targeting set: ad deal 147540');
    expect(syncConversationTransport).toHaveBeenCalledWith(context, 'conv-1');

    deferred.resolve({});
    await submitPromise;
  });

  it('does not run transcript dstick after submit when a fresh live stream owns the turn', async () => {
    client.query.mockResolvedValue({});
    ensureConversation.mockResolvedValue('conv-1');
    resolveUserID.mockReturnValue('');
    const chatState = {
      runningTurnId: '',
      lastHasRunning: false,
      activeConversationID: '',
      liveOwnedConversationID: '',
      activeStreamPrompt: '',
      activeStreamTurnId: '',
      activeStreamStartedAt: 0,
      stream: { close: vi.fn() },
    };
    ensureContextResources.mockReturnValue(chatState);

    const convForm = {};
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => convForm,
                setFormData: vi.fn(({ values }) => Object.assign(convForm, values)),
              },
            },
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({
                  defaults: { model: 'openai_gpt-5_4' },
                }),
              },
            },
          };
        }
        return null;
      },
    };

    await submitMessage({
      context,
      message: 'Analyze the business impact of deal 146901',
      model: 'openai_gpt-5_4',
      agent: 'steward',
    });

    expect(syncConversationTransport).toHaveBeenCalledWith(context, 'conv-1');
    expect(submitToChatStore).toHaveBeenCalledWith(expect.objectContaining({
      conversationId: 'conv-1',
      content: 'Analyze the business impact of deal 146901',
    }));
    expect(dsTick).not.toHaveBeenCalled();
  });

  it('adds explicit web client context to query payloads', async () => {
    client.query.mockResolvedValue({});
    ensureConversation.mockResolvedValue('conv-ctx');
    resolveUserID.mockReturnValue('');
    ensureContextResources.mockReturnValue({
      runningTurnId: '',
      lastHasRunning: false,
      activeConversationID: '',
      liveOwnedConversationID: '',
      activeStreamPrompt: '',
      activeStreamTurnId: '',
      activeStreamStartedAt: 0,
    });
    dsTick.mockResolvedValue({
      conversationID: 'conv-ctx',
      hasRunning: false,
    });

    const previousWindow = global.window;
    global.window = { innerWidth: 640 };

    const convForm = {};
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => convForm,
                setFormData: vi.fn(({ values }) => Object.assign(convForm, values)),
              },
            },
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({
                  defaults: { model: 'openai_gpt-5_4' },
                }),
              },
            },
          };
        }
        return null;
      },
    };

    try {
      await submitMessage({
        context,
        message: 'Summarize this workspace state',
        model: 'openai_gpt-5_4',
        agent: 'steward',
      });
    } finally {
      global.window = previousWindow;
    }

    expect(client.query).toHaveBeenCalledWith(expect.objectContaining({
      context: {
        client: {
          kind: 'web',
          platform: 'web',
          formFactor: 'phone',
          surface: 'browser',
          capabilities: ['markdown', 'chart', 'upload', 'code', 'diff'],
        },
      },
    }));
  });

  it('submits unresolved required starter lookups by preserving the /name token for the model', async () => {
    listLookupRegistry.mockResolvedValue([
      {
        name: 'order',
        required: true,
        token: {
          queryInput: 'AdOrderName',
          resolveInput: 'AdOrderId',
          modelForm: '${id}',
        },
      },
    ]);
    client.query.mockResolvedValue({});
    ensureConversation.mockResolvedValue('conv-unresolved');
    resolveUserID.mockReturnValue('');
    ensureContextResources.mockReturnValue({
      runningTurnId: '',
      lastHasRunning: false,
      activeConversationID: '',
      liveOwnedConversationID: '',
      activeStreamPrompt: '',
      activeStreamTurnId: '',
      activeStreamStartedAt: 0,
    });
    dsTick.mockResolvedValue({
      conversationID: 'conv-unresolved',
      hasRunning: false,
    });

    const convForm = {};
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => convForm,
                setFormData: vi.fn(({ values }) => Object.assign(convForm, values)),
              },
            },
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({
                  defaults: { model: 'openai_gpt-5_4' },
                }),
              },
            },
          };
        }
        return null;
      },
    };

    await submitMessage({
      context,
      message: 'Troubleshoot @{order:? "Order"} order for delivery issues.',
      model: 'openai_gpt-5_4',
      agent: 'steward',
    });

    expect(client.query).toHaveBeenCalledWith(expect.objectContaining({
      query: 'Troubleshoot /order order for delivery issues.',
    }));
  });

  it('attaches SSE on init when conversation metadata says the conversation is still live', async () => {
    fetchConversation.mockResolvedValue({
      id: 'conv-1',
      stage: 'executing',
      status: 'running',
      title: 'Live conversation'
    });
    isConversationLiveish.mockReturnValue(true);
    dsTick.mockResolvedValue({
      conversationID: 'conv-1',
      hasRunning: false,
    });

    const convForm = { id: 'conv-1' };
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => convForm,
                setFormData: vi.fn(({ values }) => Object.assign(convForm, values)),
              },
            },
          };
        }
        if (name === 'messages') {
          return {
            handlers: {
              dataSource: {
                setCollection: vi.fn(),
                setError: vi.fn()
              },
            },
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData: () => ({ defaults: {} }),
              },
            },
          };
        }
        return null;
      },
    };

    await onInit({ context });

    expect(fetchConversation).toHaveBeenCalledWith('conv-1');
    expect(dsTick).toHaveBeenCalledWith(context, expect.objectContaining({ conversationID: 'conv-1' }));
    expect(syncConversationTransport).toHaveBeenCalledWith(context, 'conv-1');
    expect(disconnectStream).not.toHaveBeenCalled();
  });
});
