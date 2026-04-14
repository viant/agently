import { describe, expect, it } from 'vitest';

import {
  resolveMainWindowCloseConversationId,
  resolveMainWindowHeaderTitle,
  resolveSelectedMainWindow,
  shouldShowChatChrome,
  shouldShowMainWindowHeader
} from './Root.jsx';

describe('Root window selection helpers', () => {
  it('prefers the selected tabbed window over stale focused window state', () => {
    const windows = [
      { windowId: 'chat/new', windowKey: 'chat/new' },
      { windowId: 'schedule', windowKey: 'schedule' }
    ];

    const selected = resolveSelectedMainWindow(windows, 'schedule', 'chat/new', windows[0]);

    expect(selected).toEqual({ windowId: 'schedule', windowKey: 'schedule' });
  });

  it('falls back to the provided fallback window when no ids match', () => {
    const fallback = { windowId: 'chat/new', windowKey: 'chat/new' };
    const selected = resolveSelectedMainWindow([], '', '', fallback);
    expect(selected).toBe(fallback);
  });

  it('shows chat chrome only for chat windows', () => {
    expect(shouldShowChatChrome({ windowKey: 'chat/new' })).toBe(true);
    expect(shouldShowChatChrome({ windowKey: 'schedule' })).toBe(false);
    expect(shouldShowChatChrome(null)).toBe(false);
  });

  it('normalizes the conversation id restored when closing a non-chat main window', () => {
    expect(resolveMainWindowCloseConversationId(' conv-123 ')).toBe('conv-123');
    expect(resolveMainWindowCloseConversationId('')).toBe('');
    expect(resolveMainWindowCloseConversationId(null)).toBe('');
  });

  it('shows the main window header only for non-chat windows with a title', () => {
    expect(resolveMainWindowHeaderTitle({ windowTitle: 'Runs', windowKey: 'schedule/history' })).toBe('Runs');
    expect(resolveMainWindowHeaderTitle({ windowKey: 'schedule' })).toBe('schedule');
    expect(shouldShowMainWindowHeader({ windowTitle: 'Runs', windowKey: 'schedule/history' })).toBe(false);
    expect(shouldShowMainWindowHeader({ windowTitle: 'Automation', windowKey: 'schedule' })).toBe(false);
    expect(shouldShowMainWindowHeader({ windowTitle: 'Runs', windowKey: 'schedule/history', inTab: false })).toBe(false);
    expect(shouldShowMainWindowHeader({ windowTitle: 'Chat', windowKey: 'chat/new' })).toBe(false);
    expect(shouldShowMainWindowHeader({ windowTitle: '', windowKey: '' })).toBe(false);
  });
});
