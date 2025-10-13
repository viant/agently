// Simple in-memory suppression for elicitation dialogs to prevent
// immediate re-open on fast polling updates. Not persisted.

const store = new Map(); // id -> expiresAt (ms since epoch)

function now() { return Date.now(); }

function cleanup() {
  const t = now();
  for (const [k, exp] of store.entries()) {
    if (typeof exp === 'number' && exp <= t) store.delete(k);
  }
}

export function markElicitationShown(id, ttlMs = 3000) {
  if (!id) return;
  cleanup();
  const t = now() + Math.max(0, Number(ttlMs) || 0);
  store.set(String(id), t);
}

export function isElicitationSuppressed(id) {
  if (!id) return false;
  cleanup();
  const exp = store.get(String(id));
  return typeof exp === 'number' && exp > now();
}

export function clearElicitation(id) {
  if (!id) return;
  store.delete(String(id));
}

