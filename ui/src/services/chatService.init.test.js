import { describe, expect, it, vi, beforeEach } from 'vitest';

const {
  setStageMock,
  bindConversationWindowEventsMock,
  bootstrapConversationSelectionMock,
  cacheSettledConversationBootstrapSnapshotMock,
  renderMergedRowsForContextMock,
  hydrateMetaMock,
  hydrateConversationFromBootstrapSnapshotMock,
  ensureContextResourcesMock,
  fetchConversationMock,
  getSettledConversationBootstrapSnapshotMock,
  dsTickMock,
  syncConversationTransportMock,
  disconnectStreamMock,
  logExecutorDebugMock,
  publishActiveConversationMock,
  publishConversationMetaUpdatedMock,
  fetchPendingElicitationsMock,
  refreshGoalFeedMock,
  hasPendingConversationBootstrapMock,
  startPollingMock,
  connectForgeUIActionsToCallbacksOrChatMock,
} = vi.hoisted(() => ({
  setStageMock: vi.fn(),
  bindConversationWindowEventsMock: vi.fn(),
  bootstrapConversationSelectionMock: vi.fn(),
  cacheSettledConversationBootstrapSnapshotMock: vi.fn(),
  renderMergedRowsForContextMock: vi.fn(),
  hydrateMetaMock: vi.fn(),
  hydrateConversationFromBootstrapSnapshotMock: vi.fn(() => false),
  ensureContextResourcesMock: vi.fn(() => ({})),
  fetchConversationMock: vi.fn(),
  getSettledConversationBootstrapSnapshotMock: vi.fn(() => null),
  dsTickMock: vi.fn(),
  syncConversationTransportMock: vi.fn(),
  disconnectStreamMock: vi.fn(),
  logExecutorDebugMock: vi.fn(),
  publishActiveConversationMock: vi.fn(),
  publishConversationMetaUpdatedMock: vi.fn(),
  fetchPendingElicitationsMock: vi.fn(),
  refreshGoalFeedMock: vi.fn(),
  hasPendingConversationBootstrapMock: vi.fn(() => false),
  startPollingMock: vi.fn(),
  connectForgeUIActionsToCallbacksOrChatMock: vi.fn(() => () => {}),
}));

vi.mock('./stageBus', () => ({
  setStage: setStageMock,
}));

vi.mock('./chatRuntime', () => ({
  bindConversationWindowEvents: bindConversationWindowEventsMock,
  bootstrapConversationSelection: bootstrapConversationSelectionMock,
  cacheSettledConversationBootstrapSnapshot: cacheSettledConversationBootstrapSnapshotMock,
  createNewConversation: vi.fn(),
  dsTick: dsTickMock,
  disconnectStream: disconnectStreamMock,
  ensureContextResources: ensureContextResourcesMock,
  ensureConversation: vi.fn(),
  fetchConversation: fetchConversationMock,
  fetchPendingElicitations: fetchPendingElicitationsMock,
  refreshGoalFeed: refreshGoalFeedMock,
  getSettledConversationBootstrapSnapshot: getSettledConversationBootstrapSnapshotMock,
  hasPendingConversationBootstrap: hasPendingConversationBootstrapMock,
  hydrateMeta: hydrateMetaMock,
  hydrateConversationFromBootstrapSnapshot: hydrateConversationFromBootstrapSnapshotMock,
  isConversationLiveish: vi.fn(() => false),
  logExecutorDebug: logExecutorDebugMock,
  logStreamDebug: vi.fn(),
  mapTranscriptToRows: vi.fn(),
  normalizeMetaResponse: vi.fn(),
  publishActiveConversation: publishActiveConversationMock,
  publishConversationMetaUpdated: publishConversationMetaUpdatedMock,
  renderMergedRowsForContext: renderMergedRowsForContextMock,
  rememberSeedTitle: vi.fn(),
  resolveUserID: vi.fn(),
  sanitizeAutoSelection: vi.fn((value) => String(value || '').trim()),
  syncConversationTransport: syncConversationTransportMock,
  startPolling: startPollingMock,
  stopPolling: vi.fn(),
  syncMessagesSnapshot: vi.fn(),
  unbindConversationWindowEvents: vi.fn(),
}));

vi.mock('./agentlyClient', () => ({
  client: {},
}));

vi.mock('./httpClient', () => ({
  showToast: vi.fn(),
}));

vi.mock('./forgeUIActions', () => ({
  connectForgeUIActionsToCallbacksOrChat: connectForgeUIActionsToCallbacksOrChatMock,
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

import { onInit } from './chatService';

describe('onInit', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    hydrateMetaMock.mockResolvedValue(undefined);
    hydrateConversationFromBootstrapSnapshotMock.mockReturnValue(false);
    getSettledConversationBootstrapSnapshotMock.mockReturnValue(null);
    hasPendingConversationBootstrapMock.mockReturnValue(false);
    fetchConversationMock.mockImplementation(() => new Promise(() => {}));
    fetchPendingElicitationsMock.mockResolvedValue([]);
    refreshGoalFeedMock.mockResolvedValue(undefined);
    dsTickMock.mockResolvedValue({ hasRunning: false });
  });

  it('marks the shell ready immediately after metadata hydration, before slow conversation bootstrap resolves', async () => {
    const conversationsDS = {
      peekFormData: () => ({ id: 'conv-1' }),
      setFormData: vi.fn(),
    };
    const messagesDS = {
      setCollection: vi.fn(),
      setError: vi.fn(),
    };
    const context = {
      Context(name) {
        if (name === 'conversations') return { handlers: { dataSource: conversationsDS } };
        if (name === 'messages') return { handlers: { dataSource: messagesDS } };
        if (name === 'meta') return { handlers: { dataSource: { peekFormData: () => ({ defaults: {} }) } } };
        return null;
      },
    };

    const initPromise = onInit({ context });
    await Promise.resolve();
    await Promise.resolve();

    expect(connectForgeUIActionsToCallbacksOrChatMock).toHaveBeenCalledTimes(1);
    expect(setStageMock).toHaveBeenNthCalledWith(1, { phase: 'waiting', text: 'Initializing…' });
    expect(setStageMock).toHaveBeenCalledWith({ phase: 'ready', text: 'Ready' });
    expect(renderMergedRowsForContextMock).toHaveBeenCalled();

    void initPromise;
  });

  it('hydrates an already-settled conversation from bootstrap cache without refetching conversation or transcript', async () => {
    const conversationsDS = {
      peekFormData: () => ({ id: 'conv-1' }),
      setFormData: vi.fn(),
    };
    const messagesDS = {
      setCollection: vi.fn(),
      setError: vi.fn(),
    };
    const context = {
      Context(name) {
        if (name === 'conversations') return { handlers: { dataSource: conversationsDS } };
        if (name === 'messages') return { handlers: { dataSource: messagesDS } };
        if (name === 'meta') return { handlers: { dataSource: { peekFormData: () => ({ defaults: {} }) } } };
        return null;
      },
    };

    getSettledConversationBootstrapSnapshotMock.mockReturnValue({
      conversation: { id: 'conv-1', status: 'succeeded' },
      turns: [],
      pendingElicitations: [],
      generatedFiles: []
    });
    hydrateConversationFromBootstrapSnapshotMock.mockReturnValue(true);

    await onInit({ context });

    expect(getSettledConversationBootstrapSnapshotMock).toHaveBeenCalledWith('conv-1');
    expect(hydrateConversationFromBootstrapSnapshotMock).toHaveBeenCalled();
    expect(fetchConversationMock).not.toHaveBeenCalled();
    expect(dsTickMock).not.toHaveBeenCalled();
    expect(publishActiveConversationMock).toHaveBeenCalledWith('conv-1', context);
    expect(startPollingMock).toHaveBeenCalledWith(context);
  });

  it('starts recovery polling when a pending bootstrap owns initial hydration', async () => {
    const conversationsDS = {
      peekFormData: () => ({ id: 'conv-pending' }),
      setFormData: vi.fn(),
    };
    const context = {
      Context(name) {
        if (name === 'conversations') return { handlers: { dataSource: conversationsDS } };
        if (name === 'messages') return { handlers: { dataSource: { setCollection: vi.fn(), setError: vi.fn() } } };
        if (name === 'meta') return { handlers: { dataSource: { peekFormData: () => ({ defaults: {} }) } } };
        return null;
      },
    };
    hasPendingConversationBootstrapMock.mockReturnValue(true);

    await onInit({ context });

    expect(syncConversationTransportMock).toHaveBeenCalledWith(context, 'conv-pending');
    expect(startPollingMock).toHaveBeenCalledWith(context);
  });

  it('publishes terminal conversation meta when init fetches a settled conversation', async () => {
    const convForm = { id: 'conv-1', title: 'Old title', stage: 'executing', status: 'running', running: true };
    const conversationsDS = {
      peekFormData: () => convForm,
      setFormData: vi.fn(({ values }) => Object.assign(convForm, values)),
    };
    const messagesDS = {
      setCollection: vi.fn(),
      setError: vi.fn(),
    };
    const context = {
      Context(name) {
        if (name === 'conversations') return { handlers: { dataSource: conversationsDS } };
        if (name === 'messages') return { handlers: { dataSource: messagesDS } };
        if (name === 'meta') return { handlers: { dataSource: { peekFormData: () => ({ defaults: {} }) } } };
        return null;
      },
    };

    fetchConversationMock.mockResolvedValue({
      id: 'conv-1',
      title: 'Creative rendering test request',
      stage: 'done',
      status: 'succeeded',
    });
    dsTickMock.mockResolvedValue({ conversationID: 'conv-1', hasRunning: false });

    await onInit({ context });

    expect(publishConversationMetaUpdatedMock).toHaveBeenCalledWith('conv-1', {
      title: 'Creative rendering test request',
      stage: 'done',
      status: 'succeeded',
      running: false,
    });
  });

  it('does not let an obsolete init overwrite a newer conversation selection', async () => {
    const convForm = { id: 'conv-old', title: 'Old conversation' };
    const conversationsDS = {
      peekFormData: () => convForm,
      setFormData: vi.fn(({ values }) => Object.assign(convForm, values)),
    };
    const messagesDS = {
      setCollection: vi.fn(),
      setError: vi.fn(),
    };
    const context = {
      identity: { windowId: 'chat/new' },
      Context(name) {
        if (name === 'conversations') return { handlers: { dataSource: conversationsDS } };
        if (name === 'messages') return { handlers: { dataSource: messagesDS } };
        if (name === 'meta') return { handlers: { dataSource: { peekFormData: () => ({ defaults: {} }) } } };
        return null;
      },
    };
    let resolveConversation;
    fetchConversationMock.mockReturnValue(new Promise((resolve) => {
      resolveConversation = resolve;
    }));
    dsTickMock.mockResolvedValue({ conversationID: 'conv-old', hasRunning: false });

    const initialization = onInit({ context });
    await vi.waitFor(() => expect(fetchConversationMock).toHaveBeenCalledWith('conv-old'));
    convForm.id = 'conv-new';
    resolveConversation({ id: 'conv-old', title: 'Stale title', status: 'succeeded' });
    await initialization;

    expect(convForm).toMatchObject({ id: 'conv-new', title: 'Old conversation' });
    expect(conversationsDS.setFormData).not.toHaveBeenCalled();
    expect(publishConversationMetaUpdatedMock).not.toHaveBeenCalled();
    expect(publishActiveConversationMock).not.toHaveBeenCalledWith('conv-old', context);
    expect(startPollingMock).toHaveBeenCalledWith(context);
  });

  it('fences an older Context when a replacement Context initializes the same window', async () => {
    const convForm = { id: 'conv-old', title: 'Old conversation' };
    const conversationsDS = {
      peekFormData: () => convForm,
      setFormData: vi.fn(({ values }) => Object.assign(convForm, values)),
    };
    const makeContext = () => ({
      identity: { windowId: 'chat/new' },
      Context(name) {
        if (name === 'conversations') return { handlers: { dataSource: conversationsDS } };
        if (name === 'messages') return { handlers: { dataSource: { setCollection: vi.fn(), setError: vi.fn() } } };
        if (name === 'meta') return { handlers: { dataSource: { peekFormData: () => ({ defaults: {} }) } } };
        return null;
      },
    });
    const oldContext = makeContext();
    const newContext = makeContext();
    let resolveOldConversation;
    fetchConversationMock.mockImplementation((conversationID) => {
      if (conversationID === 'conv-old') {
        return new Promise((resolve) => { resolveOldConversation = resolve; });
      }
      return Promise.resolve({ id: 'conv-new', title: 'New conversation', status: 'succeeded' });
    });
    dsTickMock.mockImplementation((_context, options) => Promise.resolve({
      conversationID: options?.conversationID,
      hasRunning: false,
    }));

    const oldInitialization = onInit({ context: oldContext });
    await vi.waitFor(() => expect(fetchConversationMock).toHaveBeenCalledWith('conv-old'));
    convForm.id = 'conv-new';
    const newInitialization = onInit({ context: newContext });
    await newInitialization;
    resolveOldConversation({ id: 'conv-old', title: 'Stale title', status: 'succeeded' });
    await oldInitialization;

    expect(convForm).toMatchObject({ id: 'conv-new', title: 'New conversation' });
    expect(publishConversationMetaUpdatedMock).not.toHaveBeenCalledWith(
      'conv-old',
      expect.anything(),
    );
    expect(startPollingMock).toHaveBeenCalledWith(newContext);
    expect(startPollingMock).not.toHaveBeenCalledWith(oldContext);
  });

  it('rejects a pre-switch result after an A-to-B-to-A selection round trip', async () => {
    const resources = { conversationSelectionGeneration: 0 };
    ensureContextResourcesMock.mockReturnValue(resources);
    const convForm = { id: 'conv-a', title: 'Current A title' };
    const conversationsDS = {
      peekFormData: () => convForm,
      setFormData: vi.fn(({ values }) => Object.assign(convForm, values)),
    };
    const context = {
      identity: { windowId: 'chat/new' },
      Context(name) {
        if (name === 'conversations') return { handlers: { dataSource: conversationsDS } };
        if (name === 'messages') return { handlers: { dataSource: { setCollection: vi.fn(), setError: vi.fn() } } };
        if (name === 'meta') return { handlers: { dataSource: { peekFormData: () => ({ defaults: {} }) } } };
        return null;
      },
    };
    let resolveConversation;
    fetchConversationMock.mockReturnValue(new Promise((resolve) => {
      resolveConversation = resolve;
    }));
    dsTickMock.mockResolvedValue({ conversationID: 'conv-a', hasRunning: false });

    const initialization = onInit({ context });
    await vi.waitFor(() => expect(fetchConversationMock).toHaveBeenCalledWith('conv-a'));
    convForm.id = 'conv-b';
    resources.conversationSelectionGeneration += 1;
    convForm.id = 'conv-a';
    resources.conversationSelectionGeneration += 1;
    resolveConversation({ id: 'conv-a', title: 'Pre-switch stale title', status: 'succeeded' });
    await initialization;

    expect(convForm).toMatchObject({ id: 'conv-a', title: 'Current A title' });
    expect(conversationsDS.setFormData).not.toHaveBeenCalled();
  });
});
