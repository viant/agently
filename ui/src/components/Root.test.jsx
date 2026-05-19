import { describe, expect, it } from 'vitest';

import {
  isConversationOwnedWorkspaceWindow,
  isConversationHostedWorkspaceChild,
  isHostedWorkspaceChildOfMainChat,
  resolveHostedWorkspaceTabLabel,
  resolveHostedWorkspaceTabs,
  resolveHostedBottomWindow,
  resolveRouteBootstrapAction,
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
      parameters: { AdOrderId: [2637048] },
      resolvedMetrics: { name: 'Delaware_SB_display_Bally', orderId: 2637048 }
    })).toBe('Delaware_SB_display_Bally');
    expect(resolveMainWindowHeaderTitle({
      windowKey: 'orderPerformance',
      parameters: { AdOrderId: [2637048] }
    })).toBe('Order 2637048');
    expect(resolveMainWindowHeaderTitle({
      windowKey: 'orderPerformance',
      parameters: { AdOrderId: [2609393] },
      resolvedMetrics: { name: 'Stale Previous Order', orderId: 2656980 }
    })).toBe('Order 2609393');
    expect(resolveMainWindowHeaderTitle({
      windowKey: 'campaign',
      parameters: { CampaignId: [551665] },
      resolvedMetrics: { campaignName: '6TJH_ThomasJHenryLaw_CTVPilot_2026', campaignId: 551665 }
    })).toBe('6TJH_ThomasJHenryLaw_CTVPilot_2026');
    expect(resolveMainWindowHeaderTitle({
      windowKey: 'campaign',
      parameters: { CampaignId: [551665] },
      windowTitle: '6TJH_ThomasJHenryLaw_CTVPilot_2026 (551665)'
    })).toBe('6TJH_ThomasJHenryLaw_CTVPilot_2026');
    expect(resolveMainWindowHeaderTitle({
      windowKey: 'campaignPerformance',
      parameters: { CampaignId: [551665] }
    })).toBe('Campaign 551665');
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

  it('uses compact order ids for hosted workspace compare tabs', () => {
    expect(resolveHostedWorkspaceTabLabel({
      windowKey: 'order',
      parameters: { AdOrderId: [2656980] },
      windowTitle: 'Order 2656980'
    })).toBe('2656980');

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
      { windowId: 'order_1', label: '2656980', isActive: false },
      { windowId: 'order_2', label: '2609393', isActive: true }
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
});
