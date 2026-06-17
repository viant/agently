import {
  addWindow,
  activeWindows,
  getFormSignal,
  getViewSignal,
  publishUIBridgeSnapshotNow,
  removeWindow,
  selectedTabId,
  selectedWindowId
} from 'forge/core';
import { deriveHostedWorkspaceRestoreStateFromTranscriptTurns } from 'agently-core-ui-sdk/workspaceRestore';
import { generateIntHash } from '../../../../forge/src/utils/hash.js';

export const CHAT_WINDOW_KEY = 'chat/new';
export const MAIN_CHAT_WINDOW_ID = CHAT_WINDOW_KEY;
const LEGACY_SELECTED_CONVERSATION_KEY = 'agently.selectedConversationId';
const WORKSPACE_SELECTION_KEY = 'agently.selectedWorkspaceWindowId';
const WORKSPACE_STATE_KEY = 'agently.workspaceState';
const WORKSPACE_PRESENTATION_MODE_KEY = 'agently.workspacePresentationMode';

function uiStateStorage() {
  if (typeof window === 'undefined') return null;
  return window.sessionStorage || null;
}

function conversationPathForID(conversationId = '') {
  const id = String(conversationId || '').trim();
  if (!id) {
    if (typeof window !== 'undefined') {
      const current = String(window.location?.pathname || '').trim();
      if (current.startsWith('/ui/')) return '/ui';
    }
    return '/';
  }
  const encoded = encodeURIComponent(id);
  if (typeof window !== 'undefined') {
    const port = String(window.location?.port || '').trim();
    const host = String(window.location?.hostname || '').trim();
    if (port === '5173' || port === '5175' || port === '5176' || host === '127.0.0.1' || host === 'localhost') {
      return `/conversation/${encoded}`;
    }
  }
  return `/v1/conversation/${encoded}`;
}

function syncMainConversationPath(conversationId = '') {
  if (typeof window === 'undefined') return;
  const target = conversationPathForID(conversationId);
  const current = String(window.location?.pathname || '').trim();
  if (current === target) return;
  if (current.startsWith('/v1/api/')) return;
  try {
    window.history.replaceState(window.history.state, '', target);
  } catch (_) {}
}

function scopedSelectionKey(windowId = '') {
  const id = String(windowId || '').trim();
  return id ? `${LEGACY_SELECTED_CONVERSATION_KEY}:${id}` : LEGACY_SELECTED_CONVERSATION_KEY;
}

function workspaceSelectionKey(conversationId = '') {
  const id = String(conversationId || '').trim();
  return id ? `${WORKSPACE_SELECTION_KEY}:${id}` : WORKSPACE_SELECTION_KEY;
}

function workspaceStateKey(conversationId = '') {
  const id = String(conversationId || '').trim();
  return id ? `${WORKSPACE_STATE_KEY}:${id}` : WORKSPACE_STATE_KEY;
}

function workspacePresentationModeKey(conversationId = '') {
  const id = String(conversationId || '').trim();
  return id ? `${WORKSPACE_PRESENTATION_MODE_KEY}:${id}` : WORKSPACE_PRESENTATION_MODE_KEY;
}

const RUNNING_TRANSCRIPT_STATUSES = new Set(['running', 'thinking', 'processing', 'waiting_for_user', 'in_progress']);

function dispatchWorkspaceStateEvent(conversationId = '') {
  const convID = String(conversationId || '').trim();
  if (!convID || typeof window === 'undefined') return;
  try {
    window.dispatchEvent(new CustomEvent('agently:workspace-state', { detail: { conversationId: convID } }));
  } catch (_) {}
}

function transcriptTurnsHaveRunningStatus(turns = []) {
  return (Array.isArray(turns) ? turns : []).some((turn) => {
    const status = String(turn?.status || turn?.Status || '').trim().toLowerCase();
    return RUNNING_TRANSCRIPT_STATUSES.has(status);
  });
}

export function nudgeWorkspaceRestore(conversationId = '', attempts = 3) {
  const convID = String(conversationId || '').trim();
  if (!convID || typeof window === 'undefined' || typeof window.setTimeout !== 'function') return;
  const total = Math.max(1, Number(attempts || 1));
  const delays = [0, 250, 1000, 2000].slice(0, total);
  for (const delay of delays) {
    window.setTimeout(() => {
      try {
        window.dispatchEvent(new CustomEvent('agently:workspace-state', { detail: { conversationId: convID } }));
      } catch (_) {}
    }, delay);
  }
}

export function isMainChatWindowId(windowId = '') {
  return String(windowId || '').trim() === MAIN_CHAT_WINDOW_ID;
}

export function focusWindow(win = null) {
  if (!win?.windowId) return null;
  selectedTabId.value = win.windowId;
  selectedWindowId.value = win.windowId;
  return win;
}

export function getWindowById(windowId = '') {
  const id = String(windowId || '').trim();
  if (!id) return null;
  const windows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  return windows.find((entry) => String(entry?.windowId || '').trim() === id) || null;
}

function computeWorkspaceWindowId(windowKey = '', parameters = {}) {
  const key = String(windowKey || '').trim();
  if (!key) return '';
  const hash = parameters && typeof parameters === 'object' && Object.keys(parameters).length > 0
    ? generateIntHash(parameters)
    : '';
  return hash ? `${key}_${hash}` : key;
}

export function getSelectedWindow() {
  return getWindowById(selectedWindowId.peek?.() || selectedTabId.peek?.() || '');
}

export function isLinkedChildWindow(win = null) {
  if (!win) return false;
  if (String(win?.windowKey || '').trim() !== CHAT_WINDOW_KEY) return false;
  if (isMainChatWindowId(String(win?.windowId || '').trim())) return false;
  return !!String(win?.parameters?.linkedParent?.conversationId || '').trim();
}

export function linkedParentConversationId(win = null) {
  return String(win?.parameters?.linkedParent?.conversationId || '').trim();
}

export function linkedParentWindowId(win = null) {
  return String(win?.parameters?.linkedParent?.windowId || '').trim();
}

function isChatOwnedWindow(win = null) {
  if (!win || typeof win !== 'object') return false;
  const windowId = String(win?.windowId || '').trim();
  const windowKey = String(win?.windowKey || '').trim();
  const parentKey = String(win?.parentKey || '').trim();
  if (windowId === MAIN_CHAT_WINDOW_ID || windowKey === CHAT_WINDOW_KEY) return true;
  if (parentKey === MAIN_CHAT_WINDOW_ID) return true;
  return false;
}

function removeNonChatTopLevelWindows() {
  const windows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  for (const win of windows) {
    const windowId = String(win?.windowId || '').trim();
    if (!windowId) continue;
    if (isChatOwnedWindow(win)) continue;
    removeWindow(windowId);
  }
}

export function getScopedConversationSelection(windowId = '') {
  const storage = uiStateStorage();
  if (!storage) return '';
  const scoped = String(storage.getItem(scopedSelectionKey(windowId)) || '').trim();
  if (scoped) return scoped;
  if (isMainChatWindowId(windowId)) {
    return String(storage.getItem(LEGACY_SELECTED_CONVERSATION_KEY) || '').trim();
  }
  return '';
}

export function currentConversationIdFromPath(pathname = '') {
  const raw = String(pathname || '').trim();
  if (!raw) return '';
  const prefixes = ['/v1/conversation/', '/conversation/', '/ui/conversation/'];
  for (const prefix of prefixes) {
    if (!raw.startsWith(prefix)) continue;
    const rest = raw.slice(prefix.length);
    const id = String(rest.split(/[/?#]/)[0] || '').trim();
    if (id) return decodeURIComponent(id);
  }
  return '';
}

export function resolveConversationSelection(windowId = '') {
  if (isMainChatWindowId(windowId)) {
    if (typeof window !== 'undefined') {
      const fromPath = currentConversationIdFromPath(window.location?.pathname);
      if (fromPath) return fromPath;
    }
    return getScopedConversationSelection(windowId);
  }
  return getScopedConversationSelection(windowId);
}

export function setScopedConversationSelection(windowId = '', conversationId = '') {
  const storage = uiStateStorage();
  if (!storage) return;
  const id = String(conversationId || '').trim();
  const targetWindowId = String(windowId || '').trim();
  try {
    if (id) {
      storage.setItem(scopedSelectionKey(targetWindowId), id);
    } else {
      storage.removeItem(scopedSelectionKey(targetWindowId));
    }
    if (isMainChatWindowId(targetWindowId)) {
      if (id) storage.setItem(LEGACY_SELECTED_CONVERSATION_KEY, id);
      else storage.removeItem(LEGACY_SELECTED_CONVERSATION_KEY);
    }
  } catch (_) {}
}

export function getScopedWorkspaceSelection(conversationId = '') {
  const storage = uiStateStorage();
  if (!storage) return '';
  const id = String(conversationId || '').trim();
  if (!id) return '';
  return String(storage.getItem(workspaceSelectionKey(id)) || '').trim();
}

export function setScopedWorkspaceSelection(conversationId = '', windowId = '') {
  const storage = uiStateStorage();
  if (!storage) return;
  const convID = String(conversationId || '').trim();
  if (!convID) return;
  const targetWindowId = String(windowId || '').trim();
  try {
    if (targetWindowId) {
      storage.setItem(workspaceSelectionKey(convID), targetWindowId);
    } else {
      storage.removeItem(workspaceSelectionKey(convID));
    }
  } catch (_) {}
}

export function getScopedWorkspaceState(conversationId = '') {
  const storage = uiStateStorage();
  if (!storage) return null;
  const id = String(conversationId || '').trim();
  if (!id) return null;
  try {
    const raw = String(storage.getItem(workspaceStateKey(id)) || '').trim();
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object') return null;
    return parsed;
  } catch (_) {
    return null;
  }
}

export function getScopedWorkspaceWindowsState(conversationId = '') {
  const saved = getScopedWorkspaceState(conversationId);
  if (!saved) return [];
  const list = Array.isArray(saved?.windows) ? saved.windows : [saved];
  return list
    .map((entry) => normalizeWorkspaceStateSnapshot(entry, { preferLiveSignals: false }))
    .filter(Boolean);
}

export function getScopedWorkspacePresentationMode(conversationId = '') {
  const storage = uiStateStorage();
  if (!storage) return 'split';
  const id = String(conversationId || '').trim();
  if (!id) return 'split';
  const raw = String(storage.getItem(workspacePresentationModeKey(id)) || '').trim().toLowerCase();
  return raw === 'full' ? 'full' : 'split';
}

export function setScopedWorkspacePresentationMode(conversationId = '', mode = 'split') {
  const storage = uiStateStorage();
  if (!storage) return;
  const id = String(conversationId || '').trim();
  if (!id) return;
  const next = String(mode || '').trim().toLowerCase() === 'full' ? 'full' : 'split';
  try {
    storage.setItem(workspacePresentationModeKey(id), next);
  } catch (_) {}
}

export function hasScopedWorkspaceState(conversationId = '') {
  return !!getScopedWorkspaceState(conversationId);
}

export function setScopedWorkspaceState(conversationId = '', win = null) {
  const storage = uiStateStorage();
  if (!storage) return;
  const convID = String(conversationId || '').trim();
  if (!convID) return;
  try {
    if (!win || typeof win !== 'object') {
      storage.removeItem(workspaceStateKey(convID));
      return;
    }
    const entries = Array.isArray(win) ? win : [win];
    const savedSnapshotsByWindowId = new Map(
      getScopedWorkspaceWindowsState(convID)
        .map((entry) => [String(entry?.windowId || '').trim(), entry])
        .filter(([windowId]) => !!windowId)
    );
    const payloads = entries
      .map((entry) => {
        const windowKey = String(entry?.windowKey || '').trim();
        const parameters = entry?.parameters && typeof entry.parameters === 'object' ? entry.parameters : {};
        const computedWindowId = computeWorkspaceWindowId(windowKey, parameters);
        const windowId = String(entry?.windowId || computedWindowId || '').trim();
        const savedSnapshot = windowId ? savedSnapshotsByWindowId.get(windowId) : null;
        const mergedEntry = savedSnapshot && entry && typeof entry === 'object'
          ? mergeWorkspaceSnapshotValue(savedSnapshot, entry)
          : entry;
        return normalizeWorkspaceStateSnapshot(mergedEntry, { preferLiveSignals: true });
      })
      .filter(Boolean);
    if (payloads.length === 0) {
      storage.removeItem(workspaceStateKey(convID));
      return;
    }
    const payload = payloads.length === 1
      ? payloads[0]
      : { windows: payloads };
    storage.setItem(workspaceStateKey(convID), JSON.stringify(payload));
  } catch (_) {}
}

export function resolveWorkspaceWindowsForConversation(conversationId = '') {
  const convID = String(conversationId || '').trim();
  if (!convID) return [];
  const windows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  return windows.filter((entry) => {
    if (String(entry?.windowKey || '').trim() === CHAT_WINDOW_KEY) return false;
    if (String(entry?.presentation || '').trim().toLowerCase() !== 'hosted') return false;
    if (String(entry?.region || '').trim().toLowerCase() !== 'chat.top') return false;
    return String(entry?.conversationId || '').trim() === convID;
  });
}

export function resolveWorkspaceWindowForConversation(conversationId = '') {
  const convID = String(conversationId || '').trim();
  if (!convID) return null;
  const liveOwnedWindows = resolveWorkspaceWindowsForConversation(convID);
  if (liveOwnedWindows.length > 0) {
    const preferredIds = [
      String(selectedWindowId.peek?.() || selectedWindowId.value || '').trim(),
      String(selectedTabId.peek?.() || selectedTabId.value || '').trim(),
      getScopedWorkspaceSelection(convID),
    ].filter(Boolean);
    for (const preferredId of preferredIds) {
      const matched = liveOwnedWindows.find((entry) => String(entry?.windowId || '').trim() === preferredId);
      if (matched) {
        return matched;
      }
    }
    return liveOwnedWindows[liveOwnedWindows.length - 1] || null;
  }
  const storedWindowId = getScopedWorkspaceSelection(convID);
  if (storedWindowId) {
    const win = getWindowById(storedWindowId);
    if (win && String(win?.windowKey || '').trim() !== CHAT_WINDOW_KEY) {
      return win;
    }
  }
  const savedWindows = getScopedWorkspaceWindowsState(convID);
  for (const saved of savedWindows) {
    const expectedWindowId = String(saved.windowId || '').trim()
      || computeWorkspaceWindowId(saved.windowKey, saved.parameters || {});
    if (!expectedWindowId) continue;
    const win = getWindowById(expectedWindowId);
    if (!win) continue;
    if (String(win?.windowKey || '').trim() === CHAT_WINDOW_KEY) continue;
    return win;
  }
  return null;
}

function hydrateLiveWorkspaceWindowFromSavedState(conversationId = '', win = null) {
  const convID = String(conversationId || '').trim();
  const windowId = String(win?.windowId || '').trim();
  if (!convID || !windowId) return win;
  const savedWindows = getScopedWorkspaceWindowsState(convID);
  const saved = savedWindows.find((entry) => String(entry?.windowId || '').trim() === windowId);
  if (!saved) return win;
  if (saved.windowForm && typeof saved.windowForm === 'object') {
    const formSignal = getFormSignal(`${windowId}:windowForm`);
    const currentForm = formSignal?.peek?.() || {};
    if (JSON.stringify(currentForm) !== JSON.stringify(saved.windowForm)) {
      formSignal.value = saved.windowForm;
    }
  }
  if (saved.viewState && typeof saved.viewState === 'object') {
    const viewSignal = getViewSignal(windowId);
    const currentView = viewSignal?.peek?.() || {};
    if (JSON.stringify(currentView) !== JSON.stringify(saved.viewState)) {
      viewSignal.value = saved.viewState;
    }
  }
  return win;
}

function restoreWorkspaceWindowForConversation(conversationId = '', { focus = true } = {}) {
  const convID = String(conversationId || '').trim();
  if (!convID) return null;
  const live = resolveWorkspaceWindowForConversation(convID);
  if (live) {
    hydrateLiveWorkspaceWindowFromSavedState(convID, live);
    return focus ? focusWindow(live) : live;
  }
  const savedWindows = getScopedWorkspaceWindowsState(convID);
  if (savedWindows.length === 0) return null;
  const previousSelectedWindowId = String(selectedWindowId.peek?.() || selectedWindowId.value || '').trim();
  const previousSelectedTabId = String(selectedTabId.peek?.() || selectedTabId.value || '').trim();
  let restored = null;
  for (const saved of savedWindows) {
    const windowKey = String(saved.windowKey || '').trim();
    if (!windowKey || windowKey === CHAT_WINDOW_KEY) continue;
    restored = addWindow(
      String(saved.windowTitle || windowKey).trim() || windowKey,
      saved.parentKey || MAIN_CHAT_WINDOW_ID,
      windowKey,
      null,
      saved.inTab !== false,
      saved.parameters || {},
      {
        autoIndexTitle: false,
        windowId: String(saved.windowId || '').trim() || undefined,
        conversationId: convID,
        presentation: String(saved.presentation || '').trim() || undefined,
        region: String(saved.region || '').trim() || undefined,
        workspaceSharePct: saved.workspaceSharePct ?? undefined,
        workspaceMinHeight: saved.workspaceMinHeight ?? undefined,
        workspaceCollapsed: saved.workspaceCollapsed === true,
      }
    );
    if (restored?.windowId && saved?.windowForm && typeof saved.windowForm === 'object') {
      getFormSignal(`${restored.windowId}:windowForm`).value = saved.windowForm;
    }
  }
  if (!restored) return null;
  const restoredWindows = resolveWorkspaceWindowsForConversation(convID);
  const preferredWindowId = String(getScopedWorkspaceSelection(convID) || '').trim();
  const targetWindow = restoredWindows.find((entry) => String(entry?.windowId || '').trim() === preferredWindowId)
    || restoredWindows[restoredWindows.length - 1]
    || restored;
  if (!focus) {
    selectedWindowId.value = previousSelectedWindowId || null;
    selectedTabId.value = previousSelectedTabId || null;
  }
  void publishUIBridgeSnapshotNow();
  return focus ? focusWindow(targetWindow) : targetWindow;
}

export function reopenWorkspaceForConversation(conversationId = '') {
  return restoreWorkspaceWindowForConversation(conversationId, { focus: true });
}

export function ensureWorkspaceWindowForConversation(conversationId = '') {
  return restoreWorkspaceWindowForConversation(conversationId, { focus: false });
}

function isPlainObject(value) {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function normalizeOptionalFiniteNumber(value) {
  if (value == null) return null;
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : null;
}

function mergeWorkspaceSnapshotValue(rawValue, liveValue) {
  if (liveValue === undefined) return rawValue;
  if (rawValue === undefined) return liveValue;
  if (Array.isArray(liveValue) || Array.isArray(rawValue)) return liveValue;
  if (!isPlainObject(rawValue) || !isPlainObject(liveValue)) return liveValue;
  const merged = { ...rawValue };
  const keys = new Set([...Object.keys(rawValue), ...Object.keys(liveValue)]);
  keys.forEach((key) => {
    merged[key] = mergeWorkspaceSnapshotValue(rawValue[key], liveValue[key]);
  });
  return merged;
}

function normalizeWorkspaceStateSnapshot(raw = null, { preferLiveSignals = true } = {}) {
  if (!raw || typeof raw !== 'object') return null;
  const windowKey = String(raw.windowKey || '').trim();
  if (!windowKey || windowKey === CHAT_WINDOW_KEY) return null;
  const parameters = raw.parameters && typeof raw.parameters === 'object' ? raw.parameters : {};
  const computedWindowId = computeWorkspaceWindowId(windowKey, parameters);
  const windowId = String(raw.windowId || computedWindowId || '').trim();
  const liveWindowForm = preferLiveSignals && windowId ? (getFormSignal(`${windowId}:windowForm`)?.peek?.() || {}) : {};
  const liveViewState = preferLiveSignals && windowId ? (getViewSignal(windowId)?.peek?.() || {}) : {};
  const rawWindowForm = raw.windowForm && typeof raw.windowForm === 'object' ? raw.windowForm : undefined;
  const rawViewState = raw.viewState && typeof raw.viewState === 'object' ? raw.viewState : undefined;
  const resolvedWindowForm = Object.keys(liveWindowForm).length > 0
    ? mergeWorkspaceSnapshotValue(rawWindowForm, liveWindowForm)
    : rawWindowForm;
  const resolvedViewState = Object.keys(liveViewState).length > 0
    ? mergeWorkspaceSnapshotValue(rawViewState, liveViewState)
    : rawViewState;
  return {
    windowId,
    conversationId: String(raw.conversationId || '').trim() || null,
    windowKey,
    windowTitle: String(raw.windowTitle || raw.windowKey || '').trim() || windowKey,
    presentation: String(raw.presentation || '').trim() || null,
    region: String(raw.region || '').trim() || null,
    parentKey: String(raw.parentKey || MAIN_CHAT_WINDOW_ID).trim() || MAIN_CHAT_WINDOW_ID,
    inTab: raw.inTab !== false,
    workspaceSharePct: normalizeOptionalFiniteNumber(raw.workspaceSharePct),
    workspaceMinHeight: normalizeOptionalFiniteNumber(raw.workspaceMinHeight),
    workspaceCollapsed: raw.workspaceCollapsed === true,
    windowForm: resolvedWindowForm,
    viewState: resolvedViewState,
    parameters,
  };
}

export function deriveWorkspaceStateFromTranscriptTurns(turns = []) {
  const derived = deriveHostedWorkspaceRestoreStateFromTranscriptTurns(Array.isArray(turns) ? turns : []);
  const windows = (Array.isArray(derived?.windows) ? derived.windows : []).filter((entry) => (
    String(entry?.windowKey || '').trim() !== CHAT_WINDOW_KEY
  ));
  if (windows.length === 0) return null;
  const selectedCandidate = String(derived.selectedWindowId || '').trim();
  const selectedWindowId = windows.some((entry) => String(entry?.windowId || '').trim() === selectedCandidate)
    ? selectedCandidate
    : (String(windows[0]?.windowId || '').trim() || null);
  return {
    windows,
    selectedWindowId,
  };
}

export function syncScopedWorkspaceStateFromTranscriptTurns(
  conversationId = '',
  turns = [],
  {
    reopen = false,
    announce = true,
    allowRunning = false,
  } = {}
) {
  const convID = String(conversationId || '').trim();
  if (!convID) return null;
  if (!allowRunning && transcriptTurnsHaveRunningStatus(turns)) return null;
  const derived = deriveWorkspaceStateFromTranscriptTurns(turns);
  if (!derived?.windows?.length) return null;
  setScopedWorkspaceState(convID, derived.windows);
  setScopedWorkspaceSelection(convID, String(derived.selectedWindowId || '').trim());
  if (!announce || typeof window === 'undefined') {
    return derived;
  }
  if (reopen && typeof window.setTimeout === 'function') {
    window.setTimeout(() => {
      reopenWorkspaceForConversation(convID);
      dispatchWorkspaceStateEvent(convID);
    }, 0);
    nudgeWorkspaceRestore(convID, 4);
    return derived;
  }
  if (reopen) {
    reopenWorkspaceForConversation(convID);
    dispatchWorkspaceStateEvent(convID);
    return derived;
  }
  nudgeWorkspaceRestore(convID, 4);
  return derived;
}

export function ensureMainChatWindow() {
  const windows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  let existing = windows.find((entry) => String(entry?.windowId || '').trim() === MAIN_CHAT_WINDOW_ID);
  if (!existing) {
    existing = addWindow('Chat', null, CHAT_WINDOW_KEY, null, true, {}, { autoIndexTitle: false });
  }
  return focusWindow(existing);
}

function updateMainChatWindowParameters(conversationId = '') {
  const targetID = String(conversationId || '').trim();
  const windows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  let changed = false;
  const next = windows.map((entry) => {
    if (String(entry?.windowId || '').trim() !== MAIN_CHAT_WINDOW_ID) return entry;
    changed = true;
    const parameters = {
      ...(entry?.parameters || {}),
      conversations: {
        ...((entry?.parameters || {}).conversations || {}),
        form: {
          ...((((entry?.parameters || {}).conversations || {}).form) || {}),
          id: targetID
        }
      },
      messages: {
        ...((entry?.parameters || {}).messages || {}),
        input: {
          ...((((entry?.parameters || {}).messages || {}).input) || {}),
          parameters: {
            ...(((((entry?.parameters || {}).messages || {}).input || {}).parameters) || {}),
            convID: targetID
          }
        }
      }
    };
    return {
      ...entry,
      parameters
    };
  });
  if (changed) {
    activeWindows.value = next;
  }
}

export function publishConversationSelection(windowId = '', conversationId = '', { syncPath = false, eventType = 'forge:conversation-active' } = {}) {
  const scopedWindowId = String(windowId || '').trim();
  const id = String(conversationId || '').trim();
  setScopedConversationSelection(scopedWindowId, id);
  if (syncPath && isMainChatWindowId(scopedWindowId)) {
    syncMainConversationPath(id);
  }
  if (typeof window === 'undefined') return;
  try {
    window.dispatchEvent(new CustomEvent(eventType, { detail: { id, windowId: scopedWindowId } }));
  } catch (_) {}
}

export function openConversationInMainWindow(conversationId = '') {
  const targetID = String(conversationId || '').trim();
  removeNonChatTopLevelWindows();
  const mainWindow = ensureMainChatWindow();
  focusWindow(mainWindow);
  updateMainChatWindowParameters(targetID);
  publishConversationSelection(mainWindow?.windowId || MAIN_CHAT_WINDOW_ID, targetID, {
    syncPath: true,
    eventType: 'agently:conversation-select'
  });
  const workspaceWindow = reopenWorkspaceForConversation(targetID);
  if (workspaceWindow) {
    return workspaceWindow;
  }
  if (targetID && hasScopedWorkspaceState(targetID)) {
    nudgeWorkspaceRestore(targetID, 4);
  }
  return mainWindow;
}

export function requestNewConversationInMainWindow() {
  removeNonChatTopLevelWindows();
  const mainWindow = ensureMainChatWindow();
  updateMainChatWindowParameters('');
  publishConversationSelection(mainWindow?.windowId || MAIN_CHAT_WINDOW_ID, '', {
    syncPath: true,
    eventType: 'agently:conversation-new'
  });
  return mainWindow;
}

export function openLinkedConversationWindow(conversationId = '') {
  const targetID = String(conversationId || '').trim();
  if (!targetID) return null;
  const parentWindow = getSelectedWindow();
  const parentWindowId = String(parentWindow?.windowId || MAIN_CHAT_WINDOW_ID).trim() || MAIN_CHAT_WINDOW_ID;
  const parentConversationId = getScopedConversationSelection(parentWindowId);
  const windows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  let existing = windows.find((entry) => {
    if (String(entry?.windowKey || '').trim() !== CHAT_WINDOW_KEY) return false;
    const entryConversationID = String(entry?.parameters?.conversations?.form?.id || '').trim();
    return entryConversationID === targetID;
  });
  if (!existing) {
    existing = addWindow(
      'Linked Chat',
      null,
      CHAT_WINDOW_KEY,
      null,
      true,
      {
        conversations: {
          form: {
            id: targetID
          }
        },
        linkedParent: {
          windowId: parentWindowId,
          conversationId: parentConversationId
        }
      },
      { autoIndexTitle: false }
    );
  }
  focusWindow(existing);
  publishConversationSelection(existing?.windowId || '', targetID, {
    syncPath: false,
    eventType: 'agently:conversation-select'
  });
  return existing;
}

export function returnToParentConversation(win = null, { closeCurrent = false } = {}) {
  const target = win || getSelectedWindow();
  const parentConversationId = linkedParentConversationId(target);
  if (closeCurrent && target?.windowId && !isMainChatWindowId(target.windowId)) {
    removeWindow(target.windowId);
    openConversationInMainWindow(parentConversationId);
    return;
  }
  openConversationInMainWindow(parentConversationId);
  const parentWindow = getWindowById(linkedParentWindowId(target)) || getWindowById(MAIN_CHAT_WINDOW_ID);
  focusWindow(parentWindow);
}
