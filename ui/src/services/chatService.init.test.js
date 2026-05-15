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
  hasPendingConversationBootstrapMock,
  startPollingMock,
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
  hasPendingConversationBootstrapMock: vi.fn(() => false),
  startPollingMock: vi.fn(),
}));

vi.mock('./stageBus', () => ({
  setStage: setStageMock,
}));

vi.mock('./chatRuntime', () => ({
  applyIterationVisibility: vi.fn(),
  bindConversationWindowEvents: bindConversationWindowEventsMock,
  bootstrapConversationSelection: bootstrapConversationSelectionMock,
  cacheSettledConversationBootstrapSnapshot: cacheSettledConversationBootstrapSnapshotMock,
  createNewConversation: vi.fn(),
  dsTick: dsTickMock,
  disconnectStream: disconnectStreamMock,
  ensureContextResources: ensureContextResourcesMock,
  ensureConversation: vi.fn(),
  fetchConversation: fetchConversationMock,
  fetchPendingElicitations: vi.fn(),
  getSettledConversationBootstrapSnapshot: getSettledConversationBootstrapSnapshotMock,
  getVisibleIterations: vi.fn(),
  hasPendingConversationBootstrap: hasPendingConversationBootstrapMock,
  hydrateMeta: hydrateMetaMock,
  hydrateConversationFromBootstrapSnapshot: hydrateConversationFromBootstrapSnapshotMock,
  isConversationLiveish: vi.fn(() => false),
  logExecutorDebug: logExecutorDebugMock,
  logStreamDebug: vi.fn(),
  mapTranscriptToRows: vi.fn(),
  normalizeMetaResponse: vi.fn(),
  publishActiveConversation: publishActiveConversationMock,
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
  });
});
