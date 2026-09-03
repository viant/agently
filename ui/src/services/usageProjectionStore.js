const STORAGE_PREFIX = 'agently.usageProjection.v1:';
const MAX_ENTRIES = 200;

function storageFor(target = null) {
  if (target) return target;
  if (typeof window === 'undefined') return null;
  try {
    return window.localStorage || null;
  } catch (_) {
    return null;
  }
}

function storageKey(conversationId = '') {
  const id = String(conversationId || '').trim();
  return id ? `${STORAGE_PREFIX}${id}` : '';
}

function tokenCount(value) {
  const count = Number(value);
  return Number.isFinite(count) && count > 0 ? Math.trunc(count) : 0;
}

export function recordConversationProjectionUsage({
  conversationId = '',
  turnId = '',
  projection = null,
} = {}, targetStorage = null) {
  const key = storageKey(conversationId);
  const tokensFreed = tokenCount(projection?.tokensFreed ?? projection?.TokensFreed);
  if (!key || !tokensFreed) return false;
  const storage = storageFor(targetStorage);
  if (!storage) return false;
  try {
    const previous = JSON.parse(storage.getItem(key) || '{}');
    const entries = Array.isArray(previous?.entries) ? previous.entries : [];
    const identity = String(turnId || projection?.turnId || '').trim() || `unknown:${Date.now()}`;
    const nextEntry = {
      turnId: identity,
      tokensFreed,
      scope: String(projection?.scope || projection?.Scope || '').trim(),
      reason: String(projection?.reason || projection?.Reason || '').trim(),
      recordedAt: new Date().toISOString(),
    };
    const nextEntries = [
      ...entries.filter((entry) => String(entry?.turnId || '').trim() !== identity),
      nextEntry,
    ].slice(-MAX_ENTRIES);
    storage.setItem(key, JSON.stringify({ conversationId: String(conversationId).trim(), entries: nextEntries }));
    return true;
  } catch (_) {
    return false;
  }
}

export function readConversationProjectionUsage(conversationId = '', targetStorage = null) {
  const key = storageKey(conversationId);
  const storage = storageFor(targetStorage);
  if (!key || !storage) return { entries: [], tokensFreed: 0 };
  try {
    const value = JSON.parse(storage.getItem(key) || '{}');
    const entries = Array.isArray(value?.entries) ? value.entries : [];
    return {
      entries,
      tokensFreed: entries.reduce((sum, entry) => sum + tokenCount(entry?.tokensFreed), 0),
    };
  } catch (_) {
    return { entries: [], tokensFreed: 0 };
  }
}
