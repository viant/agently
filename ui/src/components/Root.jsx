import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useSignals } from '@preact/signals-react/runtime';
import { Dialog } from '@blueprintjs/core';
import { activeWindows, addWindow, findCollectionSignal, findFormSignal, findMetadataSignal, findMetricsSignal, findViewSignal, removeWindow, selectedTabId, selectedWindowId } from 'forge/core';
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
import { CHAT_WINDOW_KEY, MAIN_CHAT_WINDOW_ID, ensureWorkspaceWindowForConversation, getScopedConversationSelection, getScopedWorkspacePresentationMode, getSelectedWindow, hasScopedWorkspaceState, isLinkedChildWindow, openConversationInMainWindow, reopenWorkspaceForConversation, requestNewConversationInMainWindow, resolveConversationSelection, resolveWorkspaceWindowForConversation, resolveWorkspaceWindowsForConversation, returnToParentConversation, setScopedWorkspacePresentationMode, setScopedWorkspaceSelection, setScopedWorkspaceState } from '../services/conversationWindow';
import { AGENTLY_UI_BUILD } from '../buildInfo';
import { conversationIDFromPath, publishActiveConversation } from '../services/chatRuntime';
import { beginLogin, getAuthMeSilently, getAuthProvidersSilently, recoverSessionSilently } from '../services/agentlyClient';

const SIDEBAR_WIDTH_KEY = 'agently.sidebarWidth';
const SIDEBAR_DEFAULT_WIDTH = 320;
const SIDEBAR_MIN_WIDTH = 220;
const SIDEBAR_MAX_WIDTH = 520;
const SHOW_EXECUTION_DETAILS_KEY = 'agently.showExecutionDetails';
const SHOW_INTAKE_DETAILS_KEY = 'agently.showIntakeDetails';
const SHOW_WORKSPACE_WINDOW_KEY = 'agently.showWorkspaceWindow';
const SHOW_TOOL_FEEDS_KEY = 'agently.showToolFeeds';
const WORKSPACE_HEIGHT_KEY = 'agently.workspaceHeight';
const WORKSPACE_DEFAULT_HEIGHT = 620;
const WORKSPACE_MIN_HEIGHT = 240;
const WORKSPACE_MAX_HEIGHT = 960;

function clampSidebarWidth(value) {
  const next = Number(value || 0);
  if (!Number.isFinite(next)) return SIDEBAR_DEFAULT_WIDTH;
  return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(next)));
}

function clampWorkspaceHeight(value) {
  const next = Number(value || 0);
  if (!Number.isFinite(next)) return WORKSPACE_DEFAULT_HEIGHT;
  return Math.min(WORKSPACE_MAX_HEIGHT, Math.max(WORKSPACE_MIN_HEIGHT, Math.round(next)));
}

function workspaceHeightStorageKey(conversationId = '') {
  const id = String(conversationId || '').trim();
  return id ? `${WORKSPACE_HEIGHT_KEY}:${id}` : WORKSPACE_HEIGHT_KEY;
}

function resolveActivatedWorkspaceHeight() {
  if (typeof document === 'undefined') return WORKSPACE_DEFAULT_HEIGHT;
  const shellHeight = Number(
    document.querySelector('.app-chat-content-column')?.clientHeight
    || document.querySelector('.app-window-split-stack')?.clientHeight
    || document.documentElement?.clientHeight
    || 0
  );
  if (!(shellHeight > 0)) return WORKSPACE_DEFAULT_HEIGHT;
  return clampWorkspaceHeight(Math.round(shellHeight * (2 / 3)));
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

function attachResolvedMetrics(windowEntry = null) {
  return windowEntry;
}

function resolvePathValue(holder, selector = '') {
  const path = String(selector || '').trim();
  if (!path) return holder;
  return path.split('.').reduce((acc, part) => {
    if (acc == null || typeof acc !== 'object') return undefined;
    return acc[part];
  }, holder);
}

function resolveMetadataBoundWindowTitle(windowEntry = null) {
  const windowId = String(windowEntry?.windowId || '').trim();
  if (!windowId) return '';
  try {
    const metadata = findMetadataSignal(windowId)?.value || null;
    const binding = metadata?.window?.titleBinding || metadata?.view?.titleBinding;
    if (!binding) return '';
    const dataSourceRef = String(binding?.dataSourceRef || binding?.ref || '').trim();
    const selector = String(binding?.selector || binding?.field || '').trim();
    const source = String(binding?.source || 'metrics').trim().toLowerCase();
    if (!dataSourceRef) return '';
    const dataSourceId = `${windowId}DS${dataSourceRef}`;
    let data = null;
    switch (source) {
      case 'collection':
      case 'data': {
        const collection = findCollectionSignal(dataSourceId)?.value;
        data = Array.isArray(collection) ? (collection[0] || null) : collection;
        break;
      }
      case 'metrics':
      default:
        data = findMetricsSignal(dataSourceId)?.value || null;
        if ((data == null || (typeof data === 'object' && Object.keys(data).length === 0))) {
          const collection = findCollectionSignal(dataSourceId)?.value;
          data = Array.isArray(collection) ? (collection[0] || null) : collection;
        }
        break;
    }
    const resolved = resolvePathValue(data, selector);
    if (resolved != null && String(resolved).trim() !== '') {
      return String(resolved).trim();
    }
    if (typeof document !== 'undefined') {
      const domSelector = String(binding?.domSelector || binding?.selectorCss || '').trim();
      const controlId = String(binding?.controlId || binding?.domControlId || '').trim();
      if (domSelector || controlId) {
        try {
          const base = document.querySelector(`[data-workspace-window-id="${windowId}"]`);
          const node = domSelector
            ? base?.querySelector?.(domSelector)
            : base?.querySelector?.(`[data-forge-control-id="${controlId}"]`);
          const text = String(node?.textContent || '').trim();
          if (text) return text;
        } catch (_) {}
      }
    }
    return '';
  } catch (_) {
    return '';
  }
}

export function resolveHostedWorkspaceTabLabel(windowEntry = null) {
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
  const boundTitle = resolveMetadataBoundWindowTitle(windowEntry);
  if (boundTitle) return boundTitle;
  const explicitTitle = String(windowEntry?.windowTitle || '').trim();
  if (explicitTitle) return explicitTitle;
  return String(windowEntry?.windowKey || '').trim();
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

export function resolveWindowRefreshDataSources(windowKey = '') {
  switch (String(windowKey || '').trim()) {
    case 'schedule':
      return ['schedules'];
    case 'schedule/history':
      return ['runs'];
    case 'recommendationList':
      return ['recommendation_list'];
    case 'recommendation':
    case 'recommendationReview':
      return ['recommendation'];
    default:
      return [];
  }
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

export function shouldReplayRouteConversationBootstrap({
  pathname = '',
  authState = '',
  mainConversationId = '',
  hasChatFeed = false,
  hasWorkspace = false,
} = {}) {
  if (authState !== 'ready') return false;
  const routeConversationId = String(conversationIDFromPath(pathname) || '').trim();
  if (!routeConversationId) return false;
  const selectedConversationId = String(mainConversationId || '').trim();
  if (selectedConversationId && selectedConversationId !== routeConversationId) return false;
  return !hasChatFeed && !hasWorkspace;
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
  const [workspaceHeight, setWorkspaceHeight] = useState(WORKSPACE_DEFAULT_HEIGHT);
  const [stableMainChatWindow, setStableMainChatWindow] = useState(null);
  const resizeStateRef = useRef(null);
  const workspaceResizeStateRef = useRef(null);
  const approvals = useApprovalQueue(authState === 'ready');
  const selectedWindow = resolveSelectedMainWindow(
    activeWindows.value,
    selectedTabId.value,
    selectedWindowId.value,
    getSelectedWindow()
  );
  const selectedWindowForTitle = attachResolvedMetrics(selectedWindow);
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
    () => resolveWorkspaceWindowsForConversation(mainConversationId).map((entry) => attachResolvedMetrics(entry)),
    [mainConversationId, activeWindows.value, selectedWindowId.value, selectedTabId.value]
  );
  const workspaceStatePersistenceSignature = workspaceWindows.map((entry) => {
    const windowId = String(entry?.windowId || '').trim();
    const hasInlineMetadata = entry?.inlineMetadata && typeof entry.inlineMetadata === 'object';
    const inlineNamespace = hasInlineMetadata ? String(entry.inlineMetadata.namespace || '').trim() : '';
    const windowFormState = windowId ? (findFormSignal(`${windowId}:windowForm`)?.value || {}) : {};
    const viewState = windowId ? (findViewSignal(windowId)?.value || {}) : {};
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
          ? attachResolvedMetrics(selectedWindow)
          : (isWorkspaceRegionWindow(resolvedConversationWorkspaceWindow) ? attachResolvedMetrics(resolvedConversationWorkspaceWindow) : null)
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
    Promise.allSettled([getAuthProvidersSilently(), getAuthMeSilently()])
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
    if (authState !== 'required' || typeof window === 'undefined') {
      return () => {};
    }
    let cancelled = false;
    const recheck = async () => {
      try {
        const recovered = await recoverSessionSilently();
        if (cancelled) return;
        if (recovered) {
          setAuthState('ready');
        }
      } catch (_) {}
    };
    const onFocus = () => {
      void recheck();
    };
    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        void recheck();
      }
    };
    const timer = window.setTimeout(() => {
      void recheck();
    }, 300);
    const interval = window.setInterval(() => {
      void recheck();
    }, 2000);
    window.addEventListener('focus', onFocus);
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
      window.clearInterval(interval);
      window.removeEventListener('focus', onFocus);
      document.removeEventListener('visibilitychange', onVisibility);
    };
  }, [authState]);

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
      const sidebarState = resizeStateRef.current;
      if (sidebarState) {
        const delta = Number(event.clientX || 0) - sidebarState.startX;
        setSidebarWidth(clampSidebarWidth(sidebarState.startWidth + delta));
        return;
      }
      const workspaceState = workspaceResizeStateRef.current;
      if (!workspaceState) return;
      const delta = Number(event.clientY || 0) - workspaceState.startY;
      setWorkspaceHeight(clampWorkspaceHeight(workspaceState.startHeight + delta));
    };

    const stopResize = () => {
      const wasResizing = !!resizeStateRef.current || !!workspaceResizeStateRef.current;
      if (!wasResizing) return;
      resizeStateRef.current = null;
      workspaceResizeStateRef.current = null;
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
    if (typeof window === 'undefined') return;
    const conversationId = String(mainConversationId || '').trim();
    try {
      const raw = window.sessionStorage?.getItem(workspaceHeightStorageKey(conversationId));
      setWorkspaceHeight(clampWorkspaceHeight(raw));
    } catch (_) {
      setWorkspaceHeight(WORKSPACE_DEFAULT_HEIGHT);
    }
  }, [mainConversationId]);

  useEffect(() => {
    const requiredMinHeight = Number(activeWorkspaceWindow?.workspaceMinHeight || 0);
    if (!(requiredMinHeight > 0)) return;
    setWorkspaceHeight((current) => {
      const normalizedCurrent = clampWorkspaceHeight(current);
      const normalizedRequired = clampWorkspaceHeight(requiredMinHeight);
      return normalizedCurrent >= normalizedRequired ? normalizedCurrent : normalizedRequired;
    });
  }, [activeWorkspaceWindow?.windowId, activeWorkspaceWindow?.workspaceMinHeight]);

  useEffect(() => {
    const windowId = String(activeWorkspaceWindow?.windowId || '').trim();
    if (!windowId) return;
    if (isWorkspaceFull || isWorkspaceCollapsed) return;
    const activatedHeight = resolveActivatedWorkspaceHeight();
    const requiredMinHeight = Number(activeWorkspaceWindow?.workspaceMinHeight || 0);
    const desired = Math.max(
      clampWorkspaceHeight(activatedHeight),
      requiredMinHeight > 0 ? clampWorkspaceHeight(requiredMinHeight) : WORKSPACE_MIN_HEIGHT,
    );
    setWorkspaceHeight(desired);
  }, [activeWorkspaceWindow?.windowId, activeWorkspaceWindow?.workspaceMinHeight, isWorkspaceFull, isWorkspaceCollapsed]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const conversationId = String(mainConversationId || '').trim();
    try {
      window.sessionStorage?.setItem(
        workspaceHeightStorageKey(conversationId),
        String(clampWorkspaceHeight(workspaceHeight))
      );
    } catch (_) {}
  }, [mainConversationId, workspaceHeight]);

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
    if (!windowId) return;
    const refreshRefs = resolveWindowRefreshDataSources(selectedWindow?.windowKey);
    if (refreshRefs.length === 0) return;
    const timer = window.setTimeout(() => {
      refreshWindowDataSources(windowId, refreshRefs);
    }, 0);
    return () => window.clearTimeout(timer);
  }, [selectedWindow?.windowId, selectedWindow?.windowKey]);

  useEffect(() => {
    const windowId = String(activeWorkspaceWindow?.windowId || '').trim();
    if (!windowId || windowId === String(selectedWindow?.windowId || '').trim()) return;
    const refreshRefs = resolveWindowRefreshDataSources(activeWorkspaceWindow?.windowKey);
    if (refreshRefs.length === 0) return;
    const timer = window.setTimeout(() => {
      refreshWindowDataSources(windowId, refreshRefs);
    }, 0);
    return () => window.clearTimeout(timer);
  }, [activeWorkspaceWindow?.windowId, activeWorkspaceWindow?.windowKey, selectedWindow?.windowId]);

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
    if (!shouldReplayRouteConversationBootstrap({
      pathname: window.location.pathname,
      authState,
      mainConversationId,
      hasChatFeed: !!document.querySelector('.app-chat-feed'),
      hasWorkspace: !!document.querySelector('[data-workspace-window-id]'),
    })) {
      return () => {};
    }
    const routeConversationId = conversationIDFromPath(window.location.pathname);
    if (!routeConversationId) return () => {};
    const replay = () => {
      openConversationInMainWindow(routeConversationId);
    };
    const timer = window.setTimeout(replay, 250);
    const timer2 = window.setTimeout(replay, 1200);
    return () => {
      window.clearTimeout(timer);
      window.clearTimeout(timer2);
    };
  }, [authState, mainConversationId]);

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
    const conversationId = String(mainConversationId || '').trim();
    if (!conversationId) return () => {};
    if (linkedChildWindow?.windowId) return () => {};
    if (activeWorkspaceWindow?.windowId) return () => {};
    let cancelled = false;
    const restoreIfNeeded = () => {
      if (cancelled) return;
      if (!hasScopedWorkspaceState(conversationId)) return;
      reopenWorkspaceForConversation(conversationId);
    };
    const timer = window.setTimeout(restoreIfNeeded, 250);
    const interval = window.setInterval(restoreIfNeeded, 1500);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
      window.clearInterval(interval);
    };
  }, [activeWorkspaceWindow?.windowId, linkedChildWindow?.windowId, mainConversationId]);

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

  useEffect(() => {
    const conversationId = String(mainConversationId || '').trim();
    if (!conversationId) return;
    if (authState !== 'ready') return;
    const timer = window.setTimeout(() => {
      try {
        publishActiveConversation(conversationId);
      } catch (_) {}
    }, 0);
    return () => window.clearTimeout(timer);
  }, [authState, mainConversationId]);

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
                      '--app-workspace-height': `${clampWorkspaceHeight(workspaceHeight)}px`,
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
                      <div
                        className="app-window-split-workspace-body"
                        hidden={isWorkspaceCollapsed}
                        key={String(activeWorkspaceWindow?.windowId || 'workspace')}
                      >
                        <WindowContent key={String(activeWorkspaceWindow?.windowId || 'workspace')} window={activeWorkspaceWindow} isInTab />
                      </div>
                    </section>
                  ) : null}
                  {showWorkspacePane && !isWorkspaceFull && !isWorkspaceCollapsed ? (
                    <div
                      className="app-window-split-workspace-resizer"
                      role="separator"
                      aria-orientation="horizontal"
                      aria-label="Resize workspace panel"
                      onPointerDown={(event) => {
                        workspaceResizeStateRef.current = {
                          startY: Number(event.clientY || 0),
                          startHeight: clampWorkspaceHeight(workspaceHeight),
                        };
                        try { document.body.style.cursor = 'row-resize'; } catch (_) {}
                        try { document.body.style.userSelect = 'none'; } catch (_) {}
                      }}
                    />
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
