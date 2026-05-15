import {
  addWindow,
  activeWindows,
  publishUIBridgeSnapshotNow,
  removeWindow,
  selectedTabId,
  selectedWindowId
} from 'forge/core';
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
  const scoped = getScopedConversationSelection(windowId);
  if (scoped) return scoped;
  if (typeof window === 'undefined') return '';
  if (isMainChatWindowId(windowId)) {
    return currentConversationIdFromPath(window.location?.pathname);
  }
  return '';
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
    const payload = {
      windowId: String(win.windowId || '').trim(),
      conversationId: String(win.conversationId || '').trim() || null,
      windowKey: String(win.windowKey || '').trim(),
      presentation: String(win.presentation || '').trim() || null,
      region: String(win.region || '').trim() || null,
      windowTitle: String(win.windowTitle || win.windowKey || '').trim(),
      parentKey: String(win.parentKey || '').trim() || null,
      inTab: win.inTab !== false,
      parameters: win.parameters || {},
    };
    storage.setItem(workspaceStateKey(convID), JSON.stringify(payload));
  } catch (_) {}
}

export function resolveWorkspaceWindowForConversation(conversationId = '') {
  const convID = String(conversationId || '').trim();
  if (!convID) return null;
  const windows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  const liveOwnedWindow = windows.find((entry) => {
    if (String(entry?.windowKey || '').trim() === CHAT_WINDOW_KEY) return false;
    if (String(entry?.presentation || '').trim().toLowerCase() !== 'hosted') return false;
    if (String(entry?.region || '').trim().toLowerCase() !== 'chat.top') return false;
    return String(entry?.conversationId || '').trim() === convID;
  });
  if (liveOwnedWindow) {
    return liveOwnedWindow;
  }
  const storedWindowId = getScopedWorkspaceSelection(convID);
  if (storedWindowId) {
    const win = getWindowById(storedWindowId);
    if (win && String(win?.windowKey || '').trim() !== CHAT_WINDOW_KEY) {
      return win;
    }
  }
  const saved = getScopedWorkspaceState(convID);
  if (!saved) return null;
  const expectedWindowId = String(saved.windowId || '').trim()
    || computeWorkspaceWindowId(saved.windowKey, saved.parameters || {});
  if (!expectedWindowId) return null;
  const win = getWindowById(expectedWindowId);
  if (!win) return null;
  if (String(win?.windowKey || '').trim() === CHAT_WINDOW_KEY) return null;
  return win;
}

function restoreWorkspaceWindowForConversation(conversationId = '', { focus = true } = {}) {
  const convID = String(conversationId || '').trim();
  if (!convID) return null;
  const live = resolveWorkspaceWindowForConversation(convID);
  if (live) return focus ? focusWindow(live) : live;
  const saved = getScopedWorkspaceState(convID);
  if (!saved) return null;
  const windowKey = String(saved.windowKey || '').trim();
  if (!windowKey || windowKey === CHAT_WINDOW_KEY) return null;
  const previousSelectedWindowId = String(selectedWindowId.peek?.() || selectedWindowId.value || '').trim();
  const previousSelectedTabId = String(selectedTabId.peek?.() || selectedTabId.value || '').trim();
  const win = addWindow(
    String(saved.windowTitle || windowKey).trim() || windowKey,
    saved.parentKey || MAIN_CHAT_WINDOW_ID,
    windowKey,
    null,
    saved.inTab !== false,
    saved.parameters || {},
    {
      autoIndexTitle: false,
      conversationId: convID,
      presentation: String(saved.presentation || '').trim() || undefined,
      region: String(saved.region || '').trim() || undefined
    }
  );
  if (!focus) {
    selectedWindowId.value = previousSelectedWindowId || null;
    selectedTabId.value = previousSelectedTabId || null;
  }
  void publishUIBridgeSnapshotNow();
  return focus ? focusWindow(win) : win;
}

export function reopenWorkspaceForConversation(conversationId = '') {
  return restoreWorkspaceWindowForConversation(conversationId, { focus: true });
}

export function ensureWorkspaceWindowForConversation(conversationId = '') {
  return restoreWorkspaceWindowForConversation(conversationId, { focus: false });
}

export function seedScopedWorkspaceState(conversationId = '', snapshot = null) {
  const convID = String(conversationId || '').trim();
  if (!convID || !snapshot || typeof snapshot !== 'object') return null;
  if (getScopedWorkspaceState(convID)) return getScopedWorkspaceState(convID);
  setScopedWorkspaceState(convID, snapshot);
  return getScopedWorkspaceState(convID);
}

function normalizeWorkspaceStateSnapshot(raw = null) {
  if (!raw || typeof raw !== 'object') return null;
  const windowKey = String(raw.windowKey || '').trim();
  if (!windowKey || windowKey === CHAT_WINDOW_KEY) return null;
  const parameters = raw.parameters && typeof raw.parameters === 'object' ? raw.parameters : {};
  const computedWindowId = computeWorkspaceWindowId(windowKey, parameters);
  const windowId = String(raw.windowId || computedWindowId || '').trim();
  return {
    windowId,
    conversationId: String(raw.conversationId || '').trim() || null,
    windowKey,
    windowTitle: String(raw.windowTitle || raw.windowKey || '').trim() || windowKey,
    presentation: String(raw.presentation || '').trim() || null,
    parentKey: String(raw.parentKey || MAIN_CHAT_WINDOW_ID).trim() || MAIN_CHAT_WINDOW_ID,
    inTab: raw.inTab !== false,
    parameters,
  };
}

export function deriveWorkspaceStateFromConversation(conversation = null) {
  if (!conversation || typeof conversation !== 'object') return null;
  const metadataRaw = conversation?.metadata ?? conversation?.Metadata;
  let metadata = metadataRaw;
  if (typeof metadataRaw === 'string') {
    try {
      metadata = JSON.parse(metadataRaw);
    } catch (_) {
      metadata = null;
    }
  }
  const workspace = metadata?.workspace;
  return normalizeWorkspaceStateSnapshot(workspace);
}

export function syncWorkspaceStateFromConversation(conversation = null) {
  const conversationId = String(conversation?.id || conversation?.Id || '').trim();
  if (!conversationId) return null;
  const derived = deriveWorkspaceStateFromConversation(conversation);
  if (!derived) return null;
  return seedScopedWorkspaceState(conversationId, derived);
}

function parseToolPayload(raw = null) {
  if (!raw) return null;
  if (typeof raw === 'string') {
    try {
      return JSON.parse(raw);
    } catch (_) {
      return null;
    }
  }
  if (typeof raw === 'object') {
    if (raw.inlineBody && typeof raw.inlineBody === 'string') {
      try {
        return JSON.parse(raw.inlineBody);
      } catch (_) {}
    }
    return raw;
  }
  return null;
}

function normalizeToolName(raw = '') {
  const value = String(raw || '').trim().toLowerCase();
  if (!value) return '';
  return value.replace(/:/g, '/');
}

export function deriveWorkspaceStateFromTranscriptTurns(turns = []) {
  const list = Array.isArray(turns) ? turns : [];
  for (let turnIndex = list.length - 1; turnIndex >= 0; turnIndex -= 1) {
    const turn = list[turnIndex] || {};
    const pages = Array.isArray(turn?.execution?.pages) ? turn.execution.pages : [];
    for (let pageIndex = pages.length - 1; pageIndex >= 0; pageIndex -= 1) {
      const page = pages[pageIndex] || {};
      const toolSteps = Array.isArray(page?.toolSteps) ? page.toolSteps : [];
      for (let stepIndex = toolSteps.length - 1; stepIndex >= 0; stepIndex -= 1) {
        const step = toolSteps[stepIndex] || {};
        if (normalizeToolName(step?.toolName) !== 'ui/view/open') continue;
        if (String(step?.status || '').trim().toLowerCase() !== 'completed') continue;
        const requestPayload = parseToolPayload(step?.requestPayload);
        const responsePayload = parseToolPayload(step?.responsePayload);
        const windowKey = String(requestPayload?.id || responsePayload?.windowKey || '').trim();
        if (windowKey !== 'order') continue;
        const parameters = requestPayload?.parameters;
        const adOrderId = parameters?.AdOrderId;
        if (!Array.isArray(adOrderId) || adOrderId.length === 0) continue;
        return normalizeWorkspaceStateSnapshot({
          windowId: String(responsePayload?.windowId || '').trim(),
          windowKey,
          windowTitle: String(responsePayload?.windowTitle || 'Order Summary').trim(),
          parentKey: MAIN_CHAT_WINDOW_ID,
          inTab: true,
          parameters,
        });
      }
    }
  }
  return null;
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
  const mainWindow = ensureMainChatWindow();
  updateMainChatWindowParameters(targetID);
  publishConversationSelection(mainWindow?.windowId || MAIN_CHAT_WINDOW_ID, targetID, {
    syncPath: true,
    eventType: 'agently:conversation-select'
  });
  const workspaceWindow = reopenWorkspaceForConversation(targetID);
  if (workspaceWindow) {
    return workspaceWindow;
  }
  return mainWindow;
}

export function requestNewConversationInMainWindow() {
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
  openConversationInMainWindow(parentConversationId);
  if (closeCurrent && target?.windowId && !isMainChatWindowId(target.windowId)) {
    removeWindow(target.windowId);
  } else {
    const parentWindow = getWindowById(linkedParentWindowId(target)) || getWindowById(MAIN_CHAT_WINDOW_ID);
    focusWindow(parentWindow);
  }
}
