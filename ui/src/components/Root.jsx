import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useSignals } from '@preact/signals-react/runtime';
import { Dialog } from '@blueprintjs/core';
import { activeWindows, addWindow, getFormSignal, getViewSignal, removeWindow, selectedTabId, selectedWindowId } from 'forge/core';
import { WindowManager, WindowContent } from 'forge/components';
import { DetailContext } from '../context/DetailContext';
import { ConversationViewContext } from '../context/ConversationViewContext';
import DetailPanel from './DetailPanel';
import DetailPopoutWindow from './DetailPopoutWindow';
import CodeDiffDialog from './CodeDiffDialog';
import ConfirmDialog from './ConfirmDialog';
import FileViewDialog from './FileViewDialog';
import MenuBar, { refreshWindowDataSources } from './MenuBar';
import ToolFeedWorkspace from './ToolFeedWorkspace';
import UsageBar from './UsageBar';
import StatusBar from './StatusBar';
import Sidebar from './Sidebar';
import ElicitationOverlay from './ElicitationOverlay';
import { useApprovalQueue } from '../hooks/useApprovalQueue';
import { CHAT_WINDOW_KEY, MAIN_CHAT_WINDOW_ID, ensureWorkspaceWindowForConversation, getScopedConversationSelection, getScopedWorkspacePresentationMode, getSelectedWindow, isLinkedChildWindow, openConversationInMainWindow, requestNewConversationInMainWindow, resolveConversationSelection, resolveWorkspaceWindowForConversation, resolveWorkspaceWindowsForConversation, returnToParentConversation, setScopedWorkspacePresentationMode, setScopedWorkspaceSelection, setScopedWorkspaceState } from '../services/conversationWindow';
import { AGENTLY_UI_BUILD } from '../buildInfo';
import { conversationIDFromPath } from '../services/chatRuntime';
import { beginLogin, client, getAuthMeSilently, recoverSessionSilently } from '../services/agentlyClient';

const SIDEBAR_WIDTH_KEY = 'agently.sidebarWidth';
const SIDEBAR_DEFAULT_WIDTH = 320;
const SIDEBAR_MIN_WIDTH = 220;
const SIDEBAR_MAX_WIDTH = 520;
const SHOW_EXECUTION_DETAILS_KEY = 'agently.showExecutionDetails';
const SHOW_INTAKE_DETAILS_KEY = 'agently.showIntakeDetails';
const SHOW_WORKSPACE_WINDOW_KEY = 'agently.showWorkspaceWindow';
const SHOW_TOOL_FEEDS_KEY = 'agently.showToolFeeds';

function clampSidebarWidth(value) {
  const next = Number(value || 0);
  if (!Number.isFinite(next)) return SIDEBAR_DEFAULT_WIDTH;
  return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(next)));
}

export function resolveInitialAuthState(providers, me) {
  const normalized = Array.isArray(providers) ? providers : [];
  const hasLocalProvider = normalized.some((entry) => {
    const type = String(entry?.type || '').trim().toLowerCase();
    return type === 'local';
  });
  const hasOAuthProvider = normalized.some((entry) => {
    const type = String(entry?.type || '').trim().toLowerCase();
    return type === 'bff' || type === 'oidc' || type === 'jwt';
  });
  const hasLocalOnlyProvider = normalized.length > 0 && normalized.every((entry) => {
    const type = String(entry?.type || '').trim().toLowerCase();
    return type === 'local';
  });
  const hasAnyProvider = normalized.length > 0;
  if (me) return 'ready';
  if (hasLocalProvider) return 'ready';
  if (hasLocalOnlyProvider) return 'ready';
  // Auth is enabled (providers configured) but user is not authenticated.
  if (hasOAuthProvider) return 'required';
  // For local-only auth: if providers are listed but /me returned null,
  // the session is gone (server restart) — show sign-in prompt.
  if (hasAnyProvider && !me) return 'required';
  return 'ready';
}

export function resolveOAuthProviderLabel(providers) {
  const normalized = Array.isArray(providers) ? providers : [];
  const match = normalized.find((entry) => {
    const type = String(entry?.type || '').trim().toLowerCase();
    return type === 'bff' || type === 'oidc' || type === 'jwt';
  });
  if (!match) return '';
  // Prefer explicit label; fall back to name; skip generic values.
  const label = String(match?.label || match?.name || '').trim();
  const lower = label.toLowerCase();
  if (!label || lower === 'oauth' || lower === 'oauth2' || lower === 'jwt') {
    return '';
  }
  return label;
}

export function resolveSelectedMainWindow(windows = [], selectedTabWindowId = '', selectedFocusedWindowId = '', fallbackWindow = null) {
  const list = Array.isArray(windows) ? windows : [];
  const primaryId = String(selectedTabWindowId || selectedFocusedWindowId || '').trim();
  if (primaryId) {
    const matched = list.find((entry) => String(entry?.windowId || '').trim() === primaryId);
    if (matched) return matched;
  }
  return fallbackWindow || null;
}

export function shouldShowChatChrome(windowEntry = null) {
  return String(windowEntry?.windowKey || '').trim() === CHAT_WINDOW_KEY;
}

export function isHostedWorkspaceChildOfMainChat(windowEntry = null) {
  return String(windowEntry?.presentation || '').trim().toLowerCase() === 'hosted'
    && String(windowEntry?.region || '').trim().toLowerCase() === 'chat.top'
    && String(windowEntry?.parentKey || '').trim() === MAIN_CHAT_WINDOW_ID
    && windowEntry?.inTab !== false;
}

export function isConversationHostedWorkspaceChild(windowEntry = null, conversationId = '') {
  return isHostedWorkspaceChildOfMainChat(windowEntry)
    && windowBelongsToConversation(windowEntry, conversationId);
}

export function isConversationOwnedWorkspaceWindow(windowEntry = null, conversationId = '') {
  return isWorkspaceRegionWindow(windowEntry)
    && windowBelongsToConversation(windowEntry, conversationId);
}

function isWorkspaceRegionWindow(windowEntry = null) {
  return String(windowEntry?.presentation || '').trim().toLowerCase() === 'hosted'
    && String(windowEntry?.region || '').trim().toLowerCase() === 'chat.top';
}

function isChatBottomRegionWindow(windowEntry = null) {
  return String(windowEntry?.presentation || '').trim().toLowerCase() === 'hosted'
    && String(windowEntry?.region || '').trim().toLowerCase() === 'chat.bottom';
}

function windowBelongsToConversation(windowEntry = null, conversationId = '') {
  const targetId = String(conversationId || '').trim();
  if (!targetId) return false;
  return String(windowEntry?.conversationId || '').trim() === targetId;
}

export function resolveHostedBottomWindow(selectedWindow = null, mainChatWindow = null, windows = [], conversationId = '') {
  if (selectedWindow && isLinkedChildWindow(selectedWindow)) {
    return selectedWindow;
  }
  if (selectedWindow && isChatBottomRegionWindow(selectedWindow) && windowBelongsToConversation(selectedWindow, conversationId)) {
    return selectedWindow;
  }
  const list = Array.isArray(windows) ? windows : [];
  const liveMatch = list.find((entry) => isChatBottomRegionWindow(entry) && windowBelongsToConversation(entry, conversationId));
  if (liveMatch) {
    return liveMatch;
  }
  return mainChatWindow || null;
}

export function resolveMainWindowCloseConversationId(mainWindowConversationId = '') {
  return String(mainWindowConversationId || '').trim();
}

function resolveWindowOrderId(windowEntry = null) {
  return String(windowEntry?.parameters?.AdOrderId?.[0] ?? '').trim();
}

function resolveWindowCampaignId(windowEntry = null) {
  return String(
    windowEntry?.parameters?.CampaignId?.[0]
    ?? windowEntry?.parameters?.campaignId?.[0]
    ?? ''
  ).trim();
}

export function resolveHostedWorkspaceTabLabel(windowEntry = null) {
  const orderId = resolveWindowOrderId(windowEntry);
  if (orderId) return orderId;
  return resolveMainWindowHeaderTitle(windowEntry);
}

export function resolveHostedWorkspaceTabs(workspaceWindows = [], activeWindowId = '') {
  const selectedId = String(activeWindowId || '').trim();
  return (Array.isArray(workspaceWindows) ? workspaceWindows : []).map((entry) => ({
    windowId: String(entry?.windowId || '').trim(),
    label: resolveHostedWorkspaceTabLabel(entry),
    isActive: String(entry?.windowId || '').trim() === selectedId,
  }));
}

export function resolveMainWindowHeaderTitle(windowEntry = null) {
  const windowKey = String(windowEntry?.windowKey || '').trim();
  if (windowKey === 'orderPerformance' || windowKey === 'order') {
    const metrics = windowEntry?.resolvedMetrics || {};
    const parameterOrderId = resolveWindowOrderId(windowEntry);
    const metricsOrderId = String(metrics?.orderId ?? metrics?.orderID ?? '').trim();
    const name = String(metrics?.name || '').trim();
    const orderId = parameterOrderId || metricsOrderId;
    if (parameterOrderId && metricsOrderId && metricsOrderId !== parameterOrderId) return `Order ${parameterOrderId}`;
    if (name) return name;
    if (orderId) return `Order ${orderId}`;
  }
  if (windowKey === 'campaign' || windowKey === 'campaignPerformance') {
    const metrics = windowEntry?.resolvedMetrics || {};
    const parameterCampaignId = resolveWindowCampaignId(windowEntry);
    const metricsCampaignId = String(metrics?.campaignId ?? metrics?.campaignID ?? '').trim();
    const name = String(metrics?.campaignName || metrics?.name || '').trim();
    if (name) return name;
    const campaignId = parameterCampaignId || metricsCampaignId;
    const title = String(windowEntry?.windowTitle || '').trim();
    if (title && parameterCampaignId) {
      const suffix = ` (${parameterCampaignId})`;
      if (title.endsWith(suffix)) {
        return title.slice(0, -suffix.length).trim();
      }
    }
    if (campaignId) return `Campaign ${campaignId}`;
  }
  const title = String(windowEntry?.windowTitle || windowEntry?.windowKey || '').trim();
  return title;
}

export function shouldShowMainWindowHeader(windowEntry = null) {
  const windowKey = String(windowEntry?.windowKey || '').trim();
  if (windowKey === 'schedule' || windowKey === 'schedule/history') {
    return false;
  }
  if (isLinkedChildWindow(windowEntry)) {
    return true;
  }
  return !shouldShowChatChrome(windowEntry)
    && windowEntry?.inTab !== false
    && resolveMainWindowHeaderTitle(windowEntry) !== '';
}

export function resolveRouteBootstrapAction(pathname = '', authState = '') {
  if (authState !== 'ready') return { type: 'none', conversationId: '' };
  const routeConversationId = conversationIDFromPath(pathname);
  if (routeConversationId) {
    return { type: 'conversation', conversationId: routeConversationId };
  }
  if (pathname === '/' || pathname === '/ui') {
    return { type: 'new', conversationId: '' };
  }
  return { type: 'none', conversationId: '' };
}

export default function Root() {
  useSignals();
  void selectedTabId.value;
  void selectedWindowId.value;
  void activeWindows.value;
  const [conversationSelectionEpoch, setConversationSelectionEpoch] = useState(0);
  const [selectedTool, setSelectedTool] = useState(null);
  const [isPanelOpen, setIsPanelOpen] = useState(false);
  const [detailMode, setDetailMode] = useState(() => {
    if (typeof window === 'undefined') return 'right';
    const stored = String(window.localStorage?.getItem('agently.detailMode') || '').trim();
    return stored === 'left' || stored === 'window' ? stored : 'right';
  });
  const [isSidebarOpen, setIsSidebarOpen] = useState(true);
  const [sidebarWidth, setSidebarWidth] = useState(() => {
    if (typeof window === 'undefined') return SIDEBAR_DEFAULT_WIDTH;
    try {
      return clampSidebarWidth(window.localStorage?.getItem(SIDEBAR_WIDTH_KEY));
    } catch (_) {
      return SIDEBAR_DEFAULT_WIDTH;
    }
  });
  const [authState, setAuthState] = useState('checking');
  const [oauthProviderLabel, setOAuthProviderLabel] = useState('');
  const [showExecutionDetails, setShowExecutionDetails] = useState(() => {
    if (typeof window === 'undefined') return true;
    try {
      const stored = String(window.localStorage?.getItem(SHOW_EXECUTION_DETAILS_KEY) || '').trim().toLowerCase();
      if (stored === 'false') return false;
    } catch (_) {}
    return true;
  });
  const [showIntakeDetails, setShowIntakeDetails] = useState(() => {
    if (typeof window === 'undefined') return false;
    try {
      const stored = String(window.localStorage?.getItem(SHOW_INTAKE_DETAILS_KEY) || '').trim().toLowerCase();
      if (stored === 'true') return true;
    } catch (_) {}
    return false;
  });
  const [showWorkspaceWindow, setShowWorkspaceWindow] = useState(() => {
    if (typeof window === 'undefined') return true;
    try {
      const stored = String(window.localStorage?.getItem(SHOW_WORKSPACE_WINDOW_KEY) || '').trim().toLowerCase();
      if (stored === 'false') return false;
    } catch (_) {}
    return true;
  });
  const [showToolFeeds, setShowToolFeeds] = useState(() => {
    if (typeof window === 'undefined') return true;
    try {
      const stored = String(window.localStorage?.getItem(SHOW_TOOL_FEEDS_KEY) || '').trim().toLowerCase();
      if (stored === 'false') return false;
    } catch (_) {}
    return true;
  });
  const [workspacePresentationMode, setWorkspacePresentationModeState] = useState('split');
  const [stableMainChatWindow, setStableMainChatWindow] = useState(null);
  const resizeStateRef = useRef(null);
  const approvals = useApprovalQueue(authState === 'ready');
  const selectedWindow = resolveSelectedMainWindow(
    activeWindows.value,
    selectedTabId.value,
    selectedWindowId.value,
    getSelectedWindow()
  );
  const selectedWindowForTitle = selectedWindow;
  const mainChatWindow = useMemo(
    () => (Array.isArray(activeWindows.value)
      ? activeWindows.value.find((entry) => String(entry?.windowId || '').trim() === MAIN_CHAT_WINDOW_ID) || null
      : null),
    [activeWindows.value]
  );
  useEffect(() => {
    if (mainChatWindow?.windowId) {
      setStableMainChatWindow(mainChatWindow);
    }
  }, [mainChatWindow]);
  const effectiveMainChatWindow = mainChatWindow || stableMainChatWindow;
  const mainConversationId = String(
    resolveConversationSelection(MAIN_CHAT_WINDOW_ID)
    || ''
  ).trim();
  const resolvedConversationWorkspaceWindow = useMemo(
    () => resolveWorkspaceWindowForConversation(mainConversationId),
    [mainConversationId, activeWindows.value]
  );
  const workspaceWindows = useMemo(
    () => resolveWorkspaceWindowsForConversation(mainConversationId),
    [mainConversationId, activeWindows.value, selectedWindowId.value, selectedTabId.value]
  );
  const workspaceStatePersistenceSignature = workspaceWindows.map((entry) => {
    const windowId = String(entry?.windowId || '').trim();
    const hasInlineMetadata = entry?.inlineMetadata && typeof entry.inlineMetadata === 'object';
    const inlineNamespace = hasInlineMetadata ? String(entry.inlineMetadata.namespace || '').trim() : '';
    const windowFormState = windowId ? (getFormSignal(`${windowId}:windowForm`)?.value || {}) : {};
    const viewState = windowId ? (getViewSignal(windowId)?.value || {}) : {};
    return `${windowId}:${hasInlineMetadata ? 1 : 0}:${inlineNamespace}:${JSON.stringify(windowFormState)}:${JSON.stringify(viewState)}`;
  }).join('|');
  const linkedChildWindow = isLinkedChildWindow(selectedWindow) ? selectedWindow : null;
  const showChatChrome = shouldShowChatChrome(selectedWindow);
  const activeWindowTitle = resolveMainWindowHeaderTitle(selectedWindowForTitle);
  const activeWorkspaceWindow = !linkedChildWindow
    ? (
        selectedWindow
        && (
          (selectedWindow?.inTab !== false
            && windowBelongsToConversation(selectedWindow, mainConversationId)
            && isWorkspaceRegionWindow(selectedWindow))
          || isConversationHostedWorkspaceChild(selectedWindow, mainConversationId)
        )
          ? selectedWindow
          : (isWorkspaceRegionWindow(resolvedConversationWorkspaceWindow) ? resolvedConversationWorkspaceWindow : null)
      )
    : null;
  const workspaceTabs = useMemo(
    () => resolveHostedWorkspaceTabs(workspaceWindows, activeWorkspaceWindow?.windowId),
    [workspaceWindows, activeWorkspaceWindow?.windowId]
  );
  const activeWorkspaceTitle = resolveMainWindowHeaderTitle(activeWorkspaceWindow);
  const workspaceSharePct = Number(activeWorkspaceWindow?.workspaceSharePct || 0);
  const workspaceMinHeight = Number(activeWorkspaceWindow?.workspaceMinHeight || 0);
  const activeConversationId = String(
    getScopedConversationSelection(String(selectedWindow?.windowId || '').trim())
    || ''
  ).trim();
  const hostedBottomWindow = resolveHostedBottomWindow(selectedWindow, effectiveMainChatWindow, activeWindows.value, mainConversationId);
  const shouldRenderSplitShell = !!(
    effectiveMainChatWindow
    && (
      !selectedWindow
      ||
      showChatChrome
      || !!activeWorkspaceWindow
      || isChatBottomRegionWindow(selectedWindow)
    )
  );
  const showWorkspacePane = !!(workspaceWindows.length > 0 && activeWorkspaceWindow && showWorkspaceWindow);
  const isWorkspaceFull = workspacePresentationMode === 'full';
  const isWorkspaceCollapsed = activeWorkspaceWindow?.workspaceCollapsed === true;

  const setWorkspacePresentationMode = (mode) => {
    const next = String(mode || '').trim().toLowerCase() === 'full' ? 'full' : 'split';
    setWorkspacePresentationModeState(next);
    if (mainConversationId) {
      setScopedWorkspacePresentationMode(mainConversationId, next);
    }
  };

  const setActiveWorkspaceCollapsed = (collapsed) => {
    const targetWindowId = String(activeWorkspaceWindow?.windowId || '').trim();
    if (!targetWindowId) return;
    const nextCollapsed = collapsed === true;
    activeWindows.value = (Array.isArray(activeWindows.value) ? activeWindows.value : []).map((entry) => (
      String(entry?.windowId || '').trim() === targetWindowId
        ? { ...entry, workspaceCollapsed: nextCollapsed }
        : entry
    ));
  };

  const focusWorkspaceWindow = (windowId = '') => {
    const targetId = String(windowId || '').trim();
    if (!targetId) return;
    const target = workspaceWindows.find((entry) => String(entry?.windowId || '').trim() === targetId);
    if (!target) return;
    selectedWindowId.value = target.windowId;
    selectedTabId.value = target.windowId;
    if (mainConversationId) {
      setScopedWorkspaceSelection(mainConversationId, target.windowId);
      setScopedWorkspaceState(mainConversationId, workspaceWindows);
    }
  };

  const closeActiveWorkspaceWindow = () => {
    const restoreConversationId = resolveMainWindowCloseConversationId(
      getScopedConversationSelection(MAIN_CHAT_WINDOW_ID)
    );
    const activeWindowId = String(activeWorkspaceWindow?.windowId || '').trim();
    if (!activeWindowId) {
      openConversationInMainWindow(restoreConversationId);
      return;
    }
    const currentIndex = workspaceWindows.findIndex((entry) => String(entry?.windowId || '').trim() === activeWindowId);
    const remaining = workspaceWindows.filter((entry) => String(entry?.windowId || '').trim() !== activeWindowId);
    setWorkspacePresentationModeState('split');
    setScopedWorkspacePresentationMode(restoreConversationId, 'split');
    removeWindow(activeWindowId);
    if (remaining.length > 0) {
      const nextIndex = currentIndex <= 0 ? 0 : Math.min(currentIndex - 1, remaining.length - 1);
      const nextWindow = remaining[nextIndex] || remaining[0];
      setScopedWorkspaceSelection(restoreConversationId, nextWindow.windowId);
      setScopedWorkspaceState(restoreConversationId, remaining);
      selectedWindowId.value = nextWindow.windowId;
      selectedTabId.value = nextWindow.windowId;
      return;
    }
    setScopedWorkspaceSelection(restoreConversationId, '');
    setScopedWorkspaceState(restoreConversationId, null);
    openConversationInMainWindow(restoreConversationId);
  };

  const setMode = (mode) => {
    const next = mode === 'left' || mode === 'window' ? mode : 'right';
    setDetailMode(next);
    if (typeof window !== 'undefined') {
      try { window.localStorage?.setItem('agently.detailMode', next); } catch (_) {}
    }
  };

  const value = useMemo(() => ({
    showDetail: (toolCall) => {
      setSelectedTool(toolCall || null);
      setIsPanelOpen(true);
    },
    closeDetail: () => {
      setIsPanelOpen(false);
      setSelectedTool(null);
    },
    undockDetail: () => setMode('window'),
    dockDetail: () => setMode('right'),
    setDetailMode: setMode,
    detailMode
  }), [detailMode]);

  useEffect(() => {
    console.info('[agently-ui-build]', AGENTLY_UI_BUILD);
  }, []);

  useEffect(() => {
    let mounted = true;
    Promise.allSettled([client.getAuthProviders(), getAuthMeSilently()])
      .then(async (results) => {
        if (!mounted) return;
        const providers = results[0]?.status === 'fulfilled' ? results[0].value : [];
        let me = results[1]?.status === 'fulfilled' ? results[1].value : null;
        if (!me && Array.isArray(providers) && providers.length > 0) {
          const recovered = await recoverSessionSilently();
          if (!mounted) return;
          if (recovered) {
            me = await getAuthMeSilently();
          }
        }
        setOAuthProviderLabel(resolveOAuthProviderLabel(providers));
        setAuthState(resolveInitialAuthState(providers, me));
      })
      .catch((err) => {
        if (!mounted) return;
        const status = Number(err?.status || 0);
        if (status === 401 || status === 403) {
          setAuthState('required');
          return;
        }
        setAuthState('ready');
      });
    const onUnauthorized = () => {
      setAuthState('required');
    };
    const onAuthorized = () => {
      setAuthState('ready');
    };
    window.addEventListener('agently:unauthorized', onUnauthorized);
    window.addEventListener('agently:authorized', onAuthorized);
    return () => {
      mounted = false;
      window.removeEventListener('agently:unauthorized', onUnauthorized);
      window.removeEventListener('agently:authorized', onAuthorized);
    };
  }, []);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    try {
      window.localStorage?.setItem(SIDEBAR_WIDTH_KEY, String(clampSidebarWidth(sidebarWidth)));
    } catch (_) {}
  }, [sidebarWidth]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    try {
      window.localStorage?.setItem(SHOW_EXECUTION_DETAILS_KEY, showExecutionDetails ? 'true' : 'false');
    } catch (_) {}
  }, [showExecutionDetails]);
  useEffect(() => {
    if (typeof window === 'undefined') return;
    try {
      window.localStorage?.setItem(SHOW_INTAKE_DETAILS_KEY, showIntakeDetails ? 'true' : 'false');
    } catch (_) {}
  }, [showIntakeDetails]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    try {
      window.localStorage?.setItem(SHOW_WORKSPACE_WINDOW_KEY, showWorkspaceWindow ? 'true' : 'false');
    } catch (_) {}
  }, [showWorkspaceWindow]);
  useEffect(() => {
    if (typeof window === 'undefined') return;
    try {
      window.localStorage?.setItem(SHOW_TOOL_FEEDS_KEY, showToolFeeds ? 'true' : 'false');
    } catch (_) {}
  }, [showToolFeeds]);

  useEffect(() => {
    if (typeof window === 'undefined') return () => {};

    const handlePointerMove = (event) => {
      const state = resizeStateRef.current;
      if (!state) return;
      const delta = Number(event.clientX || 0) - state.startX;
      setSidebarWidth(clampSidebarWidth(state.startWidth + delta));
    };

    const stopResize = () => {
      if (!resizeStateRef.current) return;
      resizeStateRef.current = null;
      try { document.body.style.cursor = ''; } catch (_) {}
      try { document.body.style.userSelect = ''; } catch (_) {}
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', stopResize);
    window.addEventListener('pointercancel', stopResize);
    return () => {
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', stopResize);
      window.removeEventListener('pointercancel', stopResize);
    };
  }, []);

  useEffect(() => {
    const windows = Array.isArray(activeWindows.value) ? activeWindows.value : [];
    const chatWindows = windows.filter((entry) => String(entry?.windowId || '').trim() === MAIN_CHAT_WINDOW_ID);
    if (chatWindows.length === 0 && windows.length === 0) {
      addWindow('Chat', null, CHAT_WINDOW_KEY, null, true, {}, { autoIndexTitle: true });
      return;
    }
    if (chatWindows.length > 1) {
      const keep = chatWindows[0];
      for (let index = 1; index < chatWindows.length; index++) {
        const extra = chatWindows[index];
        if (extra?.windowId) {
          removeWindow(extra.windowId);
        }
      }
      if (keep?.windowId) {
        selectedTabId.value = keep.windowId;
        selectedWindowId.value = keep.windowId;
      }
    }
  }, []);

  useEffect(() => {
    const windowId = String(selectedWindow?.windowId || '').trim();
    const windowKey = String(selectedWindow?.windowKey || '').trim();
    if (!windowId || !windowKey) return;
    const refreshRefs = windowKey === 'schedule'
      ? ['schedules']
      : (windowKey === 'schedule/history' ? ['runs'] : []);
    if (refreshRefs.length === 0) return;
    const timer = window.setTimeout(() => {
      refreshWindowDataSources(windowId, refreshRefs);
    }, 0);
    return () => window.clearTimeout(timer);
  }, [selectedWindow?.windowId, selectedWindow?.windowKey]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const action = resolveRouteBootstrapAction(window.location.pathname, authState);
    if (action.type === 'conversation') {
      openConversationInMainWindow(action.conversationId);
      return;
    }
    if (action.type === 'new') {
      requestNewConversationInMainWindow();
    }
  }, [authState]);

  useEffect(() => {
    if (typeof window === 'undefined') return () => {};
    let active = true;
    const bump = () => {
      queueMicrotask(() => {
        if (!active) return;
        setConversationSelectionEpoch((value) => value + 1);
      });
    };
    window.addEventListener('agently:conversation-select', bump);
    window.addEventListener('agently:conversation-new', bump);
    window.addEventListener('agently:workspace-state', bump);
    return () => {
      active = false;
      window.removeEventListener('agently:conversation-select', bump);
      window.removeEventListener('agently:conversation-new', bump);
      window.removeEventListener('agently:workspace-state', bump);
    };
  }, []);

  useEffect(() => {
    const conversationId = String(mainConversationId || '').trim();
    if (!conversationId) return;
    if (linkedChildWindow?.windowId) return;
    ensureWorkspaceWindowForConversation(conversationId);
  }, [linkedChildWindow?.windowId, mainConversationId, conversationSelectionEpoch]);

  useEffect(() => {
    const conversationId = mainConversationId;
    if (!conversationId) return;
    const ownedActiveWorkspaceWindow = isConversationOwnedWorkspaceWindow(activeWorkspaceWindow, conversationId)
      ? activeWorkspaceWindow
      : null;
    if (workspaceWindows.length > 0) {
      const scopedWindowId = ownedActiveWorkspaceWindow?.windowId || workspaceWindows[workspaceWindows.length - 1]?.windowId || '';
      if (scopedWindowId) {
        setScopedWorkspaceSelection(conversationId, scopedWindowId);
      }
      setScopedWorkspaceState(conversationId, workspaceWindows);
      return;
    }
    if (ownedActiveWorkspaceWindow?.windowId) {
      setScopedWorkspaceSelection(conversationId, ownedActiveWorkspaceWindow.windowId);
      setScopedWorkspaceState(conversationId, [ownedActiveWorkspaceWindow]);
      return;
    }
    const selectedWindowIdValue = String(selectedWindow?.windowId || '').trim();
    if (selectedWindowIdValue === MAIN_CHAT_WINDOW_ID) {
      return;
    }
  }, [activeWorkspaceWindow?.windowId, workspaceWindows, selectedWindow?.windowId, mainConversationId, workspaceStatePersistenceSignature]);

  useEffect(() => {
    if (!activeWorkspaceWindow?.windowId) return;
    if (showWorkspaceWindow) return;
    setShowWorkspaceWindow(true);
  }, [activeWorkspaceWindow?.windowId, showWorkspaceWindow]);

  useEffect(() => {
    const conversationId = String(mainConversationId || '').trim();
    if (!conversationId) {
      setWorkspacePresentationModeState('split');
      return;
    }
    setWorkspacePresentationModeState(getScopedWorkspacePresentationMode(conversationId));
  }, [mainConversationId]);

  if (authState === 'checking') {
    return (
      <div className="app-shell" style={{ display: 'flex', minHeight: '100vh', alignItems: 'center', justifyContent: 'center', background: '#f8fafc', color: '#64748b', fontSize: 15 }}>
        Checking authentication…
      </div>
    );
  }

  if (authState === 'required') {
    return (
      <div className="app-shell" style={{ display: 'flex', minHeight: '100vh', alignItems: 'center', justifyContent: 'center', background: '#f8fafc' }}>
        <div style={{ width: 'min(480px, 92vw)', padding: 28, borderRadius: 16, background: '#fff', boxShadow: '0 12px 36px rgba(15,23,42,0.10)', border: '1px solid #e5e7eb', textAlign: 'center' }}>
          <div style={{ fontSize: 22, fontWeight: 600, color: '#0f172a', marginBottom: 10 }}>Sign in required</div>
          <div style={{ color: '#64748b', lineHeight: 1.5, marginBottom: 18 }}>
            This workspace requires OAuth authentication before conversations, approvals, and chat windows can load.
          </div>
          <button
            type="button"
            onClick={() => beginLogin()}
            style={{ padding: '10px 18px', borderRadius: 10, border: '1px solid #d1d5db', background: '#1e293b', color: '#fff', fontWeight: 500, cursor: 'pointer', fontSize: 14 }}
          >
            {oauthProviderLabel ? `Sign in with ${oauthProviderLabel}` : 'Sign in'}
          </button>
        </div>
      </div>
    );
  }

  return (
    <DetailContext.Provider value={value}>
      <ConversationViewContext.Provider value={{ showExecutionDetails, setShowExecutionDetails, showIntakeDetails, setShowIntakeDetails, toolFeedDock: showChatChrome ? 'right' : 'inline' }}>
        <div
          className="app-shell"
          style={{
            '--app-sidebar-width': `${isSidebarOpen ? clampSidebarWidth(sidebarWidth) : 64}px`
          }}
        >
          <MenuBar
            approvals={approvals}
            onToggleSidebar={() => setIsSidebarOpen((open) => !open)}
            showExecutionDetails={showExecutionDetails}
            onToggleExecutionDetails={() => setShowExecutionDetails((value) => !value)}
            showIntakeDetails={showIntakeDetails}
            onToggleIntakeDetails={() => setShowIntakeDetails((value) => !value)}
            showWorkspaceWindow={showWorkspaceWindow}
            onToggleWorkspaceWindow={() => setShowWorkspaceWindow((value) => !value)}
            showToolFeeds={showToolFeeds}
            onToggleToolFeeds={() => setShowToolFeeds((value) => !value)}
          />

        <div className="app-main">
          <Sidebar collapsed={!isSidebarOpen} />
          {isSidebarOpen ? (
            <div
              className="app-sidebar-resizer"
              role="separator"
              aria-orientation="vertical"
              aria-label="Resize sidebar"
              onPointerDown={(event) => {
                resizeStateRef.current = {
                  startX: Number(event.clientX || 0),
                  startWidth: clampSidebarWidth(sidebarWidth)
                };
                try { document.body.style.cursor = 'col-resize'; } catch (_) {}
                try { document.body.style.userSelect = 'none'; } catch (_) {}
              }}
            />
          ) : null}
          <main className={`app-chat-pane${showChatChrome ? ' is-chat-main-window' : ''}`}>
            <div className={`app-chat-layout${showChatChrome ? ' has-tool-workspace' : ''}`}>
            <div className="app-chat-content-column" style={{ flex: 1, minHeight: 0, overflow: 'visible', display: 'flex', flexDirection: 'column' }}>
              {shouldShowMainWindowHeader(selectedWindow) && !activeWorkspaceWindow ? (
                <div className="app-main-window-header">
                  <button
                    type="button"
                    className="app-main-window-header-close"
                    aria-label={linkedChildWindow ? 'Close linked conversation' : `Close ${activeWindowTitle}`}
                    title={linkedChildWindow ? 'Close linked conversation' : `Close ${activeWindowTitle}`}
                    onClick={() => {
                      if (linkedChildWindow?.windowId) {
                        returnToParentConversation(linkedChildWindow, { closeCurrent: true });
                        return;
                      }
                      const restoreConversationId = resolveMainWindowCloseConversationId(
                        getScopedConversationSelection(MAIN_CHAT_WINDOW_ID)
                      );
                      if (selectedWindow?.windowId) {
                        removeWindow(selectedWindow.windowId);
                      }
                      openConversationInMainWindow(restoreConversationId);
                    }}
                  >
                    <span aria-hidden="true" className="app-main-window-header-close-dot" />
                  </button>
                  <div className="app-main-window-header-title">{linkedChildWindow ? 'Linked conversation' : activeWindowTitle}</div>
                </div>
              ) : null}
              {shouldRenderSplitShell ? (
                <div className={`app-window-split-stack${isWorkspaceFull ? ' is-full' : ''}${isWorkspaceCollapsed ? ' is-collapsed' : ''}${!showWorkspacePane ? ' is-chat-only' : ''}`}>
                  <div
                    className={`app-window-split-shell${isWorkspaceFull ? ' is-full' : ''}${isWorkspaceCollapsed ? ' is-collapsed' : ''}${!showWorkspacePane ? ' is-chat-only' : ''}`}
                    style={{
                      ...(workspaceSharePct > 0 ? { '--app-workspace-share': `${workspaceSharePct}%` } : {}),
                      ...(workspaceMinHeight > 0 ? { '--app-workspace-min-height': `${workspaceMinHeight}px` } : {}),
                    }}
                  >
                  {showWorkspacePane ? (
                    <section
                      key="workspace"
                      className="app-window-split-workspace"
                      aria-label={`${activeWorkspaceTitle} workspace`}
                      aria-expanded={!isWorkspaceCollapsed}
                      data-workspace-window-id={String(activeWorkspaceWindow?.windowId || '')}
                      data-workspace-window-key={String(activeWorkspaceWindow?.windowKey || '')}
                      data-workspace-region="chat.top"
                      data-workspace-collapsed={isWorkspaceCollapsed ? 'true' : 'false'}
                    >
                      <div className="app-window-split-workspace-header">
                        <div className="app-window-split-workspace-dots" aria-label="Workspace window controls">
                          <button
                            type="button"
                            className="app-window-dot app-window-dot-close"
                            aria-label={`Close ${activeWorkspaceTitle}`}
                            title="Close workspace"
                            onClick={closeActiveWorkspaceWindow}
                          />
                          <button
                            type="button"
                            className="app-window-dot app-window-dot-collapse"
                            aria-label={isWorkspaceCollapsed ? `Restore split view for ${activeWorkspaceTitle}` : `Collapse ${activeWorkspaceTitle}`}
                            title={isWorkspaceCollapsed ? 'Restore split workspace' : 'Collapse workspace body'}
                            onClick={() => setActiveWorkspaceCollapsed(!isWorkspaceCollapsed)}
                          />
                          <button
                            type="button"
                            className="app-window-dot app-window-dot-expand"
                            aria-label={isWorkspaceFull ? `Restore split view for ${activeWorkspaceTitle}` : `Expand ${activeWorkspaceTitle}`}
                            title={isWorkspaceFull ? 'Restore split workspace' : 'Expand workspace'}
                            onClick={() => {
                              setActiveWorkspaceCollapsed(false);
                              setWorkspacePresentationMode(isWorkspaceFull ? 'split' : 'full');
                            }}
                          />
                        </div>
                        <div className="app-window-split-workspace-title">{activeWorkspaceTitle}</div>
                      </div>
                      {!isWorkspaceCollapsed && workspaceTabs.length > 1 ? (
                        <div className="app-window-split-workspace-tabs" role="tablist" aria-label="Workspace compare tabs">
                          {workspaceTabs.map((tab) => (
                            <button
                              key={tab.windowId}
                              type="button"
                              role="tab"
                              aria-selected={tab.isActive}
                              className={`app-window-split-workspace-tab${tab.isActive ? ' is-active' : ''}`}
                              onClick={() => focusWorkspaceWindow(tab.windowId)}
                            >
                              {tab.label}
                            </button>
                          ))}
                        </div>
                      ) : null}
                      <div className="app-window-split-workspace-body" hidden={isWorkspaceCollapsed}>
                        <WindowContent window={activeWorkspaceWindow} isInTab />
                      </div>
                    </section>
                  ) : null}
                  <section
                    key="chat"
                    className="app-window-split-chat"
                    aria-label={shouldShowChatChrome(hostedBottomWindow) ? 'Conversation' : `${resolveMainWindowHeaderTitle(hostedBottomWindow)} panel`}
                  >
                    <WindowContent key={String(hostedBottomWindow?.windowId || 'chat')} window={hostedBottomWindow} isInTab />
                  </section>
                  </div>
                </div>
              ) : (
                <WindowManager />
              )}
            </div>
            {showChatChrome && showToolFeeds ? <ToolFeedWorkspace conversationId={activeConversationId} /> : null}
            </div>
            {showChatChrome ? <UsageBar /> : null}
          </main>
        </div>

          <StatusBar backendUnavailable={!!approvals?.backendUnavailable} approvals={approvals} />
        </div>
      </ConversationViewContext.Provider>

      <Dialog
        isOpen={isPanelOpen}
        onClose={value.closeDetail}
        style={{ width: '70vw', minWidth: 900, maxWidth: '95vw' }}
      >
        <div className="app-detail-dialog-shell">
          <DetailPanel
            toolCall={selectedTool}
            mode="dialog"
            onClose={value.closeDetail}
            onUndock={() => setMode('window')}
            onDockRight={() => setMode('right')}
            onDockLeft={() => setMode('left')}
          />
        </div>
      </Dialog>

      {isPanelOpen && detailMode === 'window' ? (
        <DetailPopoutWindow title="Execution Detail" onClose={value.closeDetail}>
          <div className="app-detail-popout-shell">
            <DetailPanel
              toolCall={selectedTool}
              mode="window"
              onClose={value.closeDetail}
              onUndock={() => setMode('window')}
              onDockRight={() => setMode('right')}
              onDockLeft={() => setMode('left')}
            />
          </div>
        </DetailPopoutWindow>
      ) : null}
      <CodeDiffDialog />
      <ConfirmDialog />
      <FileViewDialog />
      <ElicitationOverlay context={null} />
    </DetailContext.Provider>
  );
}
