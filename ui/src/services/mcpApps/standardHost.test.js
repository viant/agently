import { describe, it, expect, vi } from 'vitest';
import { App } from '@modelcontextprotocol/ext-apps';
import { createStandardAppHost, AppHostTransport } from './standardHost.js';

function channel() {
  const listeners = new Set();
  const eventWindow = { addEventListener: (_, fn) => listeners.add(fn), removeEventListener: (_, fn) => listeners.delete(fn) };
  const origin = 'https://sandbox.example';
  const view = { async start() {}, async close() { this.onclose?.(); },
    async send(message) { for (const fn of listeners) fn({ source: target, origin, data: message }); } };
  const target = { postMessage: vi.fn(message => queueMicrotask(() => view.onmessage?.(message))) };
  return { view, target, origin, eventWindow, dispatch: (data, options = {}) => { for (const fn of listeners) fn({ source: target, origin, ...options, data }); } };
}
const resource = { html: '<!doctype html><p>A generic app</p>', csp: {} };

describe('official MCP Apps host', () => {
  it('interoperates with the real pinned App SDK, delivers inputs/results, calls tools and acknowledges teardown', async () => {
    const wire = channel();
    const proxy = { call: vi.fn(async () => ({ content: [{ type: 'text', text: 'refreshed' }] })), dispose: vi.fn() };
    let resolveReady; const ready = new Promise(resolve => { resolveReady = resolve; });
    const host = createStandardAppHost({ ...wire, proxy, resource, onReady: resolveReady });
    await host.connect();
    wire.dispatch({ jsonrpc: '2.0', method: 'ui/notifications/sandbox-proxy-ready', params: {} });
    const app = new App({ name: 'generic-fixture', version: '1' }, {}, { autoResize: false });
    const input = vi.fn(), partial = vi.fn(), result = vi.fn(), teardown = vi.fn(async () => ({}));
    app.ontoolinput = input; app.ontoolinputpartial = partial; app.ontoolresult = result; app.onteardown = teardown;
    await app.connect(wire.view);
    await ready;
    await host.sendPartialInput({ query: 'par' });
    await host.sendInput({ query: 'complete' });
    await host.sendResult({ content: [{ type: 'text', text: 'completed' }], _meta: { fixture: true } });
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(input).toHaveBeenCalledWith({ arguments: { query: 'complete' } });
    expect(partial).toHaveBeenCalled(); expect(result.mock.calls[0][0]._meta).toEqual({ fixture: true });
    const answer = await app.callServerTool({ name: 'refresh', arguments: {} });
    expect(answer.content[0].text).toBe('refreshed'); expect(proxy.call).toHaveBeenCalledWith('tools/call', expect.objectContaining({ name: 'refresh', arguments: {} }));
    await host.close();
    expect(teardown).toHaveBeenCalledOnce(); expect(proxy.dispose).toHaveBeenCalledOnce();
    await app.close();
  });
  it('rejects wrong windows/origins, pre-handshake tools, duplicates, unknown and oversized frames', async () => {
    const wire = channel(), transport = new AppHostTransport(wire.target, wire.origin, wire.eventWindow);
    transport.onmessage = vi.fn(); await transport.start();
    const initialize = { jsonrpc: '2.0', id: 1, method: 'ui/initialize', params: {} };
    wire.dispatch(initialize, { source: {} }); wire.dispatch(initialize, { origin: 'https://evil.example' });
    wire.dispatch({ jsonrpc: '2.0', id: 2, method: 'tools/call', params: { name: 'refresh' } });
    expect(transport.onmessage).not.toHaveBeenCalled();
    wire.dispatch({ jsonrpc: '2.0', method: 'ui/notifications/sandbox-proxy-ready', params: {} });
    wire.dispatch(initialize); wire.dispatch(initialize);
    await transport.send({ jsonrpc: '2.0', id: 1, result: { protocolVersion: '2026-01-26' } });
    wire.dispatch({ jsonrpc: '2.0', method: 'ui/notifications/initialized', params: {} });
    const call = { jsonrpc: '2.0', id: 3, method: 'tools/call', params: { name: 'refresh' } };
    wire.dispatch(call); wire.dispatch(call);
    wire.dispatch({ ...call, id: 4, method: 'ui/message' });
    wire.dispatch({ ...call, id: 5, params: { name: 'refresh', arguments: { text: 'x'.repeat(300000) } } });
    expect(transport.onmessage).toHaveBeenCalledTimes(4);
    transport.beginTeardown(); wire.dispatch({ ...call, id: 6 });
    expect(transport.onmessage).toHaveBeenCalledTimes(4);
    await transport.close(); wire.dispatch({ ...call, id: 7 });
    expect(transport.onmessage).toHaveBeenCalledTimes(4);
  });
});
