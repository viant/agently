import { ensureUIBridgeClientId } from 'forge/core';
import * as ForgeComponents from 'forge/components';

const responsiveResolverKey = ['forge', 'FormFactor', 'ForWidth'].join('');
const forgeFormFactorForWidth = ForgeComponents[responsiveResolverKey] || ((width) => {
  const value = Number(width);
  if (value > 0 && value < 768) return 'phone';
  if (value > 0 && value < 1100) return 'tablet';
  return 'desktop';
});

export function detectWebFormFactor() {
  if (typeof window === 'undefined') return 'desktop';
  return forgeFormFactorForWidth(window.innerWidth || 0);
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
