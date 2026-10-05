import { afterEach, describe, expect, it, vi } from 'vitest';
import { AgUiCommands } from 'agently-core-ui-sdk';
import { AgUiMcpProxy } from './aguiProxy';
import { buildStandardMcpDocument } from './standardDocument';
afterEach(() => vi.restoreAllMocks());

describe('isolated MCP Apps AG-UI proxy', () => {
  it('settles an approved RPC only from its exact genuine proxy successor', async () => {
    let phase = 'interrupted';
    let complete;
    const full = { content: [], _meta: { private: 'full host result' } };
    const session = { continue: async () => ({}), detach: vi.fn(),
      attach: vi.fn(() => new Promise(resolve => { complete = () => { phase = 'completed'; resolve({ result: full }); }; })),
      getSnapshot: () => ({ phase, threadId: 'proxy-thread', runId: 'original', interrupts: [{ id: 'approval' }] }) };
    const interrupted = { ...session, attach: vi.fn(() => { throw new Error('Pending interrupts require resume'); }) };
    const createAgUiSession = vi.fn().mockReturnValueOnce(interrupted).mockReturnValueOnce(session);
    const pending = vi.fn();
    const host = new AgUiMcpProxy({ createAgUiSession }, { serverId: 'issued', serverHash: 'hash' }, pending);
    const resolved = vi.fn();
    const result = host.call('tools/call', { name: 'tool' }).then(resolved);
    await vi.waitFor(() => expect(pending).toHaveBeenCalled());
    const outcome = { approvalId: 'approval', result: 'not the host response', protocol: { version: '1', kind: 'mcp-app', threadId: 'proxy-thread', originalRunId: 'original', continuationRunId: 'successor' } };
    expect(host.observeOutcome({ ...outcome, approvalId: 'foreign' })).toBe(false);
    expect(host.observeOutcome({ ...outcome, protocol: { ...outcome.protocol, threadId: 'other' } })).toBe(false);
    expect(host.observeOutcome(outcome)).toBe(true);
    expect(host.observeOutcome(outcome)).toBe(false);
    expect(resolved).not.toHaveBeenCalled();
    complete(); await result;
    expect(session.attach).toHaveBeenCalledTimes(1);
    expect(interrupted.attach).not.toHaveBeenCalled();
    expect(interrupted.detach).toHaveBeenCalledOnce();
    expect(createAgUiSession).toHaveBeenLastCalledWith(expect.objectContaining({ threadId: 'proxy-thread', durableReplay: true }));
    expect(resolved).toHaveBeenCalledWith(full);
    host.dispose();
  });
  it('discovers a successor on a committed application hint without resubmitting the effect', async () => {
    let handlers;
    let phase = 'interrupted';
    const session = { continue: vi.fn(async () => ({})), detach: vi.fn(),
      attach: vi.fn(async () => { phase = 'completed'; return { result: { content: [], isError: false } }; }),
      getSnapshot: () => ({ phase, threadId: 'proxy-thread', runId: 'original', interrupts: [{ id: 'approval' }] }) };
    const execute = vi.spyOn(AgUiCommands.prototype, 'execute').mockResolvedValueOnce({}).mockResolvedValue({ resumedByRunId: 'successor' });
    const close = vi.fn();
    const host = new AgUiMcpProxy({ createAgUiSession: () => session, agUiTransport: () => ({ url: '/v1/ag-ui/run' }),
      observeNativeEvents: (_id, callbacks) => { handlers = callbacks; return { close }; } },
      { serverId: 'issued', serverHash: 'hash', _agentlyApp: { threadId: 'native-thread' } });
    const result = host.call('tools/call', { name: 'tool' });
    await vi.waitFor(() => expect(execute).toHaveBeenCalledTimes(1));
    await host.refreshing;
    handlers.onEvent({ type: 'conversation_meta_updated', patch: { aguiUpdated: true } });
    await expect(result).resolves.toEqual({ content: [], isError: false });
    expect(session.continue).toHaveBeenCalledTimes(1);
    expect(execute.mock.calls[0][2]).toMatchObject({ threadId: 'proxy-thread' });
    expect(close).toHaveBeenCalledOnce();
    host.dispose();
  });
  it('captures the issued server alias and returns the full real host result', async () => {
    const result = { content: [], _meta: { private: 'host' } };
    const session = { continue: vi.fn(async () => ({ result })), getSnapshot: () => ({ phase: 'completed' }), detach: vi.fn() };
    const client = { createAgUiSession: vi.fn(() => session) };
    const host = new AgUiMcpProxy(client, { serverId: 'issued', serverHash: 'hash', nativeServerId: 'never-use' });
    expect(await host.call('tools/call', { name: 'tool', serverId: 'guest' })).toEqual(result);
    const input = session.continue.mock.calls[0][0];
    expect(input.forwardedProps.__proxiedMCPRequest).toMatchObject({ serverId: 'issued', serverHash: 'hash', method: 'tools/call' });
    expect(JSON.stringify(input)).not.toContain('never-use');
    expect(client.createAgUiSession.mock.calls[0][0]).toMatchObject({ threadId: input.runId, connectionId: 'agently' });
    await expect(host.call('arbitrary-method')).rejects.toThrow('Unsupported');
  });
  it('keeps interrupted calls pending until a genuine resumed result and detaches on disposal', async () => {
    let phase = 'interrupted';
    const session = { continue: vi.fn(async () => ({})), resume: vi.fn(async () => { phase = 'completed'; return { result: { content: [], structuredContent: { real: true } } }; }),
      getSnapshot: () => ({ phase, interrupts: [{ id: 'approval' }] }), detach: vi.fn() };
    const pending = vi.fn();
    const host = new AgUiMcpProxy({ createAgUiSession: () => session }, { serverId: 'issued', serverHash: 'hash' }, pending);
    const resolved = vi.fn();
    const result = host.call('tools/call', { name: 'tool' }).then(resolved);
    await vi.waitFor(() => expect(pending).toHaveBeenCalled());
    expect(resolved).not.toHaveBeenCalled();
    await host.resume(pending.mock.calls[0][0].id, { approval: { status: 'resolved', payload: { action: 'approve' } } });
    await result;
    expect(resolved).toHaveBeenCalledWith({ content: [], structuredContent: { real: true } });
    phase = 'interrupted';
    const abandoned = host.call('tools/call', { name: 'another' });
    const rejection = expect(abandoned).rejects.toMatchObject({ name: 'AbortError' });
    host.dispose();
    await rejection;
    expect(session.detach).toHaveBeenCalledOnce();
  });
});
describe('standard app document policy', () => {
  it('isolates resource policy from legacy sandbox overrides and rejects CSP injection', () => {
    const html = buildStandardMcpDocument({ mimeType: 'text/html;profile=mcp-app', text: '<h1>App</h1>', _meta: { ui: {
      sandbox: 'allow-same-origin', csp: { connectDomains: ['https://api.example', "https://bad.example/;default-src *"], resourceDomains: [] },
    } } });
    expect(html).toContain("default-src 'none'");
    expect(html).toContain('connect-src https://api.example');
    expect(html).not.toContain('bad.example');
    expect(html).not.toContain('allow-same-origin');
    expect(html).toContain("base-uri 'none'");
    expect(() => buildStandardMcpDocument({ mimeType: 'text/plain', text: 'data' })).toThrow('HTML');
  });
});
