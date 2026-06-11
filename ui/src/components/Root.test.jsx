import { describe, expect, it } from 'vitest';

import {
  isCompactShellViewport,
  isConversationOwnedWorkspaceWindow,
  isConversationHostedWorkspaceChild,
  isHostedWorkspaceChildOfMainChat,
  resolveActiveConversationId,
  resolveChatChromeWindow,
  resolveEffectiveWorkspaceCollapsed,
  resolveHostedWorkspaceTabLabel,
  resolveHostedWorkspaceTabs,
  resolveHostedBottomWindow,
  resolveRouteBootstrapAction,
  shouldReturnSelectionToMainChat,
  resolveWorkspaceVisibilitySelection,
  shouldShowChatChromeForLayout,
  shouldReplayRouteConversationBootstrap,
  shouldCaptureDesktopSidebarPreference,
  shouldPersistWorkspaceHeight,
  shouldRestoreDesktopSidebarPreference,
  resolveMainWindowCloseConversationId,
  resolveMainWindowHeaderTitle,
  resolveSelectedMainWindow,
  resolveWindowRefreshDataSources,
  shouldForceWorkspaceFull,
  shouldShowChatChrome,
  shouldShowMainWindowHeader
} from './Root.jsx';

describe('Root window selection helpers', () => {
  it('detects phone-sized shell viewports for overlay navigation', () => {
    expect(isCompactShellViewport(390)).toBe(true);
    expect(isCompactShellViewport(820)).toBe(true);
    expect(isCompactShellViewport(821)).toBe(false);
    expect(isCompactShellViewport(0)).toBe(false);
  });

  it('forces hosted workspace full-screen on compact shells', () => {
    expect(shouldForceWorkspaceFull({ isCompactShell: true, showWorkspacePane: true })).toBe(true);
    expect(shouldForceWorkspaceFull({ isCompactShell: true, showWorkspacePane: false })).toBe(false);
    expect(shouldForceWorkspaceFull({ isCompactShell: false, showWorkspacePane: true })).toBe(false);
  });

  it('keeps collapsed workspaces visible on compact shells', () => {
    expect(resolveEffectiveWorkspaceCollapsed({ isCompactShell: true, isWorkspaceCollapsed: true })).toBe(false);
    expect(resolveEffectiveWorkspaceCollapsed({ isCompactShell: false, isWorkspaceCollapsed: true })).toBe(true);
    expect(resolveEffectiveWorkspaceCollapsed({ isCompactShell: false, isWorkspaceCollapsed: false })).toBe(false);
  });

  it('captures and restores desktop sidebar preference only across compact-shell transitions', () => {
    expect(shouldCaptureDesktopSidebarPreference({ wasCompactShell: false, isCompactShell: true })).toBe(true);
    expect(shouldCaptureDesktopSidebarPreference({ wasCompactShell: true, isCompactShell: true })).toBe(false);
    expect(shouldRestoreDesktopSidebarPreference({ wasCompactShell: true, isCompactShell: false })).toBe(true);
    expect(shouldRestoreDesktopSidebarPreference({ wasCompactShell: false, isCompactShell: false })).toBe(false);
  });

  it('persists workspace height only when a workspace exists or a stored height already exists', () => {
    expect(shouldPersistWorkspaceHeight({ activeWorkspaceWindowId: 'workspace-1', hasStoredHeight: false })).toBe(true);
    expect(shouldPersistWorkspaceHeight({ activeWorkspaceWindowId: '', hasStoredHeight: true })).toBe(true);
    expect(shouldPersistWorkspaceHeight({ activeWorkspaceWindowId: '', hasStoredHeight: false })).toBe(false);
  });

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

  it('uses the hosted bottom window as the chat-chrome source in split shell mode', () => {
    expect(resolveChatChromeWindow({
      shouldRenderSplitShell: true,
      hostedBottomWindow: { windowKey: 'chat/new' },
      selectedWindow: { windowKey: 'metricReportBuilder' },
    })).toEqual({ windowKey: 'chat/new' });
    expect(resolveChatChromeWindow({
      shouldRenderSplitShell: false,
      hostedBottomWindow: { windowKey: 'chat/new' },
      selectedWindow: { windowKey: 'metricReportBuilder' },
    })).toEqual({ windowKey: 'metricReportBuilder' });
  });

  it('suppresses chat chrome when the workspace is fully expanded over the chat pane', () => {
    expect(shouldShowChatChromeForLayout({
      chatChromeWindow: { windowKey: 'chat/new' },
      effectiveWorkspaceFull: false,
    })).toBe(true);
    expect(shouldShowChatChromeForLayout({
      chatChromeWindow: { windowKey: 'chat/new' },
      effectiveWorkspaceFull: true,
    })).toBe(false);
  });

  it('resolves the active conversation id from the chat pane before falling back to the main conversation', () => {
    expect(resolveActiveConversationId({
      chatChromeWindowId: 'chat/new',
      mainConversationId: 'conv-main',
      scopedConversationId: 'conv-chat',
    })).toBe('conv-chat');
    expect(resolveActiveConversationId({
      chatChromeWindowId: 'workspace-1',
      mainConversationId: 'conv-main',
      scopedConversationId: '',
    })).toBe('conv-main');
  });

  it('returns the chat window selection when hiding an active workspace pane', () => {
    expect(resolveWorkspaceVisibilitySelection({
      nextVisible: false,
      activeWorkspaceWindowId: 'workspace-1',
      mainChatWindowId: 'chat/new',
    })).toBe('chat/new');
    expect(resolveWorkspaceVisibilitySelection({
      nextVisible: true,
      activeWorkspaceWindowId: 'workspace-1',
      mainChatWindowId: 'chat/new',
    })).toBe('');
    expect(resolveWorkspaceVisibilitySelection({
      nextVisible: false,
      activeWorkspaceWindowId: '',
      mainChatWindowId: 'chat/new',
    })).toBe('');
  });

  it('returns selection to the main chat only when a hidden workspace is still selected', () => {
    expect(shouldReturnSelectionToMainChat({
      showWorkspaceWindow: false,
      activeWorkspaceWindowId: 'workspace-1',
      selectedWindowId: 'workspace-1',
    })).toBe(true);
    expect(shouldReturnSelectionToMainChat({
      showWorkspaceWindow: false,
      activeWorkspaceWindowId: 'workspace-1',
      selectedWindowId: 'chat/new',
    })).toBe(false);
    expect(shouldReturnSelectionToMainChat({
      showWorkspaceWindow: true,
      activeWorkspaceWindowId: 'workspace-1',
      selectedWindowId: 'workspace-1',
    })).toBe(false);
  });

  it('resolves datasource refresh refs for windows that need first-open data', () => {
    expect(resolveWindowRefreshDataSources('schedule')).toEqual(['schedules']);
    expect(resolveWindowRefreshDataSources('schedule/history')).toEqual(['runs']);
    expect(resolveWindowRefreshDataSources('recommendationList')).toEqual(['recommendation_list']);
    expect(resolveWindowRefreshDataSources('recommendation')).toEqual(['recommendation']);
    expect(resolveWindowRefreshDataSources('recommendationReview')).toEqual(['recommendation']);
    expect(resolveWindowRefreshDataSources('order')).toEqual([]);
  });

  it('routes chat.bottom windows into the hosted lower region', () => {
    const bottom = { windowId: 'schedule', windowKey: 'schedule', presentation: 'hosted', region: 'chat.bottom', conversationId: 'conv-1' };
    const chat = { windowId: 'chat/new', windowKey: 'chat/new' };
    expect(resolveHostedBottomWindow(bottom, chat, [bottom], 'conv-1')).toBe(bottom);
    expect(resolveHostedBottomWindow(chat, chat, [], 'conv-1')).toBe(chat);
  });

  it('prefers the selected linked child chat window over the main chat window', () => {
    const linkedChild = {
      windowId: 'chat/new__child',
      windowKey: 'chat/new',
      parameters: {
        conversations: { form: { id: 'child-456' } },
        linkedParent: {
          windowId: 'chat/new',
          conversationId: 'parent-123'
        }
      }
    };
    const mainChat = { windowId: 'chat/new', windowKey: 'chat/new' };
    expect(resolveHostedBottomWindow(linkedChild, mainChat, [], 'parent-123')).toBe(linkedChild);
  });

  it('treats parented hosted chat.top windows as main-chat subwindows', () => {
    expect(isHostedWorkspaceChildOfMainChat({
      windowKey: 'order',
      presentation: 'hosted',
      region: 'chat.top',
      parentKey: 'chat/new',
      inTab: true
    })).toBe(true);
    expect(isHostedWorkspaceChildOfMainChat({
      windowKey: 'order',
      presentation: 'hosted',
      region: 'chat.top',
      parentKey: 'order_123',
      inTab: true
    })).toBe(false);
  });

  it('requires hosted workspace children to match the active conversation before treating them as active', () => {
    const win = {
      windowKey: 'order',
      presentation: 'hosted',
      region: 'chat.top',
      parentKey: 'chat/new',
      inTab: true,
      conversationId: 'conv-a'
    };
    expect(isConversationHostedWorkspaceChild(win, 'conv-a')).toBe(true);
    expect(isConversationHostedWorkspaceChild(win, 'conv-b')).toBe(false);
  });

  it('does not treat a mismatched hosted workspace as owned by the active conversation', () => {
    const win = {
      windowKey: 'metricReportBuilder',
      presentation: 'hosted',
      region: 'chat.top',
      parentKey: 'chat/new',
      inTab: true,
      conversationId: 'conv-a'
    };
    expect(isConversationOwnedWorkspaceWindow(win, 'conv-a')).toBe(true);
    expect(isConversationOwnedWorkspaceWindow(win, 'conv-b')).toBe(false);
    expect(isConversationOwnedWorkspaceWindow({ ...win, region: 'chat.bottom' }, 'conv-a')).toBe(false);
  });

  it('normalizes the conversation id restored when closing a non-chat main window', () => {
    expect(resolveMainWindowCloseConversationId(' conv-123 ')).toBe('conv-123');
    expect(resolveMainWindowCloseConversationId('')).toBe('');
    expect(resolveMainWindowCloseConversationId(null)).toBe('');
  });

  it('shows the main window header only for non-chat windows with a title', () => {
    expect(resolveMainWindowHeaderTitle({ windowTitle: 'Runs', windowKey: 'schedule/history' })).toBe('Runs');
    expect(resolveMainWindowHeaderTitle({ windowKey: 'schedule' })).toBe('schedule');
    expect(resolveMainWindowHeaderTitle({
      windowKey: 'orderPerformance',
      windowTitle: 'Delaware_SB_display_Bally'
    })).toBe('Delaware_SB_display_Bally');
    expect(resolveMainWindowHeaderTitle({
      windowKey: 'orderPerformance',
    })).toBe('orderPerformance');
    expect(resolveMainWindowHeaderTitle({
      windowKey: 'campaign',
      windowTitle: '6TJH_ThomasJHenryLaw_CTVPilot_2026'
    })).toBe('6TJH_ThomasJHenryLaw_CTVPilot_2026');
    expect(resolveMainWindowHeaderTitle({
      windowKey: 'campaignPerformance',
      windowTitle: 'Campaign 551665'
    })).toBe('Campaign 551665');
    expect(resolveMainWindowHeaderTitle({
      windowKey: 'line',
      windowTitle: 'English OLV'
    })).toBe('English OLV');
    expect(resolveMainWindowHeaderTitle({
      windowKey: 'line',
    })).toBe('line');
    expect(shouldShowMainWindowHeader({ windowTitle: 'Runs', windowKey: 'schedule/history' })).toBe(false);
    expect(shouldShowMainWindowHeader({ windowTitle: 'Automation', windowKey: 'schedule' })).toBe(false);
    expect(shouldShowMainWindowHeader({ windowTitle: 'Runs', windowKey: 'schedule/history', inTab: false })).toBe(false);
    expect(shouldShowMainWindowHeader({ windowTitle: 'Chat', windowKey: 'chat/new' })).toBe(false);
    expect(shouldShowMainWindowHeader({
      windowTitle: 'Linked Chat',
      windowId: 'chat/new__child',
      windowKey: 'chat/new',
      parameters: {
        linkedParent: {
          windowId: 'chat/new',
          conversationId: 'parent-123'
        }
      }
    })).toBe(true);
    expect(shouldShowMainWindowHeader({ windowTitle: '', windowKey: '' })).toBe(false);
  });

  it('uses the current window title for hosted workspace tabs', () => {
    expect(resolveHostedWorkspaceTabLabel({
      windowKey: 'order',
      parameters: { AdOrderId: [2656980] },
      windowTitle: 'Order 2656980'
    })).toBe('Order 2656980');

    expect(resolveHostedWorkspaceTabs([
      {
        windowId: 'order_1',
        windowKey: 'order',
        parameters: { AdOrderId: [2656980] },
        windowTitle: 'Order 2656980'
      },
      {
        windowId: 'order_2',
        windowKey: 'order',
        parameters: { AdOrderId: [2609393] },
        windowTitle: 'Order 2609393'
      }
    ], 'order_2')).toEqual([
      { windowId: 'order_1', label: 'Order 2656980', isActive: false },
      { windowId: 'order_2', label: 'Order 2609393', isActive: true }
    ]);
  });

  it('does not bootstrap route selection until auth is ready', () => {
    expect(resolveRouteBootstrapAction('/v1/conversation/conv-123', 'checking')).toEqual({
      type: 'none',
      conversationId: ''
    });
  });

  it('boots the main chat window to the route conversation when auth is ready', () => {
    expect(resolveRouteBootstrapAction('/v1/conversation/conv-123', 'ready')).toEqual({
      type: 'conversation',
      conversationId: 'conv-123'
    });
  });

  it('boots the main chat window from the /conversation route variant too', () => {
    expect(resolveRouteBootstrapAction('/conversation/conv-123', 'ready')).toEqual({
      type: 'conversation',
      conversationId: 'conv-123'
    });
  });

  it('requests a new conversation on the root path when auth is ready', () => {
    expect(resolveRouteBootstrapAction('/', 'ready')).toEqual({
      type: 'new',
      conversationId: ''
    });
  });

  it('replays conversation route bootstrap after auth when the page is still empty', () => {
    expect(shouldReplayRouteConversationBootstrap({
      pathname: '/conversation/conv-123',
      authState: 'ready',
      mainConversationId: 'conv-123',
      hasChatFeed: false,
      hasWorkspace: false,
    })).toBe(true);
  });

  it('skips route bootstrap replay once either chat feed or workspace is already mounted', () => {
    expect(shouldReplayRouteConversationBootstrap({
      pathname: '/conversation/conv-123',
      authState: 'ready',
      mainConversationId: 'conv-123',
      hasChatFeed: true,
      hasWorkspace: false,
    })).toBe(false);
    expect(shouldReplayRouteConversationBootstrap({
      pathname: '/conversation/conv-123',
      authState: 'ready',
      mainConversationId: 'conv-123',
      hasChatFeed: false,
      hasWorkspace: true,
    })).toBe(false);
  });

  it('does not replay route bootstrap when the selected conversation differs from the route', () => {
    expect(shouldReplayRouteConversationBootstrap({
      pathname: '/conversation/conv-123',
      authState: 'ready',
      mainConversationId: 'conv-999',
      hasChatFeed: false,
      hasWorkspace: false,
    })).toBe(false);
  });
});
