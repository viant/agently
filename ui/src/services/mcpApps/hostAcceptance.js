// Host-side acceptance rules for guest postMessage envelopes received by the
// MCP UI browser host. Phase 6b origin + echo validation lives here so the
// AppRenderer wiring and tests share a single source of truth.

import { isEnvelopeCandidate } from './appproto.js';

export const OPAQUE_ORIGIN = 'null';

// hasSameOriginSandbox returns true when the iframe sandbox attribute opts in
// to the embedding origin. Same-origin renderer-route frames must then also
// validate event.origin against the embedding window.location.origin.
export function hasSameOriginSandbox(sandbox = '') {
  return String(sandbox || '')
    .split(/\s+/)
    .filter(Boolean)
    .includes('allow-same-origin');
}

// acceptsGuestMessage applies Phase 6b acceptance rules to a postMessage event.
// It is intentionally narrow: exact source-window identity, exact windowId
// echo, exact resourceUri echo, and an origin check that depends only on the
// iframe sandbox mode. There is no fuzzy matching and no widening fallback.
export function acceptsGuestMessage({
  event,
  expectedSourceWindow,
  expectedWindowId,
  expectedResourceUri,
  expectedSameOriginValue,
  sandbox = '',
} = {}) {
  if (!event || typeof event !== 'object') return false;
  if (!expectedSourceWindow) return false;
  if (event.source !== expectedSourceWindow) return false;
  if (!isEnvelopeCandidate(event.data)) return false;

  const sameOriginFrame = hasSameOriginSandbox(sandbox);
  const eventOrigin = String(event.origin == null ? '' : event.origin);
  if (sameOriginFrame) {
    const expected = String(expectedSameOriginValue || '');
    if (!expected) return false;
    if (eventOrigin !== expected) return false;
  } else if (eventOrigin && eventOrigin !== OPAQUE_ORIGIN) {
    return false;
  }

  const params = event.data?.params;
  if (!params || typeof params !== 'object') return false;
  const wantedWindowId = String(expectedWindowId || '');
  const wantedResourceUri = String(expectedResourceUri || '');
  if (!wantedWindowId || !wantedResourceUri) return false;
  if (String(params.windowId || '') !== wantedWindowId) return false;
  if (String(params.resourceUri || '') !== wantedResourceUri) return false;
  return true;
}
