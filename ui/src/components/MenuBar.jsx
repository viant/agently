import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Dialog } from '@blueprintjs/core';
import { addWindow, activeWindows, getWindowContext, selectedTabId, selectedWindowId } from 'forge/core';
import { client } from '../services/agentlyClient';
import logo from '../viant-logo.png';

export function resolveStartupAuthAction(providers) {
  const normalized = Array.isArray(providers) ? providers : [];
  const hasOAuthProvider = normalized.some((entry) => {
    const type = String(entry?.type || '').trim().toLowerCase();
    return type === 'bff' || type === 'oidc' || type === 'jwt';
  });
  if (hasOAuthProvider) {
    return { type: 'oauth' };
  }
  const localProvider = normalized.find((entry) => String(entry?.type || '').trim().toLowerCase() === 'local' && String(entry?.defaultUsername || '').trim());
  if (!localProvider) {
    return { type: 'none' };
  }
  return { type: 'local', username: String(localProvider.defaultUsername || '').trim() };
}

export function refreshWindowDataSources(windowId, dataSourceRefs = []) {
  if (!windowId || !Array.isArray(dataSourceRefs) || dataSourceRefs.length === 0) return;
  const base = getWindowContext?.(windowId);
  if (!base?.Context) return;
  dataSourceRefs.forEach((dataSourceRef) => {
    if (!dataSourceRef) return;
    try {
      base.Context(dataSourceRef)?.handlers?.dataSource?.fetchCollection?.();
    } catch (_) {}
  });
}

export function openWindow(windowKey, windowTitle, refreshDataSources = [], options = {}) {
  const windows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  const replaceTabbedWindows = options?.replaceTabbedWindows === true;
  let existing = windows.find((entry) => entry?.windowKey === windowKey);
  if (replaceTabbedWindows) {
    const keepWindowId = String(existing?.windowId || '').trim();
    activeWindows.value = windows.filter((entry) => {
      if (entry?.inTab === false) return true;
      if (keepWindowId && String(entry?.windowId || '').trim() === keepWindowId) return true;
      return false;
    });
  }
  const currentWindows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  existing = currentWindows.find((entry) => entry?.windowKey === windowKey);
  if (!existing) {
    existing = addWindow(windowTitle, null, windowKey, null, true, {}, { autoIndexTitle: false });
  }
  if (existing?.windowId) {
    selectedTabId.value = existing.windowId;
    selectedWindowId.value = existing.windowId;
    refreshWindowDataSources(existing.windowId, refreshDataSources);
  }
}

export const APPROVALS_PAGE_SIZE = 8;

export function paginateApprovalItems(items = [], page = 0, pageSize = APPROVALS_PAGE_SIZE) {
  const list = Array.isArray(items) ? items : [];
  const normalizedPageSize = Number.isFinite(Number(pageSize)) && Number(pageSize) > 0
    ? Math.max(1, Math.floor(Number(pageSize)))
    : APPROVALS_PAGE_SIZE;
  const pageCount = Math.max(1, Math.ceil(list.length / normalizedPageSize));
  const safePage = Math.min(Math.max(0, Math.floor(Number(page) || 0)), pageCount - 1);
  const start = safePage * normalizedPageSize;
  const end = Math.min(list.length, start + normalizedPageSize);
  return {
    items: list.slice(start, end),
    page: safePage,
    pageCount,
    start,
    end,
    total: list.length,
    hasPrevious: safePage > 0,
    hasNext: safePage < pageCount - 1
  };
}

export default function MenuBar({ approvals, onToggleSidebar }) {
  const {
    items = [],
    pendingCount = 0,
    page: approvalsPage = 0,
    pageCount: approvalsPageCount = 1,
    start: approvalsStart = 0,
    end: approvalsEnd = 0,
    hasPrevious: approvalsHasPrevious = false,
    hasNext: approvalsHasNext = false,
    open,
    selected,
    setOpen,
    setPage,
    setSelected,
    decide
  } = approvals || {};
  const [user, setUser] = useState(null);
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const [approvalPage, setApprovalPage] = useState(0);

  useEffect(() => {
    let mounted = true;
    const loadUser = async () => {
      try {
        const me = await client.getAuthMe();
        if (mounted) setUser(me || null);
        if (me) return;
        const providers = await client.getAuthProviders();
        const action = resolveStartupAuthAction(providers);
        if (action.type !== 'local' || !action.username) return;
        await client.localLogin({ username: action.username });
        const autoMe = await client.getAuthMe();
        if (!mounted || !autoMe) return;
        setUser(autoMe);
        try { window.dispatchEvent(new CustomEvent('forge:conversation-active', { detail: { id: '' } })); } catch (_) {}
        try { window.location.reload(); } catch (_) {}
      } catch (_) {}
    };
    const onAuthorized = () => { void loadUser(); };
    void loadUser();
    if (typeof window !== 'undefined') {
      window.addEventListener('agently:authorized', onAuthorized);
    }
    return () => {
      mounted = false;
      if (typeof window !== 'undefined') {
        window.removeEventListener('agently:authorized', onAuthorized);
      }
    };
  }, []);

  const displayName = user?.displayName || user?.username || user?.email || user?.subject || '';

  const approvalPageData = useMemo(() => {
    if (typeof setPage === 'function') {
      return {
        items,
        page: approvalsPage,
        pageCount: approvalsPageCount,
        start: Math.max(0, approvalsStart > 0 ? approvalsStart - 1 : 0),
        end: Math.max(0, approvalsEnd),
        total: pendingCount,
        hasPrevious: approvalsHasPrevious,
        hasNext: approvalsHasNext
      };
    }
    return paginateApprovalItems(items, approvalPage, APPROVALS_PAGE_SIZE);
  }, [items, approvalPage, approvalsPage, approvalsPageCount, approvalsStart, approvalsEnd, pendingCount, approvalsHasPrevious, approvalsHasNext, setPage]);

  useEffect(() => {
    setApprovalPage((current) => {
      const maxPage = Math.max(0, approvalPageData.pageCount - 1);
      return current > maxPage ? maxPage : current;
    });
  }, [approvalPageData.pageCount]);

  useEffect(() => {
    if (!open) {
      setApprovalPage(0);
      setPage?.(0);
    }
  }, [open, setPage]);

  const handleLogout = useCallback(async () => {
    setUserMenuOpen(false);
    try {
      await client.logout();
    } catch (_) {}
    try { window.localStorage?.removeItem('agently.userName'); } catch (_) {}
    window.location.reload();
  }, []);

  const handleApprovalDecision = useCallback(async (item, action) => {
    if (!item || !decide) return;
    return decide(item, action);
  }, [decide]);

  return (
    <header className="app-topbar">
      <div className="app-topbar-left">
        <div className="app-brand">
          <img src={logo} alt="Viant" className="app-brand-logo" />
          <span className="app-brand-name">Agently</span>
        </div>
        <div className="app-topbar-divider" />
        <div className="app-topbar-actions">
          <Button minimal icon="menu" data-testid="sidebar-toggle" onClick={onToggleSidebar} />
          <Button
            minimal
            icon="time"
            text="Automation"
            className="app-topbar-nav-btn"
            data-testid="automation-nav"
            onClick={() => openWindow('schedule', 'Automation', ['schedules'], { replaceTabbedWindows: true })}
          />
          <Button
            minimal
            icon="history"
            text="Runs"
            className="app-topbar-nav-btn"
            data-testid="runs-nav"
            onClick={() => openWindow('schedule/history', 'Runs', ['runs'], { replaceTabbedWindows: true })}
          />
          <Button
            minimal
            icon="notifications"
            intent={pendingCount > 0 ? 'warning' : 'none'}
            className={pendingCount > 0 ? 'app-approval-bell is-pending' : 'app-approval-bell'}
            data-testid="approval-bell"
            onClick={() => setOpen?.(!open)}
          />
        </div>
      </div>
      <div className="app-topbar-right" style={{ position: 'relative' }}>
        <Button
          minimal
          icon="user"
          className="app-user-btn"
          data-testid="user-menu-btn"
          onClick={() => setUserMenuOpen((v) => !v)}
        >
          {displayName}
        </Button>
        {userMenuOpen ? (
          <div className="app-user-menu" data-testid="user-menu">
            {user?.email ? <div className="app-user-menu-email">{user.email}</div> : null}
            <Button
              minimal
              icon="log-out"
              text="Logout"
              className="app-user-menu-logout"
              data-testid="logout-btn"
              onClick={handleLogout}
            />
          </div>
        ) : null}
      </div>

      {open ? (
        <div className="app-approval-popover app-approval-popover-left">
          {items.length === 0 ? (
            <div className="app-approval-empty">No pending approvals</div>
          ) : (
            <>
              <div className="app-approval-list">
                {approvalPageData.items.map((item) => (
                  <button
                    key={item.id}
                    className="app-approval-row"
                    onClick={() => setSelected?.(item)}
                  >
                    <div className="app-approval-title">{item.title || item.toolName || item.id}</div>
                    <div className="app-approval-subtitle">{item.status}</div>
                  </button>
                ))}
              </div>
              {approvalPageData.total > APPROVALS_PAGE_SIZE ? (
                <div className="app-approval-pagination">
                  <Button
                    small
                    minimal
                    disabled={!approvalPageData.hasPrevious}
                    onClick={() => {
                      if (typeof setPage === 'function') {
                        setPage(Math.max(0, approvalPageData.page - 1));
                        return;
                      }
                      setApprovalPage((current) => Math.max(0, current - 1));
                    }}
                  >
                    Previous
                  </Button>
                  <div className="app-approval-pagination-status">
                    {approvalPageData.total > 0 ? `${approvalPageData.start + 1}-${approvalPageData.end} of ${approvalPageData.total}` : '0 of 0'}
                  </div>
                  <Button
                    small
                    minimal
                    disabled={!approvalPageData.hasNext}
                    onClick={() => {
                      if (typeof setPage === 'function') {
                        setPage(approvalPageData.page + 1);
                        return;
                      }
                      setApprovalPage((current) => current + 1);
                    }}
                  >
                    Next
                  </Button>
                </div>
              ) : null}
            </>
          )}
        </div>
      ) : null}

      <Dialog
        isOpen={!!selected}
        onClose={() => setSelected?.(null)}
        title={selected?.title || selected?.toolName || 'Approval detail'}
      >
        <div className="app-approval-dialog">
          <div><strong>Tool:</strong> {selected?.toolName}</div>
          <pre>{JSON.stringify(selected?.arguments || {}, null, 2)}</pre>
          <div className="app-approval-dialog-actions">
            <Button intent="danger" onClick={() => handleApprovalDecision(selected, 'reject')}>Reject</Button>
            <Button
              intent="primary"
              onClick={() => handleApprovalDecision(selected, 'approve')}
            >
              Approve
            </Button>
          </div>
        </div>
      </Dialog>
    </header>
  );
}
