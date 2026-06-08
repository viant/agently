import { afterEach, describe, expect, it } from 'vitest';

import { startUIBridgeHTTP } from 'forge/core';

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const originalFetch = globalThis.fetch;
const originalWindow = globalThis.window;
const originalDocument = globalThis.document;

afterEach(() => {
  globalThis.fetch = originalFetch;
  globalThis.window = originalWindow;
  globalThis.document = originalDocument;
});

describe('bridge startup readiness', () => {
  it('skips waiting for forge:conversation-active when snapshot already knows the conversation', async () => {
    const windowTarget = new EventTarget();
    let storedClientId = '';
    windowTarget.sessionStorage = {
      getItem(key) {
        return key === 'forge.uiBridge.clientId' ? storedClientId : null;
      },
      setItem(key, value) {
        if (key === 'forge.uiBridge.clientId') storedClientId = String(value || '');
      },
    };
    globalThis.window = windowTarget;
    globalThis.document = {
      visibilityState: 'visible',
      hasFocus: () => true,
      addEventListener: windowTarget.addEventListener.bind(windowTarget),
      removeEventListener: windowTarget.removeEventListener.bind(windowTarget),
    };

    const calls = [];
    globalThis.fetch = async (_url, options = {}) => {
      const body = JSON.parse(String(options.body || '{}'));
      calls.push(body.method);
      const headers = new Headers({ 'Mcp-Session-Id': 'session-ready-immediate' });
      if (body.method === 'ui.hello') {
        return new Response(JSON.stringify({ jsonrpc: '2.0', id: body.id, result: { ok: true } }), { status: 200, headers });
      }
      if (body.method === 'ui.snapshot.get') {
        return new Response(JSON.stringify({
          jsonrpc: '2.0',
          id: body.id,
          result: { snapshot: { selected: { windowId: 'chat/new', tabId: 'chat/new' }, windows: [] } }
        }), { status: 200, headers });
      }
      if (body.method === 'ui.snapshot') {
        return new Response(JSON.stringify({ jsonrpc: '2.0', id: body.id, result: { ok: true } }), { status: 200, headers });
      }
      if (body.method === 'ui.poll') {
        return new Response('', { status: 202, headers });
      }
      return new Response(JSON.stringify({ jsonrpc: '2.0', id: body.id, result: {} }), { status: 200, headers });
    };

    const stop = startUIBridgeHTTP({
      url: 'http://example.test/v1/ui/rpc',
      snapshotIntervalMs: 10_000,
      reconnectDelayMs: 10_000,
      startupReadyEvent: 'forge:conversation-active',
      startupReadyTimeoutMs: 10_000,
      snapshotBuilder: () => ({
        selected: { windowId: 'chat/new', tabId: 'chat/new' },
        windows: [{ windowId: 'chat/new', windowKey: 'chat/new' }],
        conversationId: 'conv-ready',
      }),
    });

    await sleep(40);
    stop();

    expect(calls.slice(0, 4)).toEqual(['ui.hello', 'ui.snapshot.get', 'ui.snapshot', 'ui.poll']);
    expect(storedClientId.length).toBeGreaterThan(0);
  });
});
