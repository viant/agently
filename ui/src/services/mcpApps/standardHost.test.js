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
    const earlyPartial = host.sendPartialInput({ query: 'par' });
    const earlyInput = host.sendInput({ query: 'complete' });
    const earlyResult = host.sendResult({ content: [{ type: 'text', text: 'completed' }], _meta: { fixture: true } });
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(wire.target.postMessage).not.toHaveBeenCalled();
    wire.dispatch({ jsonrpc: '2.0', method: 'ui/notifications/sandbox-proxy-ready', params: {} });
    const app = new App({ name: 'generic-fixture', version: '1' }, {}, { autoResize: false });
    const input = vi.fn(), partial = vi.fn(), result = vi.fn(), cancelled = vi.fn(), teardown = vi.fn(async () => ({}));
    app.ontoolinput = input; app.ontoolinputpartial = partial; app.ontoolresult = result; app.onteardown = teardown;
    app.ontoolcancelled = cancelled;
    await app.connect(wire.view);
    await ready;
    await Promise.all([earlyPartial, earlyInput, earlyResult]);
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(input).toHaveBeenCalledWith({ arguments: { query: 'complete' } });
    expect(partial).toHaveBeenCalled(); expect(result.mock.calls[0][0]._meta).toEqual({ fixture: true });
    const delivered = wire.target.postMessage.mock.calls.map(([message]) => message.method).filter(method => method?.startsWith('ui/notifications/tool-'));
    expect(delivered).toEqual(['ui/notifications/tool-input-partial', 'ui/notifications/tool-input', 'ui/notifications/tool-result']);
    await expect(host.sendPartialInput({ query: 'late' })).rejects.toThrow('Complete app tool input');
    await host.sendCancelled('User cancelled');
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(cancelled).toHaveBeenCalledWith({ reason: 'User cancelled' });
    expect(await app.requestDisplayMode({ mode: 'fullscreen' })).toEqual({ mode: 'inline' });
    const answer = await app.callServerTool({ name: 'refresh', arguments: {} });
    expect(answer.content[0].text).toBe('refreshed'); expect(proxy.call).toHaveBeenCalledWith('tools/call', expect.objectContaining({ name: 'refresh', arguments: {} }));
    await host.close();
    expect(teardown).toHaveBeenCalledOnce(); expect(proxy.dispose).toHaveBeenCalledOnce();
    await app.close();
  });
  it('rejects queued deliveries on teardown before initialization and bounds the queue', async () => {
    const wire = channel(), proxy = { call: vi.fn(), dispose: vi.fn() };
    const host = createStandardAppHost({ ...wire, proxy, resource });
    await host.connect();
    const pending = Array.from({ length: 32 }, (_, i) => host.sendInput({ sequence: i }));
    const failures = pending.map(promise => expect(promise).rejects.toThrow('closed'));
    await expect(host.sendResult({ content: [] })).rejects.toThrow('queue exhausted');
    await host.close();
    await Promise.all(failures);
    expect(wire.target.postMessage).not.toHaveBeenCalled();
    await expect(host.sendInput({})).rejects.toThrow('closed');
  });
  it('rejects wrong windows/origins, pre-handshake tools, duplicates, unknown and oversized frames', async () => {
    const wire = channel(), transport = new AppHostTransport(wire.target, wire.origin, wire.eventWindow);
    transport.onmessage = vi.fn(); await transport.start();
    await expect(transport.send({ jsonrpc: '2.0', method: 'ui/notifications/tool-input', params: { arguments: {} } })).rejects.toThrow('not initialized');
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
