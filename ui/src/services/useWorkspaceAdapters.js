import { useEffect } from 'react';
import { activeWindows, addWindow, selectedWindowId, selectedTabId } from 'forge/core';
import { MAIN_CHAT_WINDOW_ID } from './conversationWindow.js';
import { feedTracker, getActiveFeeds, onFeedChange } from './toolFeedBus.js';
import { mcpWorkspaceDescriptor, toolFeedWorkspaceDescriptor } from './workspaceAdapters.js';

export function setWorkspaceLifecycle(windowId, state) {
  const entries = activeWindows.peek();
  const entry = entries.find((item) => item.windowId === windowId);
  if (!entry?.workspaceObject || entry.workspaceObject.lifecycle?.state === state) return;
  activeWindows.value = entries.map((item) => item.windowId === windowId ? {...item,
    workspaceObject: {...item.workspaceObject, lifecycle: {...item.workspaceObject.lifecycle, state}},
  } : item);
}

export function useWorkspaceAdapters({mainConversationId, workspaceWindows, setActiveSurface, setWorkspacePresentationMode}) {
  useEffect(() => {
    if (typeof window === 'undefined') return () => {};
    const onOpenMCPUIWorkspace = (event) => {
      const detail = event?.detail || {};
      const uri = String(detail?.uri || '').trim();
      const conversationId = String(detail?.conversationId || mainConversationId || '').trim();
      if (!uri || !conversationId || conversationId !== String(mainConversationId || '').trim()) return;
      const navigation = detail?.navigation && typeof detail.navigation === 'object'
        ? detail.navigation
        : { label: String(detail?.title || 'Interactive app').trim(), icon: 'application' };
      const previousWindowId = selectedWindowId.peek();
      const previousTabId = selectedTabId.peek();
      const descriptor = mcpWorkspaceDescriptor({uri, conversationId, title: navigation.label,
        turnId: detail.origin?.turnId, toolCallId: detail.origin?.toolCallId, historical: detail.historical !== false});
      const opened = addWindow(
        String(navigation?.label || detail?.title || 'Interactive app').trim(),
        MAIN_CHAT_WINDOW_ID,
        'mcpui/workspace',
        null,
        true,
        { uri, conversationId },
        {
          ...descriptor,
          autoIndexTitle: false,
          conversationId,
          presentation: 'hosted',
          region: 'chat.top',
          navigation,
          mcpUI: { uri, title: String(detail?.title || navigation?.label || 'Interactive app').trim() },
        }
      );
      if (opened?.windowId && detail.historical !== false) {
        selectedWindowId.value = previousWindowId;
        selectedTabId.value = previousTabId;
      }
    };
    window.addEventListener('agently:mcpui-workspace-open', onOpenMCPUIWorkspace);
    return () => window.removeEventListener('agently:mcpui-workspace-open', onOpenMCPUIWorkspace);
  }, [mainConversationId, setActiveSurface]);

  useEffect(() => {
    const promote = (event) => {
      const feed = event.detail;
      if (!feed?.feedId || feed.conversationId !== mainConversationId) return;
      const descriptor = toolFeedWorkspaceDescriptor(feed);
      addWindow(feed.title || 'Tool output', MAIN_CHAT_WINDOW_ID, 'toolfeed/workspace', null, true, {}, {
        ...descriptor, conversationId: mainConversationId, presentation: 'hosted', region: 'chat.top',
        navigation: descriptor.workspaceObject.navigation, hostOpenState: 'user_requested',
      });
      feedTracker.setActive({...feed, presentation: {...feed.presentation, workspaceObjectId: descriptor.workspaceObject.objectId}});
      setWorkspacePresentationMode('full');
      setActiveSurface('workspace');
    };
    window.addEventListener('agently:toolfeed-workspace-open', promote);
    return () => window.removeEventListener('agently:toolfeed-workspace-open', promote);
  }, [mainConversationId, setActiveSurface, setWorkspacePresentationMode]);

  useEffect(() => {
    const reconcileFeedPlacement = () => {
      for (const feed of getActiveFeeds()) {
        if (feed.conversationId !== mainConversationId) continue;
        const owner = workspaceWindows.find((entry) => entry.workspaceObject?.content?.renderer === 'toolFeed'
          && entry.workspaceObject.content.feedId === feed.feedId);
        const objectId = owner?.workspaceObject?.objectId;
        if (feed.presentation?.workspaceObjectId !== objectId) {
          feedTracker.setActive({...feed, presentation: {...feed.presentation, workspaceObjectId: objectId}});
        }
      }
    };
    reconcileFeedPlacement();
    return onFeedChange(reconcileFeedPlacement);
  }, [workspaceWindows, mainConversationId]);

}
