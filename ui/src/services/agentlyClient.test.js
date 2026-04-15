import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('agently-core-ui-sdk', () => ({
  AgentlyClient: class AgentlyClient {
    constructor(options = {}) {
      this.options = options;
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
