import { describe, expect, it } from 'vitest';
import { installForgeGuestBridge, MCPUI_FORGE_GUEST_KEY, MCPUI_GUEST_BRIDGE_READY_EVENT, MCPUI_HOST_KEY, MCPUI_HOST_READY_EVENT, MCPUI_HOST_TEARDOWN_EVENT } from './forgeGuestBridge.js';
import { handleGuestEnvelope } from './bridge.js';
import {
  MCPUI_VERIFIER_UNSAFE_LINK_PROBE_URL,
  probeDirectOpenLink,
  probeDirectToolCall,
  probeUnsafeOpenLink,
} from './mcpuiVerifierRouteDiagnostics.js';

function createFakeWindow() {
  const listeners = new Map();
  const posted = [];
  const dispatched = [];
  const win = {
    parent: {
      postMessage(payload) {
        posted.push(payload);
      },
    },
    addEventListener(type, cb) {
      listeners.set(type, cb);
    },
    removeEventListener(type) {
      listeners.delete(type);
    },
    emit(type, payload) {
      const cb = listeners.get(type);
      if (cb) cb({ data: payload });
    },
    dispatchEvent(event) {
      dispatched.push(event);
      const cb = listeners.get(event?.type);
      if (cb) cb(event);
      return true;
    },
    posted,
    dispatched,
  };
  return win;
}

describe('mcpApps/forgeGuestBridge', () => {
  it('captures host-ready state and posts message/tool envelopes through parent', () => {
    const fake = createFakeWindow();
    const cleanup = installForgeGuestBridge(fake);
    fake.emit('message', {
      method: 'mcpui:host-ready',
      params: {
        windowId: 'wid-1',
        resourceUri: 'ui://mcpuiverify/view/interactive',
        allowedTools: ['system/os:getEnv'],
      },
    });

    expect(fake[MCPUI_HOST_KEY]).toMatchObject({
      windowId: 'wid-1',
      resourceUri: 'ui://mcpuiverify/view/interactive',
      allowedTools: ['system/os:getEnv'],
    });
    expect(fake.dispatched.some((event) => event?.type === MCPUI_HOST_READY_EVENT)).toBe(true);

    fake[MCPUI_FORGE_GUEST_KEY].message('hello');
    fake[MCPUI_FORGE_GUEST_KEY].toolCall('system/os:getEnv', { names: ['HOME'] }, 'Read HOME');

    expect(fake.posted[0]).toMatchObject({
      method: 'mcpui:message',
      params: {
        windowId: 'wid-1',
        resourceUri: 'ui://mcpuiverify/view/interactive',
        content: 'hello',
      },
    });
    expect(fake.posted[1]).toMatchObject({
      method: 'mcpui:tools-call',
      params: {
        windowId: 'wid-1',
        resourceUri: 'ui://mcpuiverify/view/interactive',
        name: 'system/os:getEnv',
        arguments: { names: ['HOME'] },
        assistantText: 'Read HOME',
      },
    });
    cleanup();
  });

  it('replays an already-seeded same-origin host state on install', () => {
    const fake = createFakeWindow();
    fake[MCPUI_HOST_KEY] = {
      windowId: 'wid-seeded',
      resourceUri: 'ui://mcpuiverify/view/seeded',
    };
    installForgeGuestBridge(fake);
    expect(fake.dispatched.some((event) => event?.type === MCPUI_HOST_READY_EVENT && event?.detail?.windowId === 'wid-seeded')).toBe(true);
  });

  it('posts a canonical open-link envelope through the parent window', () => {
    const fake = createFakeWindow();
    const cleanup = installForgeGuestBridge(fake);
    fake.emit('message', {
      method: 'mcpui:host-ready',
      params: {
        windowId: 'wid-3',
        resourceUri: 'ui://mcpuiverify/view/interactive',
      },
    });
    fake[MCPUI_FORGE_GUEST_KEY].openLink('https://example.com/verifier');
    expect(fake.posted[0]).toMatchObject({
      version: '1.0.0',
      method: 'mcpui:open-link',
      params: {
        windowId: 'wid-3',
        resourceUri: 'ui://mcpuiverify/view/interactive',
        url: 'https://example.com/verifier',
      },
    });
    cleanup();
  });

  it('records the last tool-result envelope delivered from the host', () => {
    const fake = createFakeWindow();
    const cleanup = installForgeGuestBridge(fake);
    fake.emit('message', {
      method: 'mcpui:host-ready',
      params: { windowId: 'wid-4', resourceUri: 'ui://mcpuiverify/view/interactive' },
    });
    fake.emit('message', {
      version: '1.0.0',
      method: 'mcpui:tool-result',
      params: {
        windowId: 'wid-4',
        resourceUri: 'ui://mcpuiverify/view/interactive',
        toolName: 'system/os:getEnv',
        structuredContent: { status: 'completed', result: 'HOME=/root' },
      },
    });
    expect(fake[MCPUI_HOST_KEY].lastToolResult).toMatchObject({
      windowId: 'wid-4',
      resourceUri: 'ui://mcpuiverify/view/interactive',
      toolName: 'system/os:getEnv',
      structuredContent: { status: 'completed', result: 'HOME=/root' },
    });
    cleanup();
  });

  it('dispatches a guest-bridge-ready event after install so route diagnostics can refresh deterministically', () => {
    const fake = createFakeWindow();
    installForgeGuestBridge(fake);
    expect(fake.dispatched.some((event) => event?.type === MCPUI_GUEST_BRIDGE_READY_EVENT && event?.detail?.installed === true)).toBe(true);
  });

  it('clears the active host binding on teardown and rejects stale guest requests afterwards', () => {
    const fake = createFakeWindow();
    const cleanup = installForgeGuestBridge(fake);
    fake.emit('message', {
      method: 'mcpui:host-ready',
      params: {
        windowId: 'wid-teardown',
        resourceUri: 'ui://mcpuiverify/view/interactive',
      },
    });
    fake.emit('message', {
      version: '1.0.0',
      method: 'mcpui:teardown',
      params: {
        windowId: 'wid-teardown',
        resourceUri: 'ui://mcpuiverify/view/interactive',
      },
    });
    expect(fake[MCPUI_HOST_KEY]).toEqual({});
    expect(fake.dispatched.some((event) => event?.type === MCPUI_HOST_TEARDOWN_EVENT)).toBe(true);
    expect(() => fake[MCPUI_FORGE_GUEST_KEY].message('after teardown')).toThrow('host not ready');
    cleanup();
  });

  it('ignores stale tool-result payloads after teardown invalidates the prior binding', () => {
    const fake = createFakeWindow();
    const cleanup = installForgeGuestBridge(fake);
    fake.emit('message', {
      method: 'mcpui:host-ready',
      params: {
        windowId: 'wid-stale',
        resourceUri: 'ui://mcpuiverify/view/interactive',
      },
    });
    fake.emit('message', {
      version: '1.0.0',
      method: 'mcpui:teardown',
      params: {
        windowId: 'wid-stale',
        resourceUri: 'ui://mcpuiverify/view/interactive',
      },
    });
    fake.emit('message', {
      version: '1.0.0',
      method: 'mcpui:tool-result',
      params: {
        windowId: 'wid-stale',
        resourceUri: 'ui://mcpuiverify/view/interactive',
        toolName: 'system/os:getEnv',
        structuredContent: { status: 'completed', result: 'HOME=/tmp' },
      },
    });
    expect(fake[MCPUI_HOST_KEY].lastToolResult).toBeUndefined();
    cleanup();
  });

  it('preserves the host allowedToolBundles list in captured host state', () => {
    const fake = createFakeWindow();
    const cleanup = installForgeGuestBridge(fake);
    fake.emit('message', {
      method: 'mcpui:host-ready',
      params: {
        windowId: 'wid-5',
        resourceUri: 'ui://mcpuiverify/view/interactive',
        allowedTools: ['system/os:getEnv'],
        allowedToolBundles: ['mcpuiverify_queue'],
      },
    });
    expect(fake[MCPUI_HOST_KEY].allowedToolBundles).toEqual(['mcpuiverify_queue']);
    cleanup();
  });

  it('rejects unsafe openLink envelopes at the host bridge and never surfaces a host link affordance', async () => {
    const fake = createFakeWindow();
    const cleanup = installForgeGuestBridge(fake);
    fake.emit('message', {
      method: 'mcpui:host-ready',
      params: {
        windowId: 'wid-unsafe',
        resourceUri: 'ui://mcpuiverify/view/interactive',
      },
    });

    const probe = probeUnsafeOpenLink(fake, { timestamp: '2026-05-27T03:00:02Z' });
    expect(probe.ok).toBe(true);
    expect(probe.hostPolicy.ok).toBe(false);
    expect(probe.hostPolicy.hostLinkAffordance).toBe(false);

    const dispatched = fake.posted[0];
    expect(dispatched).toMatchObject({
      method: 'mcpui:open-link',
      params: {
        windowId: 'wid-unsafe',
        resourceUri: 'ui://mcpuiverify/view/interactive',
        url: MCPUI_VERIFIER_UNSAFE_LINK_PROBE_URL,
      },
    });

    await expect(handleGuestEnvelope(dispatched, {})).rejects.toThrow(
      'open-link url is not allowed: javascript:',
    );

    cleanup();
  });

  it('rejects data: openLink envelopes at the host bridge as well', async () => {
    const fake = createFakeWindow();
    const cleanup = installForgeGuestBridge(fake);
    fake.emit('message', {
      method: 'mcpui:host-ready',
      params: { windowId: 'wid-data', resourceUri: 'ui://mcpuiverify/view/interactive' },
    });
    fake[MCPUI_FORGE_GUEST_KEY].openLink('data:text/html,boom');
    const dispatched = fake.posted[0];
    expect(dispatched).toMatchObject({
      method: 'mcpui:open-link',
      params: { url: 'data:text/html,boom' },
    });
    await expect(handleGuestEnvelope(dispatched, {})).rejects.toThrow(
      'open-link url is not allowed: data:',
    );
    cleanup();
  });

  it('dispatches route-level toolCall and openLink probes through the guest bridge', () => {
    const fake = createFakeWindow();
    const cleanup = installForgeGuestBridge(fake);
    fake.emit('message', {
      method: 'mcpui:host-ready',
      params: {
        windowId: 'wid-6',
        resourceUri: 'ui://mcpuiverify/view/interactive',
      },
    });
    const tool = probeDirectToolCall(fake, { timestamp: '2026-05-27T03:00:00Z' });
    const link = probeDirectOpenLink(fake, { timestamp: '2026-05-27T03:00:01Z' });
    expect(tool.ok).toBe(true);
    expect(link.ok).toBe(true);
    expect(fake.posted[0]).toMatchObject({
      method: 'mcpui:tools-call',
      params: {
        windowId: 'wid-6',
        resourceUri: 'ui://mcpuiverify/view/interactive',
        name: 'system/os:getEnv',
      },
    });
    expect(fake.posted[1]).toMatchObject({
      method: 'mcpui:open-link',
      params: {
        windowId: 'wid-6',
        resourceUri: 'ui://mcpuiverify/view/interactive',
        url: 'https://example.com/',
      },
    });
    cleanup();
  });
});
