import {
  addWindow,
  activeWindows,
  removeWindow,
  selectedTabId,
  selectedWindowId
} from 'forge/core';

export const CHAT_WINDOW_KEY = 'chat/new';
export const MAIN_CHAT_WINDOW_ID = CHAT_WINDOW_KEY;
const LEGACY_SELECTED_CONVERSATION_KEY = 'agently.selectedConversationId';
const WORKSPACE_SELECTION_KEY = 'agently.selectedWorkspaceWindowId';
const WORKSPACE_STATE_KEY = 'agently.workspaceState';

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
  if (typeof window === 'undefined') return '';
  const scoped = String(window.localStorage?.getItem(scopedSelectionKey(windowId)) || '').trim();
  if (scoped) return scoped;
  if (isMainChatWindowId(windowId)) {
    return String(window.localStorage?.getItem(LEGACY_SELECTED_CONVERSATION_KEY) || '').trim();
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
  if (typeof window === 'undefined') return;
  const id = String(conversationId || '').trim();
  const targetWindowId = String(windowId || '').trim();
  try {
    if (id) {
      window.localStorage?.setItem(scopedSelectionKey(targetWindowId), id);
    } else {
      window.localStorage?.removeItem(scopedSelectionKey(targetWindowId));
    }
    if (isMainChatWindowId(targetWindowId)) {
      if (id) window.localStorage?.setItem(LEGACY_SELECTED_CONVERSATION_KEY, id);
      else window.localStorage?.removeItem(LEGACY_SELECTED_CONVERSATION_KEY);
    }
  } catch (_) {}
}

export function getScopedWorkspaceSelection(conversationId = '') {
  if (typeof window === 'undefined') return '';
  const id = String(conversationId || '').trim();
  if (!id) return '';
  return String(window.localStorage?.getItem(workspaceSelectionKey(id)) || '').trim();
}

export function setScopedWorkspaceSelection(conversationId = '', windowId = '') {
  if (typeof window === 'undefined') return;
  const convID = String(conversationId || '').trim();
  if (!convID) return;
  const targetWindowId = String(windowId || '').trim();
  try {
    if (targetWindowId) {
      window.localStorage?.setItem(workspaceSelectionKey(convID), targetWindowId);
    } else {
      window.localStorage?.removeItem(workspaceSelectionKey(convID));
    }
  } catch (_) {}
}

export function getScopedWorkspaceState(conversationId = '') {
  if (typeof window === 'undefined') return null;
  const id = String(conversationId || '').trim();
  if (!id) return null;
  try {
    const raw = String(window.localStorage?.getItem(workspaceStateKey(id)) || '').trim();
    if (!raw) return null;
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object') return null;
    return parsed;
  } catch (_) {
    return null;
  }
}

export function setScopedWorkspaceState(conversationId = '', win = null) {
  if (typeof window === 'undefined') return;
  const convID = String(conversationId || '').trim();
  if (!convID) return;
  try {
    if (!win || typeof win !== 'object') {
      window.localStorage?.removeItem(workspaceStateKey(convID));
      return;
    }
    const payload = {
      windowId: String(win.windowId || '').trim(),
      windowKey: String(win.windowKey || '').trim(),
      windowTitle: String(win.windowTitle || win.windowKey || '').trim(),
      parentKey: String(win.parentKey || '').trim() || null,
      inTab: win.inTab !== false,
      parameters: win.parameters || {},
    };
    window.localStorage?.setItem(workspaceStateKey(convID), JSON.stringify(payload));
  } catch (_) {}
}

export function resolveWorkspaceWindowForConversation(conversationId = '') {
  const convID = String(conversationId || '').trim();
  if (!convID) return null;
  const storedWindowId = getScopedWorkspaceSelection(convID);
  if (!storedWindowId) return null;
  const win = getWindowById(storedWindowId);
  if (!win) return null;
  if (String(win?.windowKey || '').trim() === CHAT_WINDOW_KEY) return null;
  return win;
}

export function reopenWorkspaceForConversation(conversationId = '') {
  const convID = String(conversationId || '').trim();
  if (!convID) return null;
  const live = resolveWorkspaceWindowForConversation(convID);
  if (live) return focusWindow(live);
  const saved = getScopedWorkspaceState(convID);
  if (!saved) return null;
  const windowKey = String(saved.windowKey || '').trim();
  if (!windowKey || windowKey === CHAT_WINDOW_KEY) return null;
  const win = addWindow(
    String(saved.windowTitle || windowKey).trim() || windowKey,
    saved.parentKey || MAIN_CHAT_WINDOW_ID,
    windowKey,
    null,
    saved.inTab !== false,
    saved.parameters || {},
    { autoIndexTitle: false }
  );
  return focusWindow(win);
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
