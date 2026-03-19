import { client } from './agentlyClient';

const STORAGE_KEY = 'agently.conversationSeedTitles.v1';
const MAX_TITLE_CHARS = 200;

function normalizeSeedTitle(value = '') {
  const collapsed = String(value || '').replace(/\s+/g, ' ').trim();
  if (!collapsed) return '';
  return collapsed.slice(0, MAX_TITLE_CHARS);
}

function isUUID(value = '') {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(String(value || '').trim());
}

function basename(value = '') {
  const text = String(value || '').trim().replace(/\/+$/, '');
  if (!text) return '';
  const parts = text.split('/');
  return String(parts[parts.length - 1] || '').trim();
}

function metadataTitle(row = {}) {
  try {
    const raw = String(row?.Metadata || row?.metadata || '').trim();
    if (!raw) return '';
    const parsed = JSON.parse(raw);
    const ctx = parsed?.context || {};
    const workdir = String(ctx?.resolvedWorkdir || ctx?.ResolvedWorkdir || ctx?.workdir || '').trim();
    const base = basename(workdir);
    if (!base) return '';
    const agent = String(row?.AgentId || row?.agentId || row?.Agent || row?.agent || '').trim();
    if (agent && !/^(chatter|simple)$/i.test(agent)) {
      return `${agent} · ${base}`;
    }
    return base;
  } catch (_) {
    return '';
  }
}

function previewTitle(row = {}) {
  const candidates = [
    row?.Preview,
    row?.preview,
    row?.Summary,
    row?.summary,
    row?.Prompt,
    row?.prompt,
    row?.Query,
    row?.query
  ];
  for (const value of candidates) {
    const title = normalizeSeedTitle(value);
    if (title) return title;
  }
  return '';
}

function readMap() {
  if (typeof window === 'undefined') return {};
  try {
    const raw = window.localStorage?.getItem(STORAGE_KEY) || '{}';
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch (_) {
    return {};
  }
}

function writeMap(value) {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage?.setItem(STORAGE_KEY, JSON.stringify(value || {}));
  } catch (_) {}
}

export function rememberConversationSeedTitle(conversationID, userPrompt) {
  const id = String(conversationID || '').trim();
  const title = normalizeSeedTitle(userPrompt);
  if (!id || !title) return;
  const map = readMap();
  if (String(map[id] || '').trim()) return;
  map[id] = title;
  writeMap(map);
  // Persist the title to the backend so it survives across sessions.
  persistConversationTitle(id, title);
  if (typeof window !== 'undefined') {
    try {
      window.dispatchEvent(new CustomEvent('agently:conversation-title-seed', {
        detail: { id, title }
      }));
    } catch (_) {}
  }
}

function persistConversationTitle(conversationID, title) {
  const id = String(conversationID || '').trim();
  const text = String(title || '').trim();
  if (!id || !text) return;
  try {
    client.updateConversation(id, { title: text }).catch(() => {});
  } catch (_) {}
}

export function getConversationSeedTitle(conversationID) {
  const id = String(conversationID || '').trim();
  if (!id) return '';
  const map = readMap();
  return normalizeSeedTitle(map[id] || '');
}

export function resolveConversationTitle(row) {
  const id = String(row?.Id || row?.id || '').trim();
  const rawTitle = String(row?.Title || row?.title || '').trim();
  const needsSeed = !rawTitle || rawTitle === id || isUUID(rawTitle);
  if (!needsSeed) return rawTitle;
  const seed = getConversationSeedTitle(id);
  if (seed) return seed;
  const preview = previewTitle(row);
  if (preview) return preview;
  const meta = metadataTitle(row);
  if (meta) return meta;
  if (rawTitle) return rawTitle;
  return id || 'Conversation';
}
