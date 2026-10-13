import { afterEach, describe, expect, it } from 'vitest';

import { publishUIBridgeSnapshotNow, startUIBridgeHTTP } from 'forge/core';
import { connectorConfig } from './connector';

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
    expect(storedClientId).toBe(window.__forgeUIBridgeClientId);
    expect(window.__forgeUIBridgeClientId.length).toBeGreaterThan(0);
  });
});


describe('workspace bridge recovery regressions', () => {
  function setupBrowser() {
    const target = new EventTarget();
    target.sessionStorage = { getItem: () => '', setItem: () => {} };
    globalThis.window = target;
    globalThis.document = {
      visibilityState: 'visible', hasFocus: () => true,
      addEventListener: target.addEventListener.bind(target),
      removeEventListener: target.removeEventListener.bind(target),
    };
    return target;
  }

  function rpcResponse(body, result = {}) {
    return new Response(JSON.stringify({ jsonrpc: '2.0', id: body.id, result }), {
      status: 200, headers: { 'Mcp-Session-Id': 'regression-session' },
    });
  }

  it('reattaches the workspace after Agently recovers its login session', async () => {
    const target = setupBrowser();
    const calls = [];
    let authenticated = false;
    globalThis.fetch = async (_url, options = {}) => {
      const body = JSON.parse(options.body);
      calls.push(body.method);
      if (body.method === 'ui.hello' && !authenticated) return new Response('', { status: 401 });
      if (body.method === 'ui.poll') return new Response('', { status: 202 });
      return rpcResponse(body);
    };
    const stop = startUIBridgeHTTP({
      ...connectorConfig.uiBridge,
      snapshotIntervalMs: 10000, reconnectDelayMs: 10000,
      snapshotBuilder: () => ({ conversationId: 'conv-regression', windows: [] }),
    });
    try {
      await sleep(25);
      authenticated = true;
      target.dispatchEvent(new Event('agently:authorized'));
      await sleep(40);
      expect(calls.slice(0, 4)).toEqual(['ui.hello', 'ui.hello', 'ui.snapshot.get', 'ui.snapshot']);
    } finally { stop(); }
  });

  it('waits for an in-flight handshake before publishing the pre-submit snapshot', async () => {
    setupBrowser();
    let releaseHello;
    const calls = [];
    globalThis.fetch = async (_url, options = {}) => {
      const body = JSON.parse(options.body);
      calls.push(body.method);
      if (body.method === 'ui.hello') await new Promise(resolve => { releaseHello = resolve; });
      if (body.method === 'ui.poll') return new Response('', { status: 202 });
      return rpcResponse(body);
    };
    const stop = startUIBridgeHTTP({
      url: '/v1/ui/rpc', snapshotIntervalMs: 10000, reconnectDelayMs: 10000,
      snapshotBuilder: () => ({ conversationId: 'conv-regression', windows: [] }),
    });
    try {
      const publishing = publishUIBridgeSnapshotNow();
      releaseHello();
      expect(await publishing).toBe(true);
      expect(calls).toContain('ui.snapshot');
    } finally { stop(); }
  });
});
