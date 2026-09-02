import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getStatus, initiate, listConnections, resolveElicitation } = vi.hoisted(() => ({
  getStatus: vi.fn(),
  initiate: vi.fn(),
  listConnections: vi.fn(),
  resolveElicitation: vi.fn(),
}));
vi.mock('./agentlyClient', () => ({
  client: {
    getMCPAuthStatus: getStatus,
    initiateMCPAuth: initiate,
    listMCPAuthConnections: listConnections,
    resolveElicitation,
  },
}));

import {
  beginEagerMCPAuth,
  beginBrowserMCPAuth,
  clearPendingMCPAuth,
  currentMCPAuthReturnURL,
  pendingMCPAuth,
  rememberPendingMCPAuth,
  resumePendingMCPAuth,
} from './mcpAuth';

describe('browser-delegated MCP OAuth', () => {
  beforeEach(() => {
    getStatus.mockReset();
    initiate.mockReset();
    listConnections.mockReset();
    resolveElicitation.mockReset();
    const values = new Map();
    vi.stubGlobal('window', {
      location: { pathname: '/', search: '', hash: '', assign: vi.fn() },
      sessionStorage: {
        getItem: (key) => values.get(key) || null,
        setItem: (key, value) => values.set(key, value),
        removeItem: (key) => values.delete(key),
      },
    });
  });

  it('starts the first disconnected eager MCP connection', async () => {
    listConnections.mockResolvedValue({
      connections: [{ server: 'mediaplanner', connected: false }],
    });
    getStatus.mockResolvedValue({ server: 'mediaplanner', connected: false, csrfToken: 'csrf-1' });
    initiate.mockResolvedValue({ status: 'connect', authorizationURL: 'https://idp.test/authorize' });
    const navigate = vi.fn();

    await expect(beginEagerMCPAuth({ navigate })).resolves.toMatchObject({ status: 'connect' });

    expect(getStatus).toHaveBeenCalledWith('mediaplanner');
    expect(navigate).toHaveBeenCalledWith('https://idp.test/authorize');
  });

  it('does not restart eager linking while this browser has a pending flow', async () => {
    rememberPendingMCPAuth('mediaplanner', '/conversation/conv-1');

    await expect(beginEagerMCPAuth({ navigate: vi.fn() })).resolves.toMatchObject({ status: 'pending' });

    expect(listConnections).not.toHaveBeenCalled();
  });

  it('resolves the exact blocking elicitation after the OAuth callback', async () => {
    getStatus.mockResolvedValueOnce({ connected: false, csrfToken: 'csrf-1' });
    initiate.mockResolvedValueOnce({ status: 'connect', authorizationURL: 'https://idp.test/authorize' });
    await beginBrowserMCPAuth('catalog', {
      returnURL: '/conversation/conv-1',
      conversationId: 'conv-1',
      elicitationId: 'elic-1',
      navigate: vi.fn(),
    });
    getStatus.mockResolvedValueOnce({ connected: true });

    await expect(resumePendingMCPAuth()).resolves.toMatchObject({
      conversationId: 'conv-1', elicitationId: 'elic-1'
    });
    expect(resolveElicitation).toHaveBeenCalledWith('conv-1', 'elic-1', {
      action: 'accept', payload: { connected: true }
    });
    expect(pendingMCPAuth('catalog')).toBeNull();
  });

  it('restores the conversation before waiting for the resumed turn', async () => {
    rememberPendingMCPAuth('catalog', '/conversation/conv-1', {
      conversationId: 'conv-1',
      elicitationId: 'elic-1',
    });
    getStatus.mockResolvedValueOnce({ connected: true });
    let finishResolution;
    resolveElicitation.mockReturnValueOnce(new Promise((resolve) => { finishResolution = resolve; }));
    const onConnected = vi.fn();

    const resumed = resumePendingMCPAuth({ onConnected });
    await vi.waitFor(() => expect(onConnected).toHaveBeenCalledWith(expect.objectContaining({
      conversationId: 'conv-1',
    })));
    expect(pendingMCPAuth('catalog')).not.toBeNull();

    finishResolution();
    await expect(resumed).resolves.toMatchObject({ conversationId: 'conv-1' });
  });

  it('uses status CSRF and navigates the same browser context', async () => {
    getStatus.mockResolvedValue({ server: 'catalog', connected: false, csrfToken: 'csrf-1' });
    initiate.mockResolvedValue({ status: 'connect', authorizationURL: 'https://idp.test/authorize' });
    const navigate = vi.fn();
    const result = await beginBrowserMCPAuth('catalog', { returnURL: '/conversation/conv-1', navigate });
    expect(initiate).toHaveBeenCalledWith('catalog', 'csrf-1', { returnURL: '/conversation/conv-1', restart: false });
    expect(navigate).toHaveBeenCalledWith('https://idp.test/authorize');
    expect(result.status).toBe('connect');
    expect(pendingMCPAuth('catalog')).toMatchObject({ server: 'catalog', returnURL: '/conversation/conv-1' });
    clearPendingMCPAuth('catalog');
    expect(pendingMCPAuth('catalog')).toBeNull();
  });

  it('does not navigate when already connected', async () => {
    getStatus.mockResolvedValue({ server: 'catalog', connected: true });
    const navigate = vi.fn();
    await expect(beginBrowserMCPAuth('catalog', { navigate })).resolves.toMatchObject({ connected: true });
    expect(initiate).not.toHaveBeenCalled();
    expect(navigate).not.toHaveBeenCalled();
  });

  it('restarts a pending flow owned by this browser session', async () => {
    getStatus.mockResolvedValue({ server: 'catalog', connected: false, pending: true, csrfToken: 'csrf-1' });
    initiate.mockResolvedValue({ status: 'connect', authorizationURL: 'https://idp.test/authorize' });
    const navigate = vi.fn();
    await beginBrowserMCPAuth('catalog', { returnURL: '/conversation/conv-1', navigate });
    expect(initiate).toHaveBeenCalledWith('catalog', 'csrf-1', {
      returnURL: '/conversation/conv-1',
      restart: true,
    });
    expect(navigate).toHaveBeenCalledWith('https://idp.test/authorize');
  });

  it('forces an explicit reconnect to replace a stale flow from another session', async () => {
    getStatus.mockResolvedValue({ server: 'catalog', connected: false, pending: true, csrfToken: 'csrf-1' });
    initiate.mockResolvedValue({ status: 'connect', authorizationURL: 'https://idp.test/authorize' });

    await beginBrowserMCPAuth('catalog', { forceRestart: true, navigate: vi.fn() });

    expect(initiate).toHaveBeenCalledWith('catalog', 'csrf-1', {
      returnURL: '/',
      restart: true,
      forceRestart: true,
    });
  });

  it('keeps return targets same-origin and relative', () => {
    expect(currentMCPAuthReturnURL({ pathname: '/conversation/1', search: '?tab=x', hash: '#feed' }))
      .toBe('/conversation/1?tab=x#feed');
  });
});
