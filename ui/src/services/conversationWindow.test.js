import { beforeEach, describe, expect, it } from 'vitest';

import { activeWindows, selectedTabId, selectedWindowId } from 'forge/core';

import {
  CHAT_WINDOW_KEY,
  MAIN_CHAT_WINDOW_ID,
  isLinkedChildWindow,
  openLinkedConversationWindow,
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
