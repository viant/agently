// @vitest-environment jsdom
import './requestLifecycle.contract.test.jsx';
import { describe, expect, it, vi } from 'vitest';
import { activeWindows, selectedWindowId, selectedTabId } from 'forge/core';
import { MAIN_CHAT_WINDOW_ID, CHAT_WINDOW_KEY, openConversationInMainWindow, setScopedActiveSurface, setScopedWorkspaceSelection, setScopedWorkspaceState } from '../services/conversationWindow.js';

import {
  resolveInitialAuthState,
  scheduleMissingWorkspaceRestore,
  isCompactShellViewport,
  isConversationOwnedWorkspaceWindow,
  isConversationHostedWorkspaceChild,
  isHostedWorkspaceChildOfMainChat,
  resolveActiveConversationId,
  resolveConversationWorkspaceAttachmentWindows,
  resolveChatChromeWindow,
  resolveEffectiveWorkspaceCollapsed,
  resolveHostedWorkspaceTabLabel,
  resolveHostedWorkspaceTabs,
  resolveHostedBottomWindow,
  resolveShellWorkspaceWindows,
  hasRenderedChatContent,
  resolveSplitChatClassName,
  resolveRouteBootstrapAction,
  shouldReturnSelectionToMainChat,
  shouldReturnCollapsedWorkspaceSelectionToMainChat,
  resolveWorkspaceVisibilitySelection,
  shouldShowChatChromeForLayout,
  shouldReplayRouteConversationBootstrap,
  shouldCaptureDesktopSidebarPreference,
  shouldPersistWorkspaceHeight,
  shouldPromoteFreshWorkspaceSurface,
  resolveAcknowledgedWorkspaceWindow,
  shouldRestoreDesktopSidebarPreference,
  resolveMainWindowCloseConversationId,
  resolveMainWindowHeaderTitle,
  resolveSelectedMainWindow,
  resolveWindowRefreshDataSources,
  scrollConversationFeedToEnd,
  shouldForceWorkspaceFull,
  shouldShowChatChrome,
  shouldShowMainWindowHeader,
  shouldRestoreConversationForActivity,
  shouldScrollConversationAfterTurn,
  shouldUseConversationWorkspaceFallback
} from './Root.jsx';

describe('scheduled missing workspace restoration', () => {
  it.each(['conversation', 'route'])('respects a later %s selection before delayed restoration', (selection) => {
    vi.useFakeTimers();
    const pathname = window.location.pathname;
    const restore = vi.fn();
    let stop;
    try {
      window.history.replaceState({}, '', '/conversation/restore-race');
      setScopedWorkspaceState('restore-race', {windowId: 'saved', windowKey: 'record', conversationId: 'restore-race'});
      setScopedActiveSurface('restore-race', 'workspace');
      stop = scheduleMissingWorkspaceRestore('restore-race', restore);
      vi.advanceTimersByTime(200);
      if (selection === 'conversation') setScopedActiveSurface('restore-race', 'conversation');
      else window.history.replaceState({}, '', '/conversation/another-chat');
      vi.advanceTimersByTime(3000);
      expect(restore).not.toHaveBeenCalled();
    } finally {
      stop?.();
      window.history.replaceState({}, '', pathname);
      vi.useRealTimers();
    }
  });

  it('restores the current workspace and cancels pending attempts on disposal', () => {
    vi.useFakeTimers();
    const pathname = window.location.pathname;
    const restore = vi.fn();
    let stop;
    try {
      window.history.replaceState({}, '', '/conversation/restore-current');
      setScopedWorkspaceState('restore-current', {windowId: 'saved', windowKey: 'record', conversationId: 'restore-current'});
      setScopedActiveSurface('restore-current', 'workspace');
      stop = scheduleMissingWorkspaceRestore('restore-current', restore);
      vi.advanceTimersByTime(250);
      expect(restore).toHaveBeenCalledWith('restore-current');
      stop();
      vi.advanceTimersByTime(3000);
      expect(restore).toHaveBeenCalledTimes(1);
    } finally {
      stop?.();
      window.history.replaceState({}, '', pathname);
      vi.useRealTimers();
    }
  });
});

describe('section navigation conversation ownership', () => {
  it('uses the approved phone breakpoint only in the opt-in section shell', () => {
    expect(isCompactShellViewport(760, true)).toBe(true);
    expect(isCompactShellViewport(761, true)).toBe(false);
    expect(isCompactShellViewport(820)).toBe(true);
  });
  it('retains the main conversation while an independent app destination is selected', () => {
    const chat = {windowId: MAIN_CHAT_WINDOW_ID, windowKey: CHAT_WINDOW_KEY, conversationId: 'chat-a'};
    const destination = {windowId: 'data-view', windowKey: 'accounts', region: 'app.section'};
    expect(resolveSelectedMainWindow([chat, destination], destination.windowId, destination.windowId, destination)).toBe(chat);
    expect(resolveSelectedMainWindow([chat], '', '', destination)).toBe(chat);
    expect(resolveSelectedMainWindow([destination], destination.windowId, '', destination)).toBeNull();
  });
});

describe('initial authentication discovery', () => {
  const providers = [{name: 'oauth', type: 'bff'}];
  it('allows navigation discovery to recover after a transient auth probe failure', () => {
    expect(resolveInitialAuthState(providers, null, {status: 503})).toBe('ready');
    expect(resolveInitialAuthState(providers, null, new TypeError('connection unavailable'))).toBe('ready');
  });
  it('waits for sign-in after confirmed authentication denial', () => {
    expect(resolveInitialAuthState(providers, null, {status: 401})).toBe('required');
    expect(resolveInitialAuthState(providers, null, {status: 403})).toBe('required');
    expect(resolveInitialAuthState(providers, null)).toBe('required');
    expect(resolveInitialAuthState(providers, {subject: 'signed-in'})).toBe('ready');
  });
});

describe('landing-page workspace ownership', () => {
  it('renders menu windows without a conversation and isolates conversation-owned windows', () => {
    const windows = [
      ...['advertisers', 'campaigns', 'spo'].map((windowKey) => ({windowId: windowKey, windowKey,
        parentKey: 'chat/new', presentation: 'hosted', region: 'chat.top'})),
      {windowId: 'private', conversationId: 'conv-1', parentKey: 'chat/new', presentation: 'hosted', region: 'chat.top'},
      {windowId: 'bottom', presentation: 'hosted', region: 'chat.bottom'},
    ];
    expect(resolveShellWorkspaceWindows(windows).map((entry) => entry.windowId)).toEqual(['advertisers', 'campaigns', 'spo']);
    expect(resolveShellWorkspaceWindows(windows, 'conv-1').map((entry) => entry.windowId)).toEqual(['private']);
    expect(isConversationHostedWorkspaceChild(windows[0], '')).toBe(false);
    expect(isConversationHostedWorkspaceChild(windows[0], 'conv-1')).toBe(false);
    expect(resolveHostedBottomWindow(null,null,[windows[4]],'')).toBeNull();
  });
});

describe('Root window selection helpers', () => {
  it('shows transcript-restored workspace references when the original open did not acknowledge', () => {
    const restored = {
      windowId: 'advertiserList__conv-1',
      windowKey: 'advertiserList',
      conversationId: 'conv-1',
      workspaceObject: {
        objectId: 'workspace:advertiserList__conv-1',
        origin: { turnId: 'turn-open' },
        lifecycle: { state: 'ready' },
      },
    };
    expect(resolveConversationWorkspaceAttachmentWindows([], [restored], [])).toEqual([restored]);
    const live = { ...restored, windowTitle: 'Advertisers', windowForm: { advertiserListMode: 'starred' } };
    expect(resolveConversationWorkspaceAttachmentWindows([], [restored], [live])).toEqual([live]);
  });

  it('scrolls only for a terminal event on the focused conversation surface', () => {
    expect(shouldScrollConversationAfterTurn({
      eventConversationId: 'conv-1', activeConversationId: 'conv-1',
      activeSurface: 'conversation', eventType: 'turn_completed'
    })).toBe(true);
    expect(shouldScrollConversationAfterTurn({
      eventConversationId: 'conv-1', activeConversationId: 'conv-1',
      activeSurface: 'workspace', eventType: 'turn_completed'
    })).toBe(false);
    expect(shouldScrollConversationAfterTurn({
      eventConversationId: 'conv-other', activeConversationId: 'conv-1',
      activeSurface: 'conversation', eventType: 'turn_completed'
    })).toBe(false);
    expect(shouldScrollConversationAfterTurn({
      eventConversationId: 'conv-1', activeConversationId: 'conv-1',
      activeSurface: 'conversation', eventType: 'tool_call_completed'
    })).toBe(false);
  });

  it('scrolls the chat feed stage to its final post-layout height', () => {
    const scrollTo = vi.fn();
    const root = { querySelector: vi.fn(() => ({ scrollHeight: 840, scrollTo })) };
    expect(scrollConversationFeedToEnd(root)).toBe(true);
    expect(scrollTo).toHaveBeenCalledWith({ top: 840, behavior: 'smooth' });
  });

  it('does not treat an empty mounted chat feed as restored conversation content', () => {
    expect(hasRenderedChatContent({ querySelector: () => ({ childElementCount: 0 }) })).toBe(false);
    expect(hasRenderedChatContent({ querySelector: () => ({ childElementCount: 1 }) })).toBe(true);
  });

  it('uses composer-only chat layout only while a workspace is visible', () => {
    expect(resolveSplitChatClassName()).toBe('app-window-split-chat');
    expect(resolveSplitChatClassName({ showWorkspacePane: false, composerExpanded: true }))
      .toBe('app-window-split-chat');
    expect(resolveSplitChatClassName({ showWorkspacePane: true, activeSurface: 'conversation' }))
      .toBe('app-window-split-chat');
    expect(resolveSplitChatClassName({ showWorkspacePane: true, activeSurface: 'workspace' }))
      .toBe('app-window-split-chat is-composer-only');
    expect(resolveSplitChatClassName({
      showWorkspacePane: true,
      activeSurface: 'workspace',
      composerExpanded: true,
    })).toBe('app-window-split-chat is-composer-only is-composer-expanded');
  });

  it('keeps workspace active for submitted, queued, and started turns', () => {
    expect(shouldRestoreConversationForActivity({
      eventConversationId: 'conv-1',
      activeConversationId: 'conv-1',
      activeSurface: 'workspace',
      eventType: 'turn_started',
    })).toBe(false);
    expect(shouldRestoreConversationForActivity({
      eventConversationId: 'conv-1',
      activeConversationId: 'conv-1',
      activeSurface: 'workspace',
      eventType: 'turn_queued',
    })).toBe(false);
    expect(shouldRestoreConversationForActivity({
      eventConversationId: 'conv-1',
      activeConversationId: 'conv-1',
      activeSurface: 'conversation',
      workspaceFull: true,
      eventType: 'turn_started',
    })).toBe(false);
    expect(shouldRestoreConversationForActivity({
      eventConversationId: 'conv-other',
      activeConversationId: 'conv-1',
      activeSurface: 'workspace',
      eventType: 'turn_started',
    })).toBe(false);
    expect(shouldRestoreConversationForActivity({
      eventConversationId: 'conv-1',
      activeConversationId: 'conv-1',
      activeSurface: 'conversation',
      eventType: 'turn_started',
    })).toBe(false);
    expect(shouldRestoreConversationForActivity({
      eventConversationId: 'conv-1',
      activeConversationId: 'conv-1',
      activeSurface: 'workspace',
      eventType: 'tool_call_completed',
    })).toBe(false);
  });
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

  it('keeps the previous workspace visible until the successor is ready and confirmed', () => {
    const previous = {windowId: 'old', conversationId: 'c', hostOpenState: 'historical_replay'};
    const candidate = {windowId: 'new', conversationId: 'c', hostOpenState: 'fresh', workspaceObject: {origin: {turnId: 'new-turn'}, lifecycle: {state: 'opening'}}};
    const input = {candidate, windows: [previous, candidate], conversationId: 'c', previousWindowId: 'old'};
    expect(resolveAcknowledgedWorkspaceWindow(input)).toBe(previous);
    candidate.workspaceObject.lifecycle.state = 'ready';
    expect(resolveAcknowledgedWorkspaceWindow(input)).toBe(previous);
    expect(resolveAcknowledgedWorkspaceWindow({...input, rows: [{kind: 'assistant', turnId: 'new-turn', content: 'Opened.'}]})).toBe(candidate);
  });

  it('activates an acknowledged navigation while its content is still opening', () => {
    const input = { activeSurface: 'conversation', mainConversationId: 'conv-1', selectedWindowId: 'resource-1',
      conversationRows: [{kind: 'assistant', turnId: 'turn-1', content: 'The workspace is open.', status: 'completed'}],
      activeWorkspaceWindow: { windowId: 'resource-1', conversationId: 'conv-1', hostOpenState: 'fresh',
        workspaceObject: { origin: { turnId: 'turn-1' }, lifecycle: { state: 'ready' } } } };
    expect(shouldPromoteFreshWorkspaceSurface(input)).toBe(true);
    expect(shouldPromoteFreshWorkspaceSurface({...input,turnRunning:true})).toBe(false);
    expect(shouldPromoteFreshWorkspaceSurface({...input, activeWorkspaceWindow: {...input.activeWorkspaceWindow,
      workspaceObject: {...input.activeWorkspaceWindow.workspaceObject, lastActivatedBy: {turnId: 'latest-turn'}}}})).toBe(false);
    expect(shouldPromoteFreshWorkspaceSurface({...input, conversationRows: []})).toBe(false);
    expect(shouldPromoteFreshWorkspaceSurface({...input, conversationRows: [{...input.conversationRows[0], status: 'streaming'}]})).toBe(false);
    expect(shouldPromoteFreshWorkspaceSurface({ ...input, mainConversationId: 'other' })).toBe(false);
    expect(shouldPromoteFreshWorkspaceSurface({ ...input, activeWorkspaceWindow: { ...input.activeWorkspaceWindow, hostOpenState: 'historical_replay' } })).toBe(false);
    expect(shouldPromoteFreshWorkspaceSurface({ ...input, activeWorkspaceWindow: { ...input.activeWorkspaceWindow,
      workspaceObject: { origin: { turnId: 'turn-1' }, lifecycle: { state: 'opening' } } } })).toBe(true);
    for (const state of ['failed', 'closed']) {
      expect(shouldPromoteFreshWorkspaceSurface({ ...input, activeWorkspaceWindow: { ...input.activeWorkspaceWindow,
        workspaceObject: { origin: { turnId: 'turn-1' }, lifecycle: { state } } } })).toBe(false);
    }
  });

  it('does not let a freshly opened hosted workspace overtake the active conversation', () => {
    const freshAdvertiser = {
      windowId: 'advertiser_85141',
      windowKey: 'advertiser',
      conversationId: 'conv-advertiser',
      presentation: 'hosted',
      region: 'chat.top',
      hostOpenState: 'fresh',
    };
    expect(shouldPromoteFreshWorkspaceSurface({
      activeSurface: 'conversation',
      mainConversationId: 'conv-advertiser',
      activeWorkspaceWindow: freshAdvertiser,
      selectedWindowId: 'advertiser_85141',
    })).toBe(false);
    expect(shouldPromoteFreshWorkspaceSurface({
      activeSurface: 'conversation',
      mainConversationId: 'conv-advertiser',
      activeWorkspaceWindow: { ...freshAdvertiser, hostOpenState: 'historical_replay' },
      selectedWindowId: 'advertiser_85141',
    })).toBe(false);
    expect(shouldPromoteFreshWorkspaceSurface({
      activeSurface: 'conversation',
      mainConversationId: 'conv-advertiser',
      activeWorkspaceWindow: freshAdvertiser,
      selectedWindowId: 'chat/new',
    })).toBe(false);
  });

  it('prefers the selected tabbed window over stale focused window state', () => {
    const windows = [
      { windowId: 'chat/new', windowKey: 'chat/new' },
      { windowId: 'schedule', windowKey: 'schedule' }
    ];

    const selected = resolveSelectedMainWindow(windows, 'schedule', 'chat/new', windows[0]);

    expect(selected).toEqual({ windowId: 'schedule', windowKey: 'schedule' });
  });

  it('does not let a conversation workspace replace a selected standalone window', () => {
    expect(shouldUseConversationWorkspaceFallback({ windowId: 'schedule', windowKey: 'schedule' })).toBe(false);
    expect(shouldUseConversationWorkspaceFallback({ windowId: 'schedule/history', windowKey: 'schedule/history' })).toBe(false);
    expect(shouldUseConversationWorkspaceFallback({ windowId: 'chat/new', windowKey: 'chat/new' })).toBe(true);
    expect(shouldUseConversationWorkspaceFallback(null)).toBe(true);
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

  it('returns selection to the main chat when the active workspace is collapsed while still selected', () => {
    expect(shouldReturnCollapsedWorkspaceSelectionToMainChat({
      effectiveWorkspaceCollapsed: true,
      activeWorkspaceWindowId: 'workspace-1',
      selectedWindowId: 'workspace-1',
    })).toBe(true);
    expect(shouldReturnCollapsedWorkspaceSelectionToMainChat({
      effectiveWorkspaceCollapsed: false,
      activeWorkspaceWindowId: 'workspace-1',
      selectedWindowId: 'workspace-1',
    })).toBe(false);
    expect(shouldReturnCollapsedWorkspaceSelectionToMainChat({
      effectiveWorkspaceCollapsed: true,
      activeWorkspaceWindowId: 'workspace-1',
      selectedWindowId: 'chat/new',
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

  it('uses authored labels instead of resource URIs for hosted tabs', () => {
    for (const title of ['window://workspace/creative', 'Window: Workspace Creative', '']) {
      expect(resolveHostedWorkspaceTabLabel({windowKey:'window://workspace/creative',windowTitle:title,navigation:{chipName:'Creative'}})).toBe('Creative');
    }
    expect(resolveHostedWorkspaceTabLabel({windowKey:'window://workspace/siteList',windowTitle:'window://workspace/siteList'})).toBe('Site List');
    expect(resolveHostedWorkspaceTabLabel({windowKey:'opaque-report-key',navigation:{label:'Example report'}})).toBe('Example report');
    expect(resolveHostedWorkspaceTabLabel({windowKey:'opaque-report-key',windowTitle:'A saved report name',navigation:{label:'Example report'}})).toBe('A saved report name');
    expect(resolveHostedWorkspaceTabLabel({windowKey:'window://workspace/creative',windowTitle:'Summer Campaign Video',navigation:{chipName:'Creative'}})).toBe('Summer Campaign Video');
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

it('selects the restored conversation workspace for Root without adopting the standalone shell workspace', () => {
  const previousWindow = globalThis.window;
  const previousWindows = activeWindows.peek();
  const previousSelected = selectedWindowId.peek();
  const previousTab = selectedTabId.peek();
  const values = new Map();
  const storage = { getItem: key => values.get(key) || null, setItem: (key, value) => values.set(key, String(value)), removeItem: key => values.delete(key) };
  globalThis.window = { sessionStorage: storage, localStorage: storage, location: { pathname: '/' }, history: { replaceState() {} }, dispatchEvent() {} };
  try {
    const standalone = { windowId: 'shell-records', windowKey: 'records', parentKey: MAIN_CHAT_WINDOW_ID, presentation: 'hosted', region: 'chat.top', inTab: true };
    const saved = { ...standalone, windowId: 'history-detail', windowKey: 'recordDetail', conversationId: 'selected-history', parameters: { RecordId: [17] } };
    activeWindows.value = [{ windowId: MAIN_CHAT_WINDOW_ID, windowKey: CHAT_WINDOW_KEY, parameters: {} }, standalone];
    selectedWindowId.value = selectedTabId.value = standalone.windowId;
    setScopedWorkspaceState('selected-history', saved);
    setScopedWorkspaceSelection('selected-history', saved.windowId);
    setScopedActiveSurface('selected-history', 'workspace');

    openConversationInMainWindow('selected-history');

    const windows = activeWindows.peek();
    expect(resolveShellWorkspaceWindows(windows).map(entry => entry.windowId)).toEqual([standalone.windowId]);
    expect(resolveShellWorkspaceWindows(windows, 'selected-history').map(entry => entry.windowId)).toEqual([saved.windowId]);
    expect(resolveSelectedMainWindow(windows, selectedTabId.peek(), selectedWindowId.peek())?.windowId).toBe(saved.windowId);
    expect(standalone.conversationId).toBeUndefined();
  } finally {
    globalThis.window = previousWindow;
    activeWindows.value = previousWindows;
    selectedWindowId.value = previousSelected;
    selectedTabId.value = previousTab;
  }
});


it('loads actual recent conversations, preserves ordering/links, and handles empty/error states', async () => {
  globalThis.IS_REACT_ACT_ENVIRONMENT = true;
  const React = await import('react');
  const {createRoot} = await import('react-dom/client');
  const {default: HomeRecentConversations} = await import('./HomeRecentConversations.jsx');
  const {client} = await import('../services/agentlyClient');
  const list = vi.spyOn(client, 'listConversations').mockResolvedValue({data:[{id:'older',title:'Earlier work',createdAt:'2026-10-20T21:15:00Z'},{id:'latest',title:'Recent work',createdAt:'2026-10-21T16:42:00Z'}]});
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host);
  try {
    await React.act(async () => root.render(React.createElement(HomeRecentConversations)));
    expect(list).toHaveBeenCalledWith({excludeScheduled:true,page:{limit:3,direction:'latest'}});
    expect([...host.querySelectorAll('a')].map(n=>n.getAttribute('href'))).toEqual(['/conversation/latest','/conversation/older']);
    expect(host.textContent).toContain('Pick up where you left off');
    list.mockResolvedValue({data:[]});
    await React.act(async () => window.dispatchEvent(new CustomEvent('agently:conversation-meta-updated')));
    expect(host.textContent).toContain('Your work stays in Conversations');
    list.mockRejectedValue(new Error('Unavailable'));
    await React.act(async () => window.dispatchEvent(new CustomEvent('agently:conversation-meta-updated')));
    expect(host.textContent).toContain('Recent conversations couldn’t load.');
    list.mockResolvedValue({data:[{id:'retry',title:'Recovered'}]});
    await React.act(async () => host.querySelector('button').click());
    expect(host.querySelector('a').textContent).toContain('Recovered');
  } finally {await React.act(async () => root.unmount());host.remove();list.mockRestore();}
});

describe('canonical report workspace handoff', () => {
  const report = (sequence = 1, extra = {}) => ({id:'review',scope:'campaign',grammar:'dashboard-v1',status:'committed',sequence,
    source:{title:'Delivery review',blocks:[{id:'summary',kind:'dashboard.markdown',content:'Existing report'}]},dataSources:{},...extra});
  const row = (assembly = report(), extra = {}) => ({kind:'assistant',renderKey:'answer',messageId:'message-1',turnId:'turn-1',renderedContent:{reports:[assembly],diagnostics:[]},...extra});
  it('projects transient pending cards only for the chat and keeps their identity on commit', async () => {
    const {projectedWorkspaceEntries} = await import('../services/workspaceAttachment.js');
    const {resolveSavedOutputs} = await import('./SavedOutputInventory.jsx');
    const pending=row(report(1,{status:'rendering',source:{}}));
    expect(projectedWorkspaceEntries([pending],'conversation',{includeInlineReports:true})).toEqual([]);
    const cards=projectedWorkspaceEntries([pending],'conversation',{includeInlineReports:true,includePendingReports:true});
    expect(cards).toHaveLength(1);expect(cards[0].workspaceObject.lifecycle.state).toBe('opening');
    expect(cards[0].workspaceObject.capabilities.reopen).toBe(false);
    expect(resolveSavedOutputs(cards,'conversation')).toEqual([]);
    const ready=projectedWorkspaceEntries([pending,row(report(2))],'conversation',{includeInlineReports:true,includePendingReports:true});
    expect(ready).toHaveLength(1);expect(ready[0].windowId).toBe(cards[0].windowId);
    expect(ready[0].workspaceObject.capabilities.reopen).toBe(true);
    expect(ready[0].workspaceObject.content).not.toHaveProperty('source');
  });
  it('creates one output for revisions, different outputs for different turns, and no output for ordinary or foreign answers', async () => {
    const {projectedWorkspaceEntries} = await import('../services/workspaceAttachment.js');
    const entries = rows => projectedWorkspaceEntries(rows, 'conversation', {includeInlineReports:true});
    expect(projectedWorkspaceEntries([row()], 'conversation')).toEqual([]);
    expect(entries([{kind:'assistant',content:'Ordinary answer',turnId:'plain'}])).toEqual([]);
    expect(entries([row(report(),{hostEffectsAllowed:false})])).toEqual([]);
    expect(entries([row(report(),{connectionProfile:'standard'})])).toEqual([]);
    const revised = entries([row(),row(report(2))]);
    expect(revised).toHaveLength(1);expect(revised[0].workspaceObject.revision).toBe(2);
    expect(entries([row(),row(report(),{turnId:'another-turn',messageId:'another-message'})])).toHaveLength(2);
    expect(entries([row(report(1,{status:'incomplete'}))])).toEqual([]);
    expect(entries([row(report(1,{grammar:'unsupported'}))])).toEqual([]);
    expect(entries([row(report(1,{source:'invalid JSON'}))])).toEqual([]);
    expect(entries([row(report(),{renderedContent:{reports:[report()],diagnostics:[{reportId:'review',message:'Invalid source'}]}})])).toEqual([]);
  });
  it('projects persisted execution-page reports, coalesces the final reply, and retains stable identity through a reset', async () => {
    const {projectedWorkspaceEntries} = await import('../services/workspaceAttachment.js');
    const execution = {kind:'iteration',turnId:'turn-1',renderKey:'execution',rounds:[{pageId:'page-1',renderedContent:{reports:[report(8)],diagnostics:[]}}]};
    const entries = projectedWorkspaceEntries([execution,row(report(1,{resetVersion:1}))], 'conversation',{includeInlineReports:true});
    expect(entries).toHaveLength(1);
    expect(entries[0].workspaceObject).toMatchObject({revision:1,origin:{turnId:'turn-1'},content:{renderer:'inlineReport',resetVersion:1}});
    expect(entries[0].workspaceObject.content).not.toHaveProperty('source');
    expect(entries[0].workspaceObject.content).not.toHaveProperty('dataSources');
    const other = projectedWorkspaceEntries([row()], 'foreign',{includeInlineReports:true});
    expect(other[0].windowId).not.toBe(entries[0].windowId);
  });
});

it('renders a hosted canonical report from its admitted conversation and retains the original chat frame', async () => {
  const React = await import('react');
  const {createRoot} = await import('react-dom/client');
  const store = await import('../services/chatStore.js');
  const registry = await import('forge/core/context/registry.js');
  const rich = await import('./chat/RichContent.jsx');
  const {default: InlineReportWorkspace} = await import('./InlineReportWorkspace.jsx');
  const {projectedWorkspaceEntries} = await import('../services/workspaceAttachment.js');
  const conversationId='owned-report-test';
  const renderedContent={schemaVersion:'1',parts:[],reports:[{scope:'message',id:'overview',grammar:'dashboard-v1',status:'committed',sequence:0,
    source:{title:'Delivery overview',blocks:[{id:'rows',kind:'dashboard.table',dataSourceRef:'rows',columns:[{key:'name',label:'Name'}]}]},
    dataSources:{rows:{id:'rows',version:2,format:'json',mode:'replace',payload:[{name:'Existing row'}]}}}],diagnostics:[]};
  const api = await import('../services/agentlyClient.js');
  const {client} = api;
  const identity=vi.spyOn(api,'getAuthMeSilently').mockResolvedValue({subject:'same-user'});
  const snapshot={conversationId,turns:[{turnId:'owned-turn',status:'completed',assistant:{final:{messageId:'owned-message',content:'Report ready.',renderedContent}}}]};
  const read=vi.spyOn(client,'readConversationHistory').mockResolvedValue({conversation:snapshot});
  const context={identity:{windowId:MAIN_CHAT_WINDOW_ID},metadata:{resource:{uri:'window://agently/chat-new',authorityBinding:'original-owner'}}};
  const previous=registry.getWindowContext(MAIN_CHAT_WINDOW_ID);
  registry.setWindowContext(MAIN_CHAT_WINDOW_ID,context);
  const renderer=vi.spyOn(rich,'HostedInlineReport');
  const host=document.createElement('div');document.body.append(host);const root=createRoot(host);const lifecycle=vi.fn();
  globalThis.IS_REACT_ACT_ENVIRONMENT=true;
  try {
    store.onTranscript(conversationId,{conversationId,turns:[{turnId:'owned-turn',status:'completed',assistant:{final:{messageId:'owned-message',content:'Report ready.',renderedContent}}}]});
    const entry=projectedWorkspaceEntries(store.getProjection(conversationId),conversationId,{includeInlineReports:true})[0];
    expect(entry).toBeDefined();
    await React.act(async()=>{root.render(React.createElement(InlineReportWorkspace,{entry,onLifecycle:lifecycle}));for(let i=0;i<25;i++)await Promise.resolve();});
    expect(host.textContent).toContain('Existing row');
    expect(renderer.mock.calls.at(-1)[0].context).toBe(context);
    expect(lifecycle).toHaveBeenCalledWith('ready');
    expect(read).toHaveBeenCalledWith({conversationId,includeModelCalls:true,includeToolCalls:true,includeFeeds:true,includeModelPayloads:false,includeToolPayloads:false});
    read.mockRejectedValueOnce(Object.assign(new Error('Forbidden'),{status:403}));
    await React.act(async()=>{root.render(React.createElement(InlineReportWorkspace,{key:'denied',entry,onLifecycle:lifecycle}));});
    expect(host.textContent).toContain('You don’t have access to this output');
    expect(host.textContent).not.toContain('Existing row');
    await React.act(async()=>host.querySelector('button').click());
    expect(host.textContent).toContain('Existing row');
    read.mockRejectedValueOnce(Object.assign(new Error('Not found'),{status:404}));
    await React.act(async()=>{root.render(React.createElement(InlineReportWorkspace,{entry:{...entry,conversationId:'missing-report-test'},onLifecycle:lifecycle}));});
    expect(host.querySelector('[role="alert"]').textContent).toContain('output is unavailable');
    expect(host.textContent).not.toContain('Existing row');
    expect(lifecycle).toHaveBeenCalledWith('failed');
    const invalid = structuredClone(renderedContent);
    invalid.reports[0].grammar = 'unsupported';
    invalid.reports[0].source.blocks = [{id:'invalid',kind:'unknownBlock'}];
    const errors = vi.spyOn(console,'error').mockImplementation(()=>{});
    try {
      lifecycle.mockClear();
      await React.act(async()=>{root.render(React.createElement(rich.HostedInlineReport,{assembly:invalid.reports[0],onLifecycle:lifecycle}));});
      expect(host.querySelector('[role="alert"]').textContent).toContain('output couldn’t be displayed');
      expect(lifecycle).toHaveBeenCalledWith('failed');
      expect(lifecycle).not.toHaveBeenCalledWith('ready');
    } finally {errors.mockRestore();}
  } finally {
    await React.act(async()=>root.unmount());host.remove();renderer.mockRestore();read.mockRestore();identity.mockRestore();store.reset(conversationId);
    if(previous)registry.setWindowContext(MAIN_CHAT_WINDOW_ID,previous);else registry.clearWindowContext(MAIN_CHAT_WINDOW_ID);
    delete globalThis.IS_REACT_ACT_ENVIRONMENT;
  }
});

it('retries a failed report by remounting its reader and respects an explicit reopen restriction', async () => {
  const {rememberWorkspaceObjects,reopenWorkspaceObject} = await import('../services/conversationWindow.js');
  const previous=activeWindows.peek();
  const report={windowId:'retry-owned-report',conversationId:'retry-owned-chat',workspaceObject:{objectId:'retry-report',content:{renderer:'inlineReport'},capabilities:{reopen:true},lifecycle:{state:'failed',attempt:2}}};
  rememberWorkspaceObjects(report.conversationId,[report]);activeWindows.value=[report];
  try {
    const opened=reopenWorkspaceObject(report.conversationId,report.windowId);
    expect(opened).toBeTruthy();
    expect(activeWindows.peek()[0].workspaceObject.lifecycle).toMatchObject({state:'opening',attempt:3});
    const unavailable={...report,windowId:'restricted-report',workspaceObject:{...report.workspaceObject,objectId:'restricted-report',capabilities:{reopen:false}}};
    rememberWorkspaceObjects(report.conversationId,[unavailable]);
    expect(reopenWorkspaceObject(report.conversationId,unavailable.windowId)).toBeNull();
    expect(activeWindows.peek()).toHaveLength(1);
  } finally {activeWindows.value=previous;}
});

it('keeps a closed tool output owned by its saved workspace and leaves other feeds at their authored placement', async () => {
  const React=await import('react');const {createRoot}=await import('react-dom/client');
  const {useWorkspaceAdapters}=await import('../services/useWorkspaceAdapters.js');
  const {rememberWorkspaceObjects}=await import('../services/conversationWindow.js');
  const {feedTracker,getActiveFeeds,clearFeedState}=await import('../services/toolFeedBus.js');
  const conversationId='closed-feed-test';const feedId=`${conversationId}::review`;
  rememberWorkspaceObjects(conversationId,[{windowId:'closed-review',conversationId,workspaceObject:{objectId:'review-object',content:{renderer:'toolFeed',feedId},lifecycle:{state:'closed'}}}]);
  feedTracker.setActive({feedId,conversationId,presentation:{target:'inline'}});
  feedTracker.setActive({feedId:'another-feed',conversationId,presentation:{target:'inline'}});
  function Harness(){useWorkspaceAdapters({mainConversationId:conversationId,workspaceWindows:[],documentPresentation:true,setActiveSurface:()=>{},setWorkspacePresentationMode:()=>{}});return null;}
  const host=document.createElement('div');document.body.append(host);const root=createRoot(host);globalThis.IS_REACT_ACT_ENVIRONMENT=true;
  try {
    await React.act(async()=>root.render(React.createElement(Harness)));
    expect(getActiveFeeds().find(feed=>feed.feedId===feedId).presentation.workspaceObjectId).toBe('review-object');
    expect(getActiveFeeds().find(feed=>feed.feedId==='another-feed').presentation.workspaceObjectId).toBeUndefined();
  }finally{await React.act(async()=>root.unmount());host.remove();clearFeedState();delete globalThis.IS_REACT_ACT_ENVIRONMENT;}
});

it('keeps phone viewer utilities available in the account menu and dismisses the drawer before approvals', async () => {
  const React=await import('react');const {createRoot}=await import('react-dom/client');
  const {default:MenuBar}=await import('./MenuBar.jsx');const {client}=await import('../services/agentlyClient');
  const auth=vi.spyOn(client,'getAuthMe').mockResolvedValue({displayName:'Example Reviewer'});
  const providers=vi.spyOn(client,'getAuthProviders').mockResolvedValue([]);
  const setOpen=vi.fn(),onNativeAction=vi.fn();
  const host=document.createElement('div');document.body.append(host);const root=createRoot(host);globalThis.IS_REACT_ACT_ENVIRONMENT=true;
  try {
    await React.act(async()=>{root.render(React.createElement(MenuBar,{sectionNavigation:true,compactNavigation:true,viewerVisible:true,conversationId:'example-conversation',approvals:{items:[],open:false,setOpen},onNativeAction}));});
    expect(host.querySelector('[data-testid="approval-bell"]')).toBeNull();
    await React.act(async()=>host.querySelector('[data-testid="user-menu-btn"]').click());
    expect(host.querySelector('[data-testid="user-menu"] [data-testid="approval-bell"]').textContent).toBe('Approvals');
    expect(host.querySelector('[data-testid="user-menu"] [data-testid="conversation-usage-nav"]').textContent).toBe('Conversation usage');
    await React.act(async()=>host.querySelector('[data-testid="approval-bell"]').click());
    expect(onNativeAction).toHaveBeenCalledTimes(1);expect(setOpen).toHaveBeenCalledWith(true);
    expect(host.querySelector('[data-testid="user-menu"]')).toBeNull();
  }finally{await React.act(async()=>root.unmount());host.remove();auth.mockRestore();providers.mockRestore();delete globalThis.IS_REACT_ACT_ENVIRONMENT;}
});

it('validates report identity from a fresh owning read and rejects missing, foreign and late-account results', async () => {
  const api = await import('../services/agentlyClient.js');
  const {readWorkspaceReport} = await import('../services/workspaceReportSource.js');
  const identity=vi.spyOn(api,'getAuthMeSilently').mockResolvedValue({subject:'same-user'});
  const scope=vi.spyOn(api,'getReportRestoreScope').mockReturnValue('account-A');
  const read=vi.spyOn(api.client,'readConversationHistory');
  const report={id:'result',scope:'message',grammar:'report-document-v1',status:'committed',sequence:2,source:{title:'Current title',blocks:[{id:'text',kind:'markdownBlock',markdown:'Current source'}]},dataSources:{}};
  const snapshot={conversationId:'source-test',turns:[{turnId:'turn',status:'completed',assistant:{final:{messageId:'answer',renderedContent:{schemaVersion:'1',parts:[],reports:[report],diagnostics:[]}}}}]};
  const entry={conversationId:'source-test',workspaceObject:{content:{turnId:'turn',messageId:'answer',reportId:'result',reportScope:'message'}}};
  try {
    read.mockResolvedValueOnce({conversation:snapshot});
    expect((await readWorkspaceReport(entry)).selected.assembly).toMatchObject({sequence:2,source:{title:'Current title'}});
    read.mockResolvedValueOnce({conversation:{...snapshot,conversationId:'another-chat'}});
    await expect(readWorkspaceReport(entry)).rejects.toThrow('does not match');
    read.mockResolvedValueOnce({conversation:{...snapshot,turns:[]}});
    await expect(readWorkspaceReport(entry)).rejects.toMatchObject({status:404});
    let release;read.mockReturnValueOnce(new Promise(resolve=>{release=resolve;}));
    const pending=readWorkspaceReport(entry);await Promise.resolve();scope.mockReturnValue('account-B');release({conversation:snapshot});
    await expect(pending).rejects.toThrow('account changed');
  } finally {read.mockRestore();scope.mockRestore();identity.mockRestore();}
});

it('offers direct recovery for a denied hosted tool output without showing cached data', async () => {
  const React=await import('react');const {createRoot}=await import('react-dom/client');
  const {client}=await import('../services/agentlyClient.js');
  const bus=await import('../services/toolFeedBus.js');
  const {default:ToolFeedDetail}=await import('./ToolFeedDetail.jsx');
  const conversationId='feed-recovery-test',feedId=`${conversationId}::review`;
  bus.updateFeedData('review',{data:{output:{message:'Private cached value'}}},conversationId);
  const get=vi.spyOn(client,'getFeedData').mockRejectedValueOnce(Object.assign(new Error('Forbidden'),{status:403}));
  const back=vi.fn(),lifecycle=vi.fn();const host=document.createElement('div');document.body.append(host);const root=createRoot(host);
  globalThis.IS_REACT_ACT_ENVIRONMENT=true;
  try {
    await React.act(async()=>{root.render(React.createElement(ToolFeedDetail,{hostedFeedId:feedId,conversationId,variant:'rail',placement:'workspace',onLifecycle:lifecycle,onReturn:back}));});
    expect(host.textContent).toContain('You don’t have access');expect(host.textContent).not.toContain('Private cached value');
    expect(bus.getFeedData('review',conversationId)).toBeNull();
    expect(lifecycle).toHaveBeenCalledWith('failed');
    await React.act(async()=>[...host.querySelectorAll('button')].find(node=>node.textContent==='Return to conversation').click());
    expect(back).toHaveBeenCalledOnce();
    get.mockResolvedValueOnce({data:{output:{message:'Recovered tool output'}}});
    await React.act(async()=>[...host.querySelectorAll('button')].find(node=>node.textContent==='Try again').click());
    expect(host.textContent).toContain('Recovered tool output');expect(lifecycle).toHaveBeenCalledWith('ready');
    const api=await import('../services/agentlyClient.js');
    const scope=vi.spyOn(api,'getReportRestoreScope').mockReturnValue('account-A');
    try {
      let release;get.mockReturnValueOnce(new Promise(resolve=>{release=resolve;}));
      await React.act(async()=>root.render(React.createElement(ToolFeedDetail,{key:'pending-other-account',hostedFeedId:feedId,conversationId,onLifecycle:lifecycle})));
      scope.mockReturnValue('account-B');await React.act(async()=>bus.clearFeedState());
      await React.act(async()=>release({data:{output:{message:'Previous account response'}}}));
      expect(host.textContent).not.toContain('Previous account response');
      expect(bus.getFeedData('review',conversationId)).toBeNull();
    } finally {scope.mockRestore();}

  } finally {await React.act(async()=>root.unmount());host.remove();get.mockRestore();bus.clearFeedState();delete globalThis.IS_REACT_ACT_ENVIRONMENT;}
});

it('does not let a delayed route bootstrap for A reopen A after navigating to B', () => {
  const delayedReplay = () => shouldReplayRouteConversationBootstrap({
    pathname: window.location.pathname, expectedConversationId: 'chat-a',
    authState: 'ready', mainConversationId: '', hasChatFeed: false, hasWorkspace: false,
  });
  const before = window.location.href;
  try {
    window.history.replaceState(null, '', '/conversation/chat-a');
    expect(delayedReplay()).toBe(true);
    window.history.replaceState(null, '', '/conversation/chat-b');
    expect(delayedReplay()).toBe(false);
    window.history.replaceState(null, '', '/');
    expect(delayedReplay()).toBe(false);
  } finally { window.history.replaceState(null, '', before); }
});
