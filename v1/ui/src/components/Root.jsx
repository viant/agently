import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useSignals } from '@preact/signals-react/runtime';
import { Dialog } from '@blueprintjs/core';
import { activeWindows, addWindow, removeWindow, selectedTabId, selectedWindowId } from 'forge/core';
import { WindowManager } from 'forge/components';
import { DetailContext } from '../context/DetailContext';
import DetailPanel from './DetailPanel';
import DetailPopoutWindow from './DetailPopoutWindow';
import ChangeFeed from './ChangeFeed';
import CodeDiffDialog from './CodeDiffDialog';
import FileViewDialog from './FileViewDialog';
import MenuBar from './MenuBar';
import PlanFeed from './PlanFeed';
import ToolFeedBar from './ToolFeedBar';
import UsageBar from './UsageBar';
import StatusBar from './StatusBar';
import Sidebar from './Sidebar';
import ElicitationOverlay from './ElicitationOverlay';
import { useApprovalQueue } from '../hooks/useApprovalQueue';
import { CHAT_WINDOW_KEY, MAIN_CHAT_WINDOW_ID, getSelectedWindow, isLinkedChildWindow, returnToParentConversation } from '../services/conversationWindow';
import { AGENTLY_UI_BUILD } from '../buildInfo';

const SIDEBAR_WIDTH_KEY = 'agently.sidebarWidth';
const SIDEBAR_DEFAULT_WIDTH = 320;
const SIDEBAR_MIN_WIDTH = 220;
const SIDEBAR_MAX_WIDTH = 520;

function clampSidebarWidth(value) {
  const next = Number(value || 0);
  if (!Number.isFinite(next)) return SIDEBAR_DEFAULT_WIDTH;
  return Math.min(SIDEBAR_MAX_WIDTH, Math.max(SIDEBAR_MIN_WIDTH, Math.round(next)));
}

export default function Root() {
  useSignals();
  void selectedWindowId.value;
  void activeWindows.value;
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
  const resizeStateRef = useRef(null);
  const approvals = useApprovalQueue();
  const selectedWindow = getSelectedWindow();
  const linkedChildWindow = isLinkedChildWindow(selectedWindow) ? selectedWindow : null;

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
    if (typeof window === 'undefined') return;
    try {
      window.localStorage?.setItem(SIDEBAR_WIDTH_KEY, String(clampSidebarWidth(sidebarWidth)));
    } catch (_) {}
  }, [sidebarWidth]);

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
    if (chatWindows.length === 0) {
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

  return (
    <DetailContext.Provider value={value}>
      <div
        className="app-shell"
        style={{
          '--app-sidebar-width': `${isSidebarOpen ? clampSidebarWidth(sidebarWidth) : 64}px`
        }}
      >
        <MenuBar approvals={approvals} onToggleSidebar={() => setIsSidebarOpen((open) => !open)} />

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
          <main className="app-chat-pane">
            <div style={{ flex: 1, minHeight: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
              {linkedChildWindow ? (
                <div className="app-linked-child-banner">
                  <div className="app-linked-child-dots">
                    <button
                      type="button"
                      className="app-linked-child-dot app-linked-child-dot-close"
                      aria-label="Close linked conversation"
                      title="Close linked conversation"
                      onClick={() => removeWindow(linkedChildWindow.windowId)}
                    >
                      <span className="app-linked-child-dot-icon">×</span>
                    </button>
                    <button
                      type="button"
                      className="app-linked-child-dot app-linked-child-dot-back"
                      aria-label="Return to parent conversation"
                      title="Return to parent conversation"
                      onClick={() => returnToParentConversation(linkedChildWindow)}
                    >
                      <span className="app-linked-child-dot-icon">←</span>
                    </button>
                    <span className="app-linked-child-dot app-linked-child-dot-inert" aria-hidden="true">
                      <span className="app-linked-child-dot-icon">•</span>
                    </span>
                  </div>
                  <div className="app-linked-child-title">Linked conversation</div>
                </div>
              ) : null}
              <WindowManager />
            </div>
            <ChangeFeed anchor="composer_top" />
            <ToolFeedBar />
          </main>
          <UsageBar />
        </div>

        <StatusBar backendUnavailable={!!approvals?.backendUnavailable} approvals={approvals} />
      </div>

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
      <FileViewDialog />
      <ElicitationOverlay context={null} />
    </DetailContext.Provider>
  );
}
