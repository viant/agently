// Simple in-memory suppression for elicitation dialogs to prevent
// immediate re-open on fast polling updates. Not persisted.

const store = new Map(); // id -> expiresAt (ms since epoch)
const storeByEid = new Map(); // elicitationId -> expiresAt

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

export function markElicitationResolvedByEid(elicitationId, ttlMs = 10000) {
  if (!elicitationId) return;
  cleanup();
  const t = now() + Math.max(0, Number(ttlMs) || 0);
  storeByEid.set(String(elicitationId), t);
}

export function isElicitationEidSuppressed(elicitationId) {
  if (!elicitationId) return false;
  cleanup();
  const exp = storeByEid.get(String(elicitationId));
  return typeof exp === 'number' && exp > now();
}
