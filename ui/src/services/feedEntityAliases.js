const aliasesByConversation = new Map();
const listeners = new Set();
let aliasVersion = 0;

function notify() {
  aliasVersion += 1;
  listeners.forEach((listener) => listener());
}

export function subscribeFeedEntityAliases(listener) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function getFeedEntityAliasVersion() {
  return aliasVersion;
}

function valueAtPath(root, path = '') {
  return String(path || '').split('.').filter(Boolean).reduce((value, key) => (
    value && typeof value === 'object' ? value[key] : undefined
  ), root);
}

function escapeRegExp(value = '') {
  return String(value).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

export function registerFeedEntityAlias(payload = null, conversationId = '') {
  const id = String(conversationId || '').trim();
  const config = payload?.ui?.entity;
  if (!id || !config || typeof config !== 'object') return false;
  const entityType = String(config.type || '').trim();
  const entityId = String(valueAtPath(payload?.data, config.idPath) || '').trim();
  const label = String(valueAtPath(payload?.data, config.labelPath) || '').trim();
  if (!entityType || !entityId || !label) return false;
  const current = aliasesByConversation.get(id) || [];
  const next = current.filter((entry) => !(entry.type === entityType && entry.id === entityId));
  next.push({ type: entityType, id: entityId, label });
  aliasesByConversation.set(id, next);
  notify();
  return true;
}

export function rewriteFeedEntityAliases(text = '', conversationId = '') {
  let result = String(text || '');
  const aliases = aliasesByConversation.get(String(conversationId || '').trim()) || [];
  for (const alias of aliases) {
    if (result.includes(`@{${alias.type}:${alias.id} "`)) continue;
    const token = `@{${alias.type}:${alias.id} "${alias.label.replace(/"/g, '\\"')}"}`;
    result = result.replace(new RegExp('`?' + escapeRegExp(alias.id) + '`?', 'g'), token);
  }
  return result;
}

export function clearFeedEntityAliases(conversationId = '') {
  const id = String(conversationId || '').trim();
  if (id) aliasesByConversation.delete(id);
  else aliasesByConversation.clear();
  notify();
}
