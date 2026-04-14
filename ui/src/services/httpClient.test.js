import { beforeEach, describe, expect, it, vi } from 'vitest';

describe('redirectToLogin', () => {
  beforeEach(() => {
    vi.resetModules();
    vi.restoreAllMocks();
  });

  it('signals unauthorized state without forcing an immediate oauth redirect', async () => {
    const dispatchEvent = vi.fn();
    const listeners = [];
    globalThis.window = {
      dispatchEvent,
      setTimeout: globalThis.setTimeout.bind(globalThis),
    };
    globalThis.document = {
      getElementById: () => null,
      createElement: () => ({
        style: {},
        appendChild: vi.fn(),
        remove: vi.fn(),
        textContent: '',
        id: ''
      }),
      body: {
        appendChild: (node) => listeners.push(node)
      }
    };
    globalThis.requestAnimationFrame = (cb) => { cb(); return 1; };
    globalThis.CustomEvent = class extends Event {
      constructor(name, init = {}) {
        super(name);
        this.detail = init.detail;
      }
    };

    const module = await import('./httpClient.js');
    module.redirectToLogin('/v1/agent/query');

    expect(dispatchEvent).toHaveBeenCalledTimes(1);
    expect(dispatchEvent.mock.calls[0][0]?.type).toBe('agently:unauthorized');
    expect(listeners.length).toBe(1);
  });
});
