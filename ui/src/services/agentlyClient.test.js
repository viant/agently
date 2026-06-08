import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('agently-core-ui-sdk', () => ({
  AgentlyClient: class AgentlyClient {
    constructor(options = {}) {
      this.options = options;
    }

    getWorkspaceMetadata() {
      if (!globalThis.__workspaceMetadataResolvers) {
        return Promise.resolve({ appName: 'Agently' });
      }
      globalThis.__workspaceMetadataCalls = Number(globalThis.__workspaceMetadataCalls || 0) + 1;
      return new Promise((resolve) => {
        globalThis.__workspaceMetadataResolvers.push(resolve);
      });
    }

    getOAuthConfig() {
      return Promise.resolve({});
    }

    loginWithRedirect() {}

    loginWithPopup() {
      return Promise.resolve(false);
    }
  }
}));

vi.mock('./httpClient', () => ({
  showToast: vi.fn(),
  redirectToLogin: vi.fn()
}));

describe('recoverSessionSilently', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.restoreAllMocks();
  });

  it('recovers an existing cookie-backed session and dispatches authorized', async () => {
    const listeners = [];
    globalThis.window = {
      dispatchEvent: (event) => listeners.push(event?.type || ''),
      CustomEvent: class extends Event {
        constructor(name, init = {}) {
          super(name);
          this.detail = init.detail;
        }
      }
    };
    globalThis.CustomEvent = globalThis.window.CustomEvent;
    globalThis.fetch = vi.fn().mockResolvedValue({
      status: 200
    });

    const { recoverSessionSilently } = await import('./agentlyClient.js');
    const ok = await recoverSessionSilently();

    expect(ok).toBe(true);
    expect(globalThis.fetch).toHaveBeenCalledTimes(1);
    expect(listeners).toContain('agently:authorized');
  });

  it('uses a single in-flight recovery probe for concurrent callers', async () => {
    globalThis.window = {
      dispatchEvent: vi.fn(),
      CustomEvent: class extends Event {
        constructor(name, init = {}) {
          super(name);
          this.detail = init.detail;
        }
      }
    };
    globalThis.CustomEvent = globalThis.window.CustomEvent;
    let resolveFetch;
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise((resolve) => {
      resolveFetch = resolve;
    }));

    const { recoverSessionSilently } = await import('./agentlyClient.js');
    const first = recoverSessionSilently();
    const second = recoverSessionSilently();
    resolveFetch({ status: 200 });

    expect(await first).toBe(true);
    expect(await second).toBe(true);
    expect(globalThis.fetch).toHaveBeenCalledTimes(1);
  });

  it('returns false after retrying when auth probe stays unauthorized', async () => {
    globalThis.window = {
      dispatchEvent: vi.fn(),
      CustomEvent: class extends Event {
        constructor(name, init = {}) {
          super(name);
          this.detail = init.detail;
        }
      },
      setTimeout: globalThis.setTimeout.bind(globalThis)
    };
    globalThis.CustomEvent = globalThis.window.CustomEvent;
    globalThis.fetch = vi.fn().mockResolvedValue({
      status: 401
    });

    const { recoverSessionSilently } = await import('./agentlyClient.js');
    const ok = await recoverSessionSilently();

    expect(ok).toBe(false);
    expect(globalThis.fetch).toHaveBeenCalledTimes(2);
  });
});

describe('getAuthMeSilently', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.restoreAllMocks();
    globalThis.window = {
      dispatchEvent: vi.fn(),
      CustomEvent: class extends Event {
        constructor(name, init = {}) {
          super(name);
          this.detail = init.detail;
        }
      }
    };
    globalThis.CustomEvent = globalThis.window.CustomEvent;
  });

  it('reuses a single in-flight auth/me probe for concurrent callers', async () => {
    let resolveFetch;
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise((resolve) => {
      resolveFetch = resolve;
    }));

    const { getAuthMeSilently } = await import('./agentlyClient.js');
    const first = getAuthMeSilently();
    const second = getAuthMeSilently();
    resolveFetch({
      status: 200,
      json: async () => ({ username: 'awitas' }),
    });

    expect(await first).toEqual({ username: 'awitas' });
    expect(await second).toEqual({ username: 'awitas' });
    expect(globalThis.fetch).toHaveBeenCalledTimes(1);
  });

  it('returns cached auth/me data after the first successful probe', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      status: 200,
      json: async () => ({ username: 'awitas' }),
    });

    const { getAuthMeSilently } = await import('./agentlyClient.js');
    expect(await getAuthMeSilently()).toEqual({ username: 'awitas' });
    expect(await getAuthMeSilently()).toEqual({ username: 'awitas' });
    expect(globalThis.fetch).toHaveBeenCalledTimes(1);
  });
});

describe('getAuthProvidersSilently', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.restoreAllMocks();
    globalThis.window = {
      dispatchEvent: vi.fn(),
      CustomEvent: class extends Event {
        constructor(name, init = {}) {
          super(name);
          this.detail = init.detail;
        }
      }
    };
    globalThis.CustomEvent = globalThis.window.CustomEvent;
  });

  it('unwraps auth providers from either envelope or flat-array responses', async () => {
    globalThis.fetch = vi.fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ providers: [{ type: 'bff', name: 'oauth' }] }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ([{ type: 'local', name: 'local' }]),
      });

    const { getAuthProvidersSilently } = await import('./agentlyClient.js');

    expect(await getAuthProvidersSilently()).toEqual([{ type: 'bff', name: 'oauth' }]);
    expect(await getAuthProvidersSilently()).toEqual([{ type: 'local', name: 'local' }]);
  });

  it('returns an empty provider list when the probe fails', async () => {
    globalThis.fetch = vi.fn().mockRejectedValue(new Error('timeout'));

    const { getAuthProvidersSilently } = await import('./agentlyClient.js');

    expect(await getAuthProvidersSilently()).toEqual([]);
  });
});

describe('beginLogin', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.restoreAllMocks();
  });

  it('uses same-tab redirect by default when popup login is not enabled', async () => {
    globalThis.window = {};

    const { beginLogin, client } = await import('./agentlyClient.js');
    client.getOAuthConfig = vi.fn().mockResolvedValue({});
    client.loginWithRedirect = vi.fn();
    client.loginWithPopup = vi.fn().mockResolvedValue(false);

    const ok = await beginLogin();

    expect(ok).toBe(true);
    expect(client.getOAuthConfig).toHaveBeenCalledTimes(1);
    expect(client.loginWithRedirect).toHaveBeenCalledTimes(1);
    expect(client.loginWithPopup).not.toHaveBeenCalled();
  });

  it('uses same-tab redirect when popup login is explicitly disabled', async () => {
    globalThis.window = {};

    const { beginLogin, client } = await import('./agentlyClient.js');
    client.getOAuthConfig = vi.fn().mockResolvedValue({ usePopupLogin: false });
    client.loginWithRedirect = vi.fn();
    client.loginWithPopup = vi.fn().mockResolvedValue(true);

    const ok = await beginLogin();

    expect(ok).toBe(true);
    expect(client.loginWithRedirect).toHaveBeenCalledTimes(1);
    expect(client.loginWithPopup).not.toHaveBeenCalled();
  });

  it('uses popup login only when workspace oauth config explicitly enables it', async () => {
    globalThis.window = {};

    const { beginLogin, client } = await import('./agentlyClient.js');
    client.getOAuthConfig = vi.fn().mockResolvedValue({ usePopupLogin: true });
    client.loginWithRedirect = vi.fn();
    client.loginWithPopup = vi.fn().mockResolvedValue(true);

    const ok = await beginLogin();

    expect(ok).toBe(true);
    expect(client.loginWithPopup).toHaveBeenCalledTimes(1);
    expect(client.loginWithRedirect).not.toHaveBeenCalled();
  });
});

describe('getWorkspaceMetadata cache', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.restoreAllMocks();
    globalThis.window = {};
    globalThis.__workspaceMetadataCalls = 0;
    globalThis.__workspaceMetadataResolvers = [];
  });

  it('reuses a single in-flight workspace metadata request for concurrent callers', async () => {
    const { client } = await import('./agentlyClient.js');

    const first = client.getWorkspaceMetadata();
    const second = client.getWorkspaceMetadata();
    const [resolveMetadata] = globalThis.__workspaceMetadataResolvers;
    resolveMetadata({ appName: 'Agently' });

    expect(await first).toEqual({ appName: 'Agently' });
    expect(await second).toEqual({ appName: 'Agently' });
    expect(globalThis.__workspaceMetadataCalls).toBe(1);
  });
});
