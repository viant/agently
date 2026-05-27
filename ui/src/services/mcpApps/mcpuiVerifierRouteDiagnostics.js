import {
  MCPUI_FORGE_GUEST_KEY,
  MCPUI_HOST_KEY,
} from './forgeGuestBridge.js';
import { classifyOpenLinkURL } from './bridge.js';

export const MCPUI_VERIFIER_ROUTE_WINDOW_KEY = 'mcpuiVerifierInteractive';
export const MCPUI_VERIFIER_PROBE_LABEL = 'mcpui-verifier route diagnostic probe';
export const MCPUI_VERIFIER_TOOL_PROBE_LABEL = 'mcpui-verifier route toolCall probe';
export const MCPUI_VERIFIER_LINK_PROBE_URL = 'https://example.com/';
export const MCPUI_VERIFIER_UNSAFE_LINK_PROBE_URL = 'javascript:alert("mcpui-verifier-unsafe-probe")';

export function readVerifierDiagnosticsSnapshot(target) {
  if (!target || typeof target !== 'object') {
    return emptySnapshot();
  }
  const guest = target[MCPUI_FORGE_GUEST_KEY];
  const host = target[MCPUI_HOST_KEY];
  const hostObj = host && typeof host === 'object' ? host : null;
  const windowId = String(hostObj?.windowId || '').trim();
  const resourceUri = String(hostObj?.resourceUri || '').trim();
  return {
    guestBridgeInstalled: Boolean(guest && typeof guest.message === 'function'),
    hostReady: windowId !== '' || resourceUri !== '',
    windowId,
    resourceUri,
    allowedTools: Array.isArray(hostObj?.allowedTools) ? hostObj.allowedTools.slice() : [],
    allowedToolBundles: Array.isArray(hostObj?.allowedToolBundles) ? hostObj.allowedToolBundles.slice() : [],
  };
}

export function probeDirectBridge(target, options = {}) {
  const timestamp = String(options.timestamp || new Date().toISOString());
  if (!target || typeof target !== 'object') {
    return { ok: false, reason: 'window unavailable', timestamp };
  }
  const guest = target[MCPUI_FORGE_GUEST_KEY];
  if (!guest || typeof guest.message !== 'function') {
    return { ok: false, reason: 'guest bridge unavailable', timestamp };
  }
  try {
    guest.message(`${MCPUI_VERIFIER_PROBE_LABEL} @ ${timestamp}`);
    return { ok: true, reason: 'parent.postMessage dispatched', timestamp };
  } catch (err) {
    const message = err && err.message ? err.message : String(err);
    return { ok: false, reason: `invocation failed: ${message}`, timestamp };
  }
}

export function probeDirectToolCall(target, options = {}) {
  const timestamp = String(options.timestamp || new Date().toISOString());
  if (!target || typeof target !== 'object') {
    return { ok: false, reason: 'window unavailable', timestamp };
  }
  const guest = target[MCPUI_FORGE_GUEST_KEY];
  if (!guest || typeof guest.toolCall !== 'function') {
    return { ok: false, reason: 'guest bridge unavailable', timestamp };
  }
  try {
    guest.toolCall(
      'system/os:getEnv',
      { names: ['HOME'] },
      `${MCPUI_VERIFIER_TOOL_PROBE_LABEL} @ ${timestamp}`,
    );
    return { ok: true, reason: 'toolCall dispatched', timestamp };
  } catch (err) {
    const message = err && err.message ? err.message : String(err);
    return { ok: false, reason: `toolCall failed: ${message}`, timestamp };
  }
}

export function probeDirectOpenLink(target, options = {}) {
  const timestamp = String(options.timestamp || new Date().toISOString());
  if (!target || typeof target !== 'object') {
    return { ok: false, reason: 'window unavailable', timestamp };
  }
  const guest = target[MCPUI_FORGE_GUEST_KEY];
  if (!guest || typeof guest.openLink !== 'function') {
    return { ok: false, reason: 'guest bridge unavailable', timestamp };
  }
  try {
    guest.openLink(MCPUI_VERIFIER_LINK_PROBE_URL);
    return { ok: true, reason: 'openLink dispatched', timestamp };
  } catch (err) {
    const message = err && err.message ? err.message : String(err);
    return { ok: false, reason: `openLink failed: ${message}`, timestamp };
  }
}

export function probeUnsafeOpenLink(target, options = {}) {
  const timestamp = String(options.timestamp || new Date().toISOString());
  const url = String(options.url || MCPUI_VERIFIER_UNSAFE_LINK_PROBE_URL);
  const hostPolicy = probeHostOpenLinkPolicy(url, { timestamp });
  if (!target || typeof target !== 'object') {
    return { ok: false, reason: 'window unavailable', timestamp, url, hostPolicy };
  }
  const guest = target[MCPUI_FORGE_GUEST_KEY];
  if (!guest || typeof guest.openLink !== 'function') {
    return { ok: false, reason: 'guest bridge unavailable', timestamp, url, hostPolicy };
  }
  try {
    guest.openLink(url);
    return {
      ok: true,
      reason: hostPolicy.ok
        ? 'unsafe openLink dispatched; host policy unexpectedly accepted it'
        : `unsafe openLink dispatched; host policy rejects: ${hostPolicy.reason}`,
      timestamp,
      url,
      hostPolicy,
    };
  } catch (err) {
    const message = err && err.message ? err.message : String(err);
    return { ok: false, reason: `openLink failed: ${message}`, timestamp, url, hostPolicy };
  }
}

// probeHostOpenLinkPolicy is the diagnostics-path runtime proof that the host
// browser layer enforces the MVP open-link policy: a rejected URL never
// surfaces a host link affordance. It mirrors the deterministic decision made
// in handleGuestEnvelope by calling the same pure classifier, so the verifier
// route can demonstrate at runtime that javascript:, data:, relative and
// malformed URLs are blocked before any host-owned anchor would be rendered.
export function probeHostOpenLinkPolicy(url, options = {}) {
  const timestamp = String(options.timestamp || new Date().toISOString());
  const verdict = classifyOpenLinkURL(url);
  if (verdict.ok) {
    return {
      ok: true,
      reason: 'host policy accepted https url',
      timestamp,
      url: verdict.url,
      hostLinkAffordance: true,
    };
  }
  return {
    ok: false,
    reason: verdict.reason,
    timestamp,
    url: String(url || ''),
    hostLinkAffordance: false,
  };
}

function emptySnapshot() {
  return {
    guestBridgeInstalled: false,
    hostReady: false,
    windowId: '',
    resourceUri: '',
    allowedTools: [],
    allowedToolBundles: [],
  };
}
