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
    getTranscript: vi.fn(),
    query: vi.fn(),
  },
}));

vi.mock('./chatStore', () => ({
  onTranscript: vi.fn(),
  reset: vi.fn(),
  submit: vi.fn(),
  steer: vi.fn(),
}));

vi.mock('./chatRuntime', () => ({
  applyIterationVisibility: vi.fn(),
  bindConversationWindowEvents: vi.fn(),
  bootstrapConversationSelection: vi.fn(),
  cacheSettledConversationBootstrapSnapshot: vi.fn(),
  clearPendingConversationBootstrap: vi.fn(),
  createNewConversation: vi.fn(),
  dsTick: vi.fn(),
  disconnectStream: vi.fn(),
  ensureContextResources: vi.fn(),
  ensureConversation: vi.fn(),
  clearSettledConversationBootstrapSnapshot: vi.fn(),
  fetchConversation: vi.fn(),
  fetchPendingElicitations: vi.fn(),
  getSettledConversationBootstrapSnapshot: vi.fn(() => null),
  getVisibleIterations: vi.fn(),
  hasPendingConversationBootstrap: vi.fn(() => false),
  hydrateMeta: vi.fn(),
  hydrateConversationFromBootstrapSnapshot: vi.fn(() => false),
  isConversationLiveish: vi.fn(),
  logExecutorDebug: vi.fn(),
  logStreamDebug: vi.fn(),
  markPendingConversationBootstrap: vi.fn(),
  mapTranscriptToRows: vi.fn(() => ({ queuedTurns: [] })),
  normalizeMetaResponse: vi.fn(),
  publishActiveConversation: vi.fn(),
  publishConversationMetaUpdated: vi.fn(),
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
import { onTranscript as applyTranscriptToChatStore, reset as resetChatStoreConversation, submit as submitToChatStore, steer as steerToChatStore } from './chatStore';
import { onInit, submitMessage } from './chatService';
import { listLookupRegistry } from '../components/lookups/client.js';
import {
  dsTick,
  ensureContextResources,
  ensureConversation,
  clearPendingConversationBootstrap,
  clearSettledConversationBootstrapSnapshot,
  fetchConversation,
  fetchPendingElicitations,
  getSettledConversationBootstrapSnapshot,
  hasPendingConversationBootstrap,
  isConversationLiveish,
  hydrateConversationFromBootstrapSnapshot,
  markPendingConversationBootstrap,
  publishActiveConversation,
  rememberSeedTitle,
  resolveUserID,
  disconnectStream,
  syncConversationTransport,
} from './chatRuntime';

describe('submitMessage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getSettledConversationBootstrapSnapshot.mockReturnValue(null);
    hydrateConversationFromBootstrapSnapshot.mockReturnValue(false);
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

    await new Promise((resolve) => setTimeout(resolve, 0));
    await new Promise((resolve) => setTimeout(resolve, 0));
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

  it('persists displayQuery to transcript while sending structured planner context to the agent', async () => {
    client.query.mockResolvedValue({});
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
                  tool: ['llm/agents:list'],
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
      message: {
        content: 'Handle the planner submit event using the structured plannerSubmitEvent context.',
        displayQuery: 'Submit selected site recommendations.',
        context: {
          plannerSubmitEvent: {
            eventName: 'site_list_planner_submit',
            tableId: 'site-review',
            plannerSubmit: {
              domain: 'site_list',
              submitIntent: 'submit_selected',
              selectedKeys: ['publisher_id', 'site_id'],
              toolGuidance: {
                tool: 'steward-RecommendationPatch',
                toolBundle: 'analyst-sitelist-tools',
                useSelectedRowsOnly: true,
              },
            },
            selectedRows: [{ publisher_id: 37, site_id: 3945613211 }],
          },
        },
      },
      model: 'openai_gpt-5_4',
      agent: 'steward',
    });

    expect(submitToChatStore).toHaveBeenCalledWith(expect.objectContaining({
      conversationId: 'conv-1',
      content: 'Submit selected site recommendations.',
    }));
    expect(client.query).toHaveBeenCalledWith(expect.objectContaining({
      conversationId: 'conv-1',
      query: 'Handle the planner submit event using the structured plannerSubmitEvent context.',
      displayQuery: 'Submit selected site recommendations.',
      context: expect.objectContaining({
        plannerSubmitEvent: {
          eventName: 'site_list_planner_submit',
          tableId: 'site-review',
            plannerSubmit: {
              domain: 'site_list',
              submitIntent: 'submit_selected',
              selectedKeys: ['publisher_id', 'site_id'],
              toolGuidance: {
                tool: 'steward-RecommendationPatch',
                toolBundle: 'analyst-sitelist-tools',
                useSelectedRowsOnly: true,
              },
            },
          selectedRows: [{ publisher_id: 37, site_id: 3945613211 }],
        },
      }),
      tools: ['llm/agents:list', 'steward-RecommendationPatch'],
      toolBundles: ['analyst-sitelist-tools'],
    }));
  });

  it('resets canonical chatStore state before transcript hydration on a fast completed query', async () => {
    client.query.mockResolvedValue({ content: 'done', turnId: 'turn-fast', messageId: 'assistant-final' });
    client.getTranscript.mockResolvedValue({
      conversation: {
        conversationId: 'conv-1',
        turns: [{ turnId: 'turn-1', status: 'completed' }],
      },
    });
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
      streamTracker: { reset: vi.fn(), canonicalState: { activeTurnId: 'turn-fast' } },
    };
    ensureContextResources.mockReturnValue(chatState);
    fetchConversation.mockResolvedValue({ id: 'conv-1', status: 'succeeded' });
    fetchPendingElicitations.mockResolvedValue([]);
    isConversationLiveish.mockReturnValue(false);
    dsTick.mockResolvedValue({ conversationID: 'conv-1', hasRunning: false });

    const convForm = {};
    const context = {
      identity: { windowId: 'chat/new' },
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
      message: 'show order 2656980',
      model: 'openai_gpt-5_4',
      agent: 'steward',
    });

    expect(resetChatStoreConversation).toHaveBeenCalledWith('conv-1');
    expect(chatState.streamTracker.reset).toHaveBeenCalledTimes(1);
    expect(applyTranscriptToChatStore).toHaveBeenCalledWith('conv-1', expect.objectContaining({
      conversationId: 'conv-1',
    }));
    expect(fetchPendingElicitations).toHaveBeenCalledWith('conv-1');
    expect(dsTick).toHaveBeenCalledWith(context, {
      conversationID: 'conv-1',
      prefetchedTranscriptTurns: [{ turnId: 'turn-1', status: 'completed' }],
      prefetchedPendingElicitations: [],
    });
    expect(chatState.pendingTerminalRefreshSuppressionTurnID).toBe('turn-fast');
    expect(chatState.prefetchedTerminalTurnID).toBe('turn-1');
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
    global.window = { innerWidth: 640, __forgeUIBridgeClientId: 'client-web-123' };

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
        uiClientId: 'client-web-123',
      },
    }));
  });

  it('steers into the active running turn instead of queueing a follow-up turn', async () => {
    client.query.mockResolvedValue({});
    client.steerTurn = vi.fn().mockResolvedValue({ status: 'accepted', turnId: 'turn-1' });
    ensureConversation.mockResolvedValue('conv-steer');
    resolveUserID.mockReturnValue('');
    ensureContextResources.mockReturnValue({
      runningTurnId: 'turn-1',
      activeStreamTurnId: '',
      lastHasRunning: true,
      activeConversationID: '',
      liveOwnedConversationID: '',
      activeStreamPrompt: '',
      activeStreamTurnId: '',
      activeStreamStartedAt: 0,
    });
    dsTick.mockResolvedValue({
      conversationID: 'conv-steer',
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

    await submitMessage({
      context,
      message: 'Focus only on bid and floor evidence.',
      model: 'openai_gpt-5_4',
      agent: 'steward',
    });

    expect(steerToChatStore).toHaveBeenCalledWith(expect.objectContaining({
      conversationId: 'conv-steer',
      content: 'Focus only on bid and floor evidence.',
    }));
    expect(client.steerTurn).toHaveBeenCalledWith('conv-steer', 'turn-1', {
      content: 'Focus only on bid and floor evidence.',
      role: 'user',
    });
    expect(client.query).not.toHaveBeenCalled();
    expect(submitToChatStore).not.toHaveBeenCalled();
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

  it('reconciles transcript immediately when query returns inline content for a fast completed turn', async () => {
    client.query.mockResolvedValue({
      conversationId: 'conv-fast',
      content: 'Your order summary is now open for ad order 2656980.',
    });
    ensureConversation.mockResolvedValue('conv-fast');
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
      conversationID: 'conv-fast',
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
      message: 'show order 2656980',
      model: 'openai_gpt-5_4',
      agent: 'steward',
    });

    expect(fetchConversation).toHaveBeenCalledWith('conv-fast');
    expect(dsTick).toHaveBeenCalledWith(context, {
      conversationID: 'conv-fast',
      prefetchedTranscriptTurns: [{ turnId: 'turn-1', status: 'completed' }],
      prefetchedPendingElicitations: [],
    });
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
