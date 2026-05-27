import { describe, expect, it } from 'vitest';
import {
  MCPUI_FORGE_GUEST_KEY,
  MCPUI_HOST_KEY,
} from './forgeGuestBridge.js';
import {
  MCPUI_VERIFIER_PROBE_LABEL,
  MCPUI_VERIFIER_ROUTE_WINDOW_KEY,
  MCPUI_VERIFIER_UNSAFE_LINK_PROBE_URL,
  probeDirectBridge,
  probeHostOpenLinkPolicy,
  probeUnsafeOpenLink,
  readVerifierDiagnosticsSnapshot,
} from './mcpuiVerifierRouteDiagnostics.js';

describe('mcpApps/mcpuiVerifierRouteDiagnostics', () => {
  it('returns an explicit empty snapshot for an empty window', () => {
    expect(readVerifierDiagnosticsSnapshot({})).toEqual({
      guestBridgeInstalled: false,
      hostReady: false,
      windowId: '',
      resourceUri: '',
      allowedTools: [],
      allowedToolBundles: [],
    });
  });

  it('returns the explicit empty snapshot when target is null', () => {
    expect(readVerifierDiagnosticsSnapshot(null).guestBridgeInstalled).toBe(false);
    expect(readVerifierDiagnosticsSnapshot(null).hostReady).toBe(false);
  });

  it('reports the guest bridge as installed when window.__mcpuiForgeGuest exposes message()', () => {
    const target = {
      [MCPUI_FORGE_GUEST_KEY]: { message() {}, openLink() {}, toolCall() {} },
    };
    expect(readVerifierDiagnosticsSnapshot(target).guestBridgeInstalled).toBe(true);
  });

  it('reports host-ready when window.__mcpuiHost carries a windowId or resourceUri', () => {
    const onlyWindow = readVerifierDiagnosticsSnapshot({
      [MCPUI_HOST_KEY]: { windowId: 'wid-only', resourceUri: '' },
    });
    expect(onlyWindow.hostReady).toBe(true);
    expect(onlyWindow.windowId).toBe('wid-only');

    const onlyResource = readVerifierDiagnosticsSnapshot({
      [MCPUI_HOST_KEY]: { windowId: '', resourceUri: 'ui://mcpuiverify/view/mcpuiVerifierInteractive' },
    });
    expect(onlyResource.hostReady).toBe(true);
    expect(onlyResource.resourceUri).toBe('ui://mcpuiverify/view/mcpuiVerifierInteractive');

    const blank = readVerifierDiagnosticsSnapshot({
      [MCPUI_HOST_KEY]: { windowId: '', resourceUri: '' },
    });
    expect(blank.hostReady).toBe(false);
  });

  it('preserves allowedTools and allowedToolBundles arrays from host state', () => {
    const snapshot = readVerifierDiagnosticsSnapshot({
      [MCPUI_HOST_KEY]: {
        windowId: 'wid-1',
        resourceUri: 'ui://x',
        allowedTools: ['system/os:getEnv'],
        allowedToolBundles: ['mcpuiverify'],
      },
    });
    expect(snapshot.allowedTools).toEqual(['system/os:getEnv']);
    expect(snapshot.allowedToolBundles).toEqual(['mcpuiverify']);
  });

  it('probeDirectBridge reports unavailable bridge when window.__mcpuiForgeGuest is missing', () => {
    const result = probeDirectBridge({}, { timestamp: 't0' });
    expect(result).toEqual({ ok: false, reason: 'guest bridge unavailable', timestamp: 't0' });
  });

  it('probeDirectBridge invokes bridge.message and reports ok when invocation does not throw', () => {
    const calls = [];
    const target = {
      [MCPUI_FORGE_GUEST_KEY]: {
        message(content) { calls.push(content); },
      },
    };
    const result = probeDirectBridge(target, { timestamp: 't-ok' });
    expect(result.ok).toBe(true);
    expect(result.reason).toBe('parent.postMessage dispatched');
    expect(result.timestamp).toBe('t-ok');
    expect(calls).toEqual([`${MCPUI_VERIFIER_PROBE_LABEL} @ t-ok`]);
  });

  it('probeDirectBridge surfaces the underlying invocation failure reason', () => {
    const target = {
      [MCPUI_FORGE_GUEST_KEY]: {
        message() { throw new Error('boom'); },
      },
    };
    const result = probeDirectBridge(target, { timestamp: 't-fail' });
    expect(result.ok).toBe(false);
    expect(result.reason).toBe('invocation failed: boom');
    expect(result.timestamp).toBe('t-fail');
  });

  it('exposes the pinned verifier window key constant', () => {
    expect(MCPUI_VERIFIER_ROUTE_WINDOW_KEY).toBe('mcpuiVerifierInteractive');
  });

  it('probeHostOpenLinkPolicy reports hostLinkAffordance=true only for accepted https urls', () => {
    const accepted = probeHostOpenLinkPolicy('https://example.com/path', { timestamp: 't-accept' });
    expect(accepted).toEqual({
      ok: true,
      reason: 'host policy accepted https url',
      timestamp: 't-accept',
      url: 'https://example.com/path',
      hostLinkAffordance: true,
    });
  });

  it('probeHostOpenLinkPolicy reports hostLinkAffordance=false for every unsafe scheme path', () => {
    const cases = [
      { url: 'javascript:alert(1)', reason: 'open-link url is not allowed: javascript:' },
      { url: 'data:text/html,boom', reason: 'open-link url is not allowed: data:' },
      { url: 'http://example.com', reason: 'open-link url is not allowed: http:' },
      { url: '/local/path', reason: 'open-link url is invalid: /local/path' },
      { url: 'not a url', reason: 'open-link url is invalid: not a url' },
      { url: '', reason: 'open-link url is required' },
    ];
    for (const { url, reason } of cases) {
      const verdict = probeHostOpenLinkPolicy(url, { timestamp: 't-reject' });
      expect(verdict.ok).toBe(false);
      expect(verdict.hostLinkAffordance).toBe(false);
      expect(verdict.reason).toBe(reason);
      expect(verdict.url).toBe(url);
      expect(verdict.timestamp).toBe('t-reject');
    }
  });

  it('probeUnsafeOpenLink dispatches through the guest bridge and surfaces a hostLinkAffordance=false host verdict', () => {
    const posted = [];
    const target = {
      [MCPUI_FORGE_GUEST_KEY]: {
        openLink(url) { posted.push(url); },
      },
    };
    const result = probeUnsafeOpenLink(target, { timestamp: 't-unsafe' });
    expect(posted).toEqual([MCPUI_VERIFIER_UNSAFE_LINK_PROBE_URL]);
    expect(result.ok).toBe(true);
    expect(result.timestamp).toBe('t-unsafe');
    expect(result.url).toBe(MCPUI_VERIFIER_UNSAFE_LINK_PROBE_URL);
    expect(result.hostPolicy.ok).toBe(false);
    expect(result.hostPolicy.hostLinkAffordance).toBe(false);
    expect(result.hostPolicy.reason).toBe('open-link url is not allowed: javascript:');
    expect(result.reason).toBe(
      `unsafe openLink dispatched; host policy rejects: ${result.hostPolicy.reason}`,
    );
  });
});
