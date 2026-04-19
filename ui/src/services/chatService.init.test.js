import { describe, expect, it, vi, beforeEach } from 'vitest';

const {
  setStageMock,
  bindConversationWindowEventsMock,
  bootstrapConversationSelectionMock,
  renderMergedRowsForContextMock,
  hydrateMetaMock,
  ensureContextResourcesMock,
  fetchConversationMock,
  dsTickMock,
  syncConversationTransportMock,
  disconnectStreamMock,
  publishActiveConversationMock,
  startPollingMock,
} = vi.hoisted(() => ({
  setStageMock: vi.fn(),
  bindConversationWindowEventsMock: vi.fn(),
  bootstrapConversationSelectionMock: vi.fn(),
  renderMergedRowsForContextMock: vi.fn(),
  hydrateMetaMock: vi.fn(),
  ensureContextResourcesMock: vi.fn(() => ({})),
  fetchConversationMock: vi.fn(),
  dsTickMock: vi.fn(),
  syncConversationTransportMock: vi.fn(),
  disconnectStreamMock: vi.fn(),
  publishActiveConversationMock: vi.fn(),
  startPollingMock: vi.fn(),
}));

vi.mock('./stageBus', () => ({
  setStage: setStageMock,
}));

vi.mock('./chatRuntime', () => ({
  applyIterationVisibility: vi.fn(),
  bindConversationWindowEvents: bindConversationWindowEventsMock,
  bootstrapConversationSelection: bootstrapConversationSelectionMock,
  createNewConversation: vi.fn(),
  dsTick: dsTickMock,
  disconnectStream: disconnectStreamMock,
  ensureContextResources: ensureContextResourcesMock,
  ensureConversation: vi.fn(),
  fetchConversation: fetchConversationMock,
  fetchPendingElicitations: vi.fn(),
  getVisibleIterations: vi.fn(),
  hydrateMeta: hydrateMetaMock,
  isConversationLiveish: vi.fn(() => false),
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
});
