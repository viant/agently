import { useSyncExternalStore } from 'react';

export const CHANGE_FEED_ANCHORS = {
  COMPOSER_TOP: 'composer_top',
  SIDEBAR_TOP: 'sidebar_top',
  SIDEBAR_BOTTOM: 'sidebar_bottom'
};

const DEFAULT_CHANGE_FEED_ANCHOR = CHANGE_FEED_ANCHORS.COMPOSER_TOP;
const listeners = new Set();

let currentChangeFeed = {
  anchor: readStoredAnchor(),
  conversationId: '',
  workdir: '',
  changes: [],
  source: null,
  updatedAt: 0
};

function readStoredAnchor() {
  if (typeof window === 'undefined') return DEFAULT_CHANGE_FEED_ANCHOR;
  return normalizeChangeFeedAnchor(window.localStorage?.getItem('agently.changeFeedAnchor'));
}

export function normalizeChangeFeedAnchor(value = '') {
  switch (String(value || '').trim().toLowerCase()) {
    case CHANGE_FEED_ANCHORS.COMPOSER_TOP:
      return CHANGE_FEED_ANCHORS.COMPOSER_TOP;
    case CHANGE_FEED_ANCHORS.SIDEBAR_TOP:
      return CHANGE_FEED_ANCHORS.SIDEBAR_TOP;
    case CHANGE_FEED_ANCHORS.SIDEBAR_BOTTOM:
      return CHANGE_FEED_ANCHORS.SIDEBAR_BOTTOM;
    default:
      return CHANGE_FEED_ANCHORS.COMPOSER_TOP;
  }
}

function notify() {
  for (const listener of listeners) listener();
}

function subscribe(listener) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function getSnapshot() {
  return currentChangeFeed;
}

function parseMaybeJSON(value) {
  if (!value) return null;
  if (typeof value === 'string') {
    const text = value.trim();
    if (!text) return null;
    try {
      return JSON.parse(text);
    } catch (_) {
      return null;
    }
  }
  if (typeof value !== 'object') return null;
  const inlineBody = typeof value?.inlineBody === 'string' ? value.inlineBody.trim() : '';
  if (inlineBody) {
    try {
      return JSON.parse(inlineBody);
    } catch (_) {
      return null;
    }
  }
  return value;
}

function normalizeToolKey(value = '') {
  return String(value || '').toLowerCase().replace(/[^a-z0-9]/g, '');
}

function isPatchSnapshotTool(step = {}) {
  const key = normalizeToolKey(step?.toolName || '');
  return key.includes('systempatch') && key.includes('snapshot');
}

function normalizeChange(change = {}, index = 0) {
  const url = String(change?.url || change?.URL || change?.uri || '').trim();
  const origUrl = String(change?.origUrl || change?.OrigURL || change?.origURI || '').trim();
  const diff = String(change?.diff || change?.Diff || '').trim();
  const kind = String(change?.kind || change?.Kind || '').trim().toLowerCase() || 'modified';
  const candidate = url || origUrl;
  const name = candidate ? candidate.split('/').pop() : `change-${index + 1}`;
  if (!url && !origUrl && !diff) return null;
  return {
    id: String(change?.id || `${kind}:${candidate || index}`),
    kind,
    name,
    url,
    origUrl,
    diff
  };
}

function extractSnapshotPayload(payload = null) {
  const parsed = parseMaybeJSON(payload);
  if (!parsed || typeof parsed !== 'object') return null;
  const raw = parsed?.output || parsed?.result || parsed;
  const workdir = String(raw?.workdir || raw?.Workdir || '').trim();
  const status = String(raw?.status || raw?.Status || '').trim().toLowerCase();
  const changes = Array.isArray(raw?.changes || raw?.Changes)
    ? (raw?.changes || raw?.Changes).map(normalizeChange).filter(Boolean)
    : [];
  if (changes.length === 0 && status !== 'ok') return null;
  return { workdir, changes, raw };
}

function extractLatestChangeSnapshot(rows = []) {
  for (let rowIndex = rows.length - 1; rowIndex >= 0; rowIndex -= 1) {
    const row = rows[rowIndex];
    const executions = Array.isArray(row?.executions) ? row.executions : [];
    for (let execIndex = executions.length - 1; execIndex >= 0; execIndex -= 1) {
      const execution = executions[execIndex];
      const steps = Array.isArray(execution?.steps) ? execution.steps : [];
      for (let stepIndex = steps.length - 1; stepIndex >= 0; stepIndex -= 1) {
        const step = steps[stepIndex];
        if (!isPatchSnapshotTool(step)) continue;
        const parsed = extractSnapshotPayload(step?.responsePayload)
          || extractSnapshotPayload(step?.requestPayload);
        if (!parsed) continue;
        return {
          workdir: parsed.workdir,
          changes: parsed.changes,
          source: step
        };
      }
    }
  }
  return null;
}

export function setChangeFeedAnchor(anchor = DEFAULT_CHANGE_FEED_ANCHOR) {
  const normalized = normalizeChangeFeedAnchor(anchor);
  if (typeof window !== 'undefined') {
    try {
      window.localStorage?.setItem('agently.changeFeedAnchor', normalized);
    } catch (_) {}
  }
  currentChangeFeed = {
    ...currentChangeFeed,
    anchor: normalized,
    updatedAt: Date.now()
  };
  notify();
}

export function publishChangeFeed({ conversationId = '', rows = [] } = {}) {
  const latest = extractLatestChangeSnapshot(Array.isArray(rows) ? rows : []);
  currentChangeFeed = {
    ...currentChangeFeed,
    conversationId: String(conversationId || '').trim(),
    workdir: String(latest?.workdir || '').trim(),
    changes: Array.isArray(latest?.changes) ? latest.changes : [],
    source: latest?.source || null,
    updatedAt: Date.now()
  };
  notify();
}

export function clearChangeFeed(conversationId = '') {
  currentChangeFeed = {
    ...currentChangeFeed,
    conversationId: String(conversationId || '').trim(),
    workdir: '',
    changes: [],
    source: null,
    updatedAt: Date.now()
  };
  notify();
}

export function useChangeFeed() {
  return useSyncExternalStore(subscribe, getSnapshot);
}
