import { beforeEach, describe, expect, it, vi } from 'vitest';

import { activeWindows, selectedTabId, selectedWindowId } from 'forge/core';

import {
  CHAT_WINDOW_KEY,
  ensureWorkspaceWindowForConversation,
  MAIN_CHAT_WINDOW_ID,
  deriveWorkspaceStateFromConversation,
  resolveConversationSelection,
  resolveWorkspaceWindowForConversation,
  getScopedWorkspaceSelection,
  getScopedWorkspaceState,
  isLinkedChildWindow,
  openConversationInMainWindow,
  openLinkedConversationWindow,
  publishConversationSelection,
  requestNewConversationInMainWindow,
  setScopedWorkspaceState,
  setScopedWorkspaceSelection,
  returnToParentConversation
} from './conversationWindow';

function createStorage() {
  const store = new Map();
  return {
    getItem(key) {
      return store.has(key) ? store.get(key) : null;
    },
    setItem(key, value) {
      store.set(String(key), String(value));
    },
    removeItem(key) {
      store.delete(String(key));
    },
    clear() {
      store.clear();
    }
  };
}

describe('conversationWindow', () => {
  beforeEach(() => {
    activeWindows.value = [];
    selectedTabId.value = null;
    selectedWindowId.value = null;
    global.window = {
      location: { pathname: '/', port: '5176', hostname: '127.0.0.1' },
      history: { state: null, replaceState: () => {} },
      localStorage: createStorage(),
      sessionStorage: createStorage(),
      dispatchEvent: () => {}
    };
    global.CustomEvent = class CustomEvent {
      constructor(type, init = {}) {
        this.type = type;
        this.detail = init.detail;
      }
    };
  });

  it('opens a linked child conversation window and scopes the selection to it', () => {
    activeWindows.value = [{
      windowId: MAIN_CHAT_WINDOW_ID,
      windowKey: CHAT_WINDOW_KEY,
      parameters: {}
    }];
    selectedTabId.value = MAIN_CHAT_WINDOW_ID;
    selectedWindowId.value = MAIN_CHAT_WINDOW_ID;
    window.sessionStorage.setItem('agently.selectedConversationId', 'parent-123');

    const linked = openLinkedConversationWindow('child-456');

    expect(linked).toBeTruthy();
    expect(linked?.windowKey).toBe(CHAT_WINDOW_KEY);
    expect(linked?.parameters?.conversations?.form?.id).toBe('child-456');
    expect(linked?.parameters?.linkedParent).toEqual({
      windowId: MAIN_CHAT_WINDOW_ID,
      conversationId: 'parent-123'
    });
    expect(String(window.sessionStorage.getItem(`agently.selectedConversationId:${linked.windowId}`))).toBe('child-456');
    expect(selectedWindowId.value).toBe(linked.windowId);
  });

  it('syncs the browser path when opening a conversation in the main window', () => {
    activeWindows.value = [{
      windowId: MAIN_CHAT_WINDOW_ID,
      windowKey: CHAT_WINDOW_KEY,
      parameters: {}
    }];
    selectedTabId.value = MAIN_CHAT_WINDOW_ID;
    selectedWindowId.value = MAIN_CHAT_WINDOW_ID;
    const replaceState = vi.fn();
    window.history.replaceState = replaceState;

    openConversationInMainWindow('conv-123');

    expect(replaceState).toHaveBeenCalledWith(null, '', '/conversation/conv-123');
    expect(String(window.sessionStorage.getItem('agently.selectedConversationId'))).toBe('conv-123');
    expect(activeWindows.value[0]?.parameters?.conversations?.form?.id).toBe('conv-123');
    expect(activeWindows.value[0]?.parameters?.messages?.input?.parameters?.convID).toBe('conv-123');
  });

  it('restores a mapped workspace window when reopening a conversation in the main window', () => {
    activeWindows.value = [
      {
        windowId: MAIN_CHAT_WINDOW_ID,
        windowKey: CHAT_WINDOW_KEY,
        parameters: {}
      },
      {
        windowId: 'orderPerformance_1',
        windowKey: 'orderPerformance',
        parentKey: MAIN_CHAT_WINDOW_ID,
        inTab: true,
        parameters: {}
      }
    ];
    selectedTabId.value = MAIN_CHAT_WINDOW_ID;
    selectedWindowId.value = MAIN_CHAT_WINDOW_ID;
    setScopedWorkspaceSelection('conv-123', 'orderPerformance_1');

    const selected = openConversationInMainWindow('conv-123');

    expect(getScopedWorkspaceSelection('conv-123')).toBe('orderPerformance_1');
    expect(selected?.windowId).toBe('orderPerformance_1');
    expect(selectedWindowId.value).toBe('orderPerformance_1');
    expect(selectedTabId.value).toBe('orderPerformance_1');
  });

  it('resolves a live hosted workspace window for the current conversation before consulting saved browser state', () => {
    activeWindows.value = [
      {
        windowId: MAIN_CHAT_WINDOW_ID,
        windowKey: CHAT_WINDOW_KEY,
        parameters: {}
      },
      {
        windowId: 'order_1527048368',
        windowKey: 'order',
        conversationId: 'conv-live',
        presentation: 'hosted',
        region: 'chat.top',
        parentKey: MAIN_CHAT_WINDOW_ID,
        inTab: true,
        parameters: { AdOrderId: [2656980] }
      }
    ];

    expect(resolveWorkspaceWindowForConversation('conv-live')?.windowId).toBe('order_1527048368');
  });

  it('reopens a stored workspace descriptor when the live workspace window no longer exists', () => {
    activeWindows.value = [{
      windowId: MAIN_CHAT_WINDOW_ID,
      windowKey: CHAT_WINDOW_KEY,
      parameters: {}
    }];
    selectedTabId.value = MAIN_CHAT_WINDOW_ID;
    selectedWindowId.value = MAIN_CHAT_WINDOW_ID;

    setScopedWorkspaceSelection('conv-456', 'missing-window');
    setScopedWorkspaceState('conv-456', {
      windowId: 'missing-window',
      windowKey: 'orderPerformance',
      windowTitle: 'Order Summary',
      parentKey: MAIN_CHAT_WINDOW_ID,
      inTab: true,
      parameters: {
        order_performance_profile: {
          parameters: {
            AdOrderId: [2637048]
          }
        }
      }
    });

    const selected = openConversationInMainWindow('conv-456');

    expect(getScopedWorkspaceState('conv-456')?.windowKey).toBe('orderPerformance');
    expect(selected?.windowKey).toBe('orderPerformance');
    expect(selectedWindowId.value).toBe(selected?.windowId);
    expect(selectedTabId.value).toBe(selected?.windowId);
  });

  it('restores a stored workspace descriptor without stealing focus when only the main chat window is live', () => {
    activeWindows.value = [{
      windowId: MAIN_CHAT_WINDOW_ID,
      windowKey: CHAT_WINDOW_KEY,
      parameters: {}
    }];
    selectedTabId.value = MAIN_CHAT_WINDOW_ID;
    selectedWindowId.value = MAIN_CHAT_WINDOW_ID;

    setScopedWorkspaceState('conv-789', {
      windowId: 'order_1',
      windowKey: 'order',
      windowTitle: 'Order Summary',
      parentKey: MAIN_CHAT_WINDOW_ID,
      inTab: true,
      parameters: {
        AdOrderId: [2609393]
      }
    });

    const restored = ensureWorkspaceWindowForConversation('conv-789');

    expect(restored?.windowKey).toBe('order');
    expect(activeWindows.value.some((entry) => entry.windowKey === 'order')).toBe(true);
    expect(selectedWindowId.value).toBe(MAIN_CHAT_WINDOW_ID);
    expect(selectedTabId.value).toBe(MAIN_CHAT_WINDOW_ID);
  });

  it('uses /conversation on localhost and 127.0.0.1 hosts', () => {
    activeWindows.value = [{
      windowId: MAIN_CHAT_WINDOW_ID,
      windowKey: CHAT_WINDOW_KEY,
      parameters: {}
    }];
    selectedTabId.value = MAIN_CHAT_WINDOW_ID;
    selectedWindowId.value = MAIN_CHAT_WINDOW_ID;

    const replaceState = vi.fn();
    window.history.replaceState = replaceState;
    window.location = { pathname: '/', port: '8686', hostname: 'localhost' };

    openConversationInMainWindow('conv-localhost');
    expect(replaceState).toHaveBeenLastCalledWith(null, '', '/conversation/conv-localhost');

    window.location = { pathname: '/', port: '8686', hostname: '127.0.0.1' };
    openConversationInMainWindow('conv-loopback');
    expect(replaceState).toHaveBeenLastCalledWith(null, '', '/conversation/conv-loopback');
  });

  it('syncs the browser path back to root for a new main-window conversation', () => {
    activeWindows.value = [{
      windowId: MAIN_CHAT_WINDOW_ID,
      windowKey: CHAT_WINDOW_KEY,
      parameters: {}
    }];
    selectedTabId.value = MAIN_CHAT_WINDOW_ID;
    selectedWindowId.value = MAIN_CHAT_WINDOW_ID;
    window.location.pathname = '/conversation/conv-123';
    const replaceState = vi.fn();
    window.history.replaceState = replaceState;

    requestNewConversationInMainWindow();

    expect(replaceState).toHaveBeenCalledWith(null, '', '/');
    expect(String(window.sessionStorage.getItem('agently.selectedConversationId') || '')).toBe('');
    expect(String(activeWindows.value[0]?.parameters?.conversations?.form?.id || '')).toBe('');
    expect(String(activeWindows.value[0]?.parameters?.messages?.input?.parameters?.convID || '')).toBe('');
  });

  it('publishes scoped selection without changing the browser path for non-main windows', () => {
    const replaceState = vi.fn();
    const dispatchEvent = vi.fn();
    window.history.replaceState = replaceState;
    window.dispatchEvent = dispatchEvent;

    publishConversationSelection('workspace-window', 'conv-xyz', {
      syncPath: false,
      eventType: 'forge:conversation-active'
    });

    expect(String(window.sessionStorage.getItem('agently.selectedConversationId:workspace-window'))).toBe('conv-xyz');
    expect(replaceState).not.toHaveBeenCalled();
    expect(dispatchEvent).toHaveBeenCalledTimes(1);
  });

  it('publishes scoped selection and syncs path for the main window only when requested', () => {
    const replaceState = vi.fn();
    const dispatchEvent = vi.fn();
    window.history.replaceState = replaceState;
    window.dispatchEvent = dispatchEvent;

    publishConversationSelection(MAIN_CHAT_WINDOW_ID, 'conv-main', {
      syncPath: true,
      eventType: 'forge:conversation-active'
    });

    expect(String(window.sessionStorage.getItem('agently.selectedConversationId'))).toBe('conv-main');
    expect(replaceState).toHaveBeenCalledWith(null, '', '/conversation/conv-main');
    expect(dispatchEvent).toHaveBeenCalledTimes(1);
  });

  it('falls back to the browser path for the main chat window when scoped selection is empty', () => {
    window.location.pathname = '/conversation/conv-from-path';

    expect(resolveConversationSelection(MAIN_CHAT_WINDOW_ID)).toBe('conv-from-path');
  });

  it('returns to the parent conversation and focuses the main chat window', () => {
    activeWindows.value = [
      {
        windowId: MAIN_CHAT_WINDOW_ID,
        windowKey: CHAT_WINDOW_KEY,
        parameters: {}
      },
      {
        windowId: 'child-window',
        windowKey: CHAT_WINDOW_KEY,
        parameters: {
          conversations: { form: { id: 'child-456' } },
          linkedParent: {
            windowId: MAIN_CHAT_WINDOW_ID,
            conversationId: 'parent-123'
          }
        }
      }
    ];
    selectedTabId.value = 'child-window';
    selectedWindowId.value = 'child-window';
    window.sessionStorage.setItem('agently.selectedConversationId', 'parent-123');
    window.sessionStorage.setItem('agently.selectedConversationId:child-window', 'child-456');

    returnToParentConversation(activeWindows.value[1]);

    expect(selectedWindowId.value).toBe(MAIN_CHAT_WINDOW_ID);
    expect(selectedTabId.value).toBe(MAIN_CHAT_WINDOW_ID);
    expect(String(window.sessionStorage.getItem('agently.selectedConversationId'))).toBe('parent-123');
  });

  it('does not treat a standalone chat window as a linked child without linkedParent metadata', () => {
    expect(isLinkedChildWindow({
      windowId: 'chat/new__123',
      windowKey: CHAT_WINDOW_KEY,
      parameters: {
        conversations: {
          input: {
            parameters: {
              id: 'conv-standalone'
            }
          }
        }
      }
    })).toBe(false);
  });

  it('derives order workspace state from conversation metadata', () => {
    expect(deriveWorkspaceStateFromConversation({
      id: 'conv-1',
      metadata: JSON.stringify({
        workspace: {
          windowId: 'order_1527048368',
          windowKey: 'order',
          windowTitle: 'Order Summary',
          parentKey: MAIN_CHAT_WINDOW_ID,
          inTab: true,
          parameters: {
            AdOrderId: [2656980],
          },
        },
      }),
    })).toEqual({
      windowId: 'order_1527048368',
      conversationId: null,
      windowKey: 'order',
      windowTitle: 'Order Summary',
      presentation: null,
      parentKey: MAIN_CHAT_WINDOW_ID,
      inTab: true,
      parameters: {
        AdOrderId: [2656980],
      },
    });
  });
});
