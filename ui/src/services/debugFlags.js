export function isDebugFlagEnabled(key = '', envLevel = '') {
  const normalizedKey = String(key || '').trim();
  if (!normalizedKey) return false;
  const normalizedEnv = String(envLevel || '').trim().toLowerCase();
  if (normalizedEnv === 'debug') return true;
  if (typeof window === 'undefined') return false;
  try {
    const raw = String(window.localStorage?.getItem(normalizedKey) || '').trim().toLowerCase();
    if (['1', 'true', 'on', 'yes'].includes(raw)) return true;
    if (['0', 'false', 'off', 'no'].includes(raw)) return false;
  } catch (_) {
    return false;
  }
  return false;
}

export function isStreamDebugEnabled() {
  return isDebugFlagEnabled('agently.debugStream', import.meta?.env?.VITE_FORGE_LOG_LEVEL);
}

export function isExecutorDebugEnabled() {
  return isDebugFlagEnabled('agently.debugExecutor', import.meta?.env?.VITE_FORGE_LOG_LEVEL);
}
