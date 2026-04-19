import { beforeEach, describe, expect, it, vi } from 'vitest';

import { activeWindows, selectedTabId, selectedWindowId } from 'forge/core';

import {
  CHAT_WINDOW_KEY,
  MAIN_CHAT_WINDOW_ID,
  isLinkedChildWindow,
  openConversationInMainWindow,
  openLinkedConversationWindow,
  requestNewConversationInMainWindow,
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
    window.localStorage.setItem('agently.selectedConversationId', 'parent-123');

    const linked = openLinkedConversationWindow('child-456');

    expect(linked).toBeTruthy();
    expect(linked?.windowKey).toBe(CHAT_WINDOW_KEY);
    expect(linked?.parameters?.conversations?.form?.id).toBe('child-456');
    expect(linked?.parameters?.linkedParent).toEqual({
      windowId: MAIN_CHAT_WINDOW_ID,
      conversationId: 'parent-123'
    });
    expect(String(window.localStorage.getItem(`agently.selectedConversationId:${linked.windowId}`))).toBe('child-456');
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
    expect(String(window.localStorage.getItem('agently.selectedConversationId'))).toBe('conv-123');
    expect(activeWindows.value[0]?.parameters?.conversations?.form?.id).toBe('conv-123');
    expect(activeWindows.value[0]?.parameters?.messages?.input?.parameters?.convID).toBe('conv-123');
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
    expect(String(window.localStorage.getItem('agently.selectedConversationId') || '')).toBe('');
    expect(String(activeWindows.value[0]?.parameters?.conversations?.form?.id || '')).toBe('');
    expect(String(activeWindows.value[0]?.parameters?.messages?.input?.parameters?.convID || '')).toBe('');
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
    window.localStorage.setItem('agently.selectedConversationId', 'parent-123');
    window.localStorage.setItem('agently.selectedConversationId:child-window', 'child-456');

    returnToParentConversation(activeWindows.value[1]);

    expect(selectedWindowId.value).toBe(MAIN_CHAT_WINDOW_ID);
    expect(selectedTabId.value).toBe(MAIN_CHAT_WINDOW_ID);
    expect(String(window.localStorage.getItem('agently.selectedConversationId'))).toBe('parent-123');
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
});
