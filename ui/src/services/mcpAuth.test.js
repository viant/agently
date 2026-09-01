import { beforeEach, describe, expect, it, vi } from 'vitest';

const { getStatus, initiate, resolveElicitation } = vi.hoisted(() => ({
  getStatus: vi.fn(),
  initiate: vi.fn(),
  resolveElicitation: vi.fn(),
}));
vi.mock('./agentlyClient', () => ({
  client: { getMCPAuthStatus: getStatus, initiateMCPAuth: initiate, resolveElicitation },
}));

import {
  beginBrowserMCPAuth,
  clearPendingMCPAuth,
  currentMCPAuthReturnURL,
  pendingMCPAuth,
  resumePendingMCPAuth,
} from './mcpAuth';

describe('browser-delegated MCP OAuth', () => {
  beforeEach(() => {
    getStatus.mockReset();
    initiate.mockReset();
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

  it('keeps return targets same-origin and relative', () => {
    expect(currentMCPAuthReturnURL({ pathname: '/conversation/1', search: '?tab=x', hash: '#feed' }))
      .toBe('/conversation/1?tab=x#feed');
  });
});
