import { ensureUIBridgeClientId } from 'forge/core';

export function detectWebFormFactor() {
  if (typeof window === 'undefined') return 'desktop';
  const width = Number(window.innerWidth || 0);
  if (width > 0 && width < 768) return 'phone';
  if (width > 0 && width < 1100) return 'tablet';
  return 'desktop';
}

export function subscribeWebFormFactor(onChange) {
  if (typeof window === 'undefined' || typeof window.addEventListener !== 'function') {
    return () => {};
  }
  let current = detectWebFormFactor();
  onChange?.(current);
  const handleResize = () => {
    const next = detectWebFormFactor();
    if (next === current) return;
    current = next;
    onChange?.(next);
  };
  window.addEventListener('resize', handleResize);
  return () => window.removeEventListener?.('resize', handleResize);
}

export function buildWebTargetContext() {
  return {
    platform: 'web',
    formFactor: detectWebFormFactor(),
    surface: 'browser',
    capabilities: ['markdown', 'chart', 'upload', 'code', 'diff'],
  };
}

export function buildWebClientContext() {
  return {
    kind: 'web',
    ...buildWebTargetContext(),
  };
}

function currentUIBridgeClientId() {
  if (typeof window === 'undefined') return '';
  try {
    return String(window.__forgeUIBridgeClientId || ensureUIBridgeClientId() || '').trim();
  } catch (_) {
    return '';
  }
}

export function buildWebQueryContext() {
  const uiClientId = currentUIBridgeClientId();
  return {
    client: buildWebClientContext(),
    uiClientId: uiClientId || undefined,
  };
}
