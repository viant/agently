/**
 * Tracks pending elicitations independently by conversation + elicitation id.
 * Root proxy elicitations can arrive concurrently from different child
 * conversations, so the UI must not store them as one global pending item.
 */
const pendingByKey = new Map();
let listeners = [];

function elicitationKey(item = {}) {
  const conversationId = String(item?.conversationId || item?.ConversationId || '').trim();
  const elicitationId = String(item?.elicitationId || item?.ElicitationId || '').trim();
  if (!conversationId || !elicitationId) return '';
  return `${conversationId}::${elicitationId}`;
}

function notify() {
  const list = getPendingElicitations();
  const first = list[0] || null;
  for (const fn of listeners) {
    try { fn(first, list); } catch (_) {}
  }
}

export const elicitationTracker = {
  setPending: (elicitation) => setPendingElicitation(elicitation),
  get pending() {
    return getPendingElicitation();
  },
  clear: () => clearPendingElicitation(),
  onChange: (fn) => onElicitationChange(fn),
};

// Convenience wrappers for backward compat. setPending is now an upsert.
export function setPendingElicitation(elicitation) {
  return upsertPendingElicitation(elicitation);
}

export function upsertPendingElicitation(elicitation) {
  const normalized = normalizeElicitationDialogState(elicitation);
  if (!normalized) return false;
  const key = elicitationKey(normalized);
  if (!key) return false;
  const previous = pendingByKey.get(key) || {};
  pendingByKey.set(key, { ...previous, ...normalized });
  notify();
  return true;
}

export function getPendingElicitation() {
  return getPendingElicitations()[0] || null;
}

export function getPendingElicitations() {
  return Array.from(pendingByKey.values());
}

export function clearPendingElicitation() {
  if (pendingByKey.size === 0) return;
  pendingByKey.clear();
  notify();
}

export function removePendingElicitation(target = {}, options = {}) {
  const elicitationId = String(target?.elicitationId || target?.ElicitationId || '').trim();
  const conversationId = String(target?.conversationId || target?.ConversationId || '').trim();
  if (!elicitationId && !conversationId) return false;
  const allConversationsForElicitation = options?.allConversationsForElicitation !== false;
  let removed = false;
  for (const [key, item] of Array.from(pendingByKey.entries())) {
    const itemElicitationId = String(item?.elicitationId || '').trim();
    const itemConversationId = String(item?.conversationId || '').trim();
    const matchesElicitation = elicitationId && itemElicitationId === elicitationId;
    const matchesConversation = conversationId && itemConversationId === conversationId;
    const shouldRemove = allConversationsForElicitation
      ? matchesElicitation && (!conversationId || matchesConversation || itemElicitationId === elicitationId)
      : matchesElicitation && matchesConversation;
    if (shouldRemove || (!elicitationId && matchesConversation)) {
      pendingByKey.delete(key);
      removed = true;
    }
  }
  if (removed) notify();
  return removed;
}

export function replacePendingElicitationsForConversation(conversationId = '', pendingElicitations = []) {
  const targetConversationId = String(conversationId || '').trim();
  if (!targetConversationId) return;
  const nextItems = (Array.isArray(pendingElicitations) ? pendingElicitations : [])
    .map((item) => normalizeElicitationDialogState(item, targetConversationId))
    .filter(Boolean);
  const nextKeys = new Set(nextItems.map(elicitationKey).filter(Boolean));
  let changed = false;

  for (const [key, item] of Array.from(pendingByKey.entries())) {
    if (String(item?.conversationId || '').trim() === targetConversationId && !nextKeys.has(key)) {
      pendingByKey.delete(key);
      changed = true;
    }
  }
  for (const item of nextItems) {
    const key = elicitationKey(item);
    if (!key) continue;
    const previous = pendingByKey.get(key) || {};
    const next = { ...previous, ...item };
    pendingByKey.set(key, next);
    changed = true;
  }
  if (changed) notify();
}

export function onElicitationChange(fn) {
  listeners.push(fn);
  return () => {
    listeners = listeners.filter((listener) => listener !== fn);
  };
}

export function normalizeElicitationDialogState(source = {}, fallbackConversationId = '') {
  const direct = source?.elicitation && typeof source.elicitation === 'object'
    ? { ...source.elicitation, ...source }
    : source;
  const elicitationId = String(direct?.elicitationId || direct?.ElicitationId || '').trim();
  const requestedSchema = direct?.requestedSchema || direct?.schema || null;
  if (!elicitationId || !requestedSchema || typeof requestedSchema !== 'object') {
    return null;
  }
  const callbackURL = String(direct?.callbackURL || direct?.callbackUrl || '').trim();
  const derivedConversationId = (() => {
    const explicit = String(direct?.conversationId || direct?.ConversationId || fallbackConversationId || '').trim();
    if (explicit) return explicit;
    const match = callbackURL.match(/\/v1\/(?:api\/)?conversations\/([^/]+)\/elicitation\//i);
    return match ? String(match[1] || '').trim() : '';
  })();
  return {
    elicitationId,
    conversationId: derivedConversationId,
    turnId: String(direct?.turnId || direct?.TurnId || '').trim(),
    message: String(direct?.message || direct?.content || '').trim(),
    requestedSchema,
    callbackURL,
    url: String(direct?.url || direct?.Url || '').trim(),
    mode: String(direct?.mode || direct?.Mode || '').trim(),
    status: String(direct?.status || '').trim()
  };
}

export function openElicitationDialog(source = {}, fallbackConversationId = '') {
  const normalized = normalizeElicitationDialogState(source, fallbackConversationId);
  if (!normalized) return false;
  setPendingElicitation(normalized);
  return true;
}
