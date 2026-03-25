import React, { useCallback, useEffect, useState } from 'react';
import { Button, Dialog } from '@blueprintjs/core';
import { addWindow, activeWindows, getWindowContext, selectedTabId, selectedWindowId } from 'forge/core';
import { client } from '../services/agentlyClient';
import logo from '../viant-logo.png';

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

export function openWindow(windowKey, windowTitle, refreshDataSources = []) {
  const windows = Array.isArray(activeWindows.peek?.()) ? activeWindows.peek() : [];
  let existing = windows.find((entry) => entry?.windowKey === windowKey);
  if (!existing) {
    existing = addWindow(windowTitle, null, windowKey, null, true, {}, { autoIndexTitle: false });
  }
  if (existing?.windowId) {
    selectedTabId.value = existing.windowId;
    selectedWindowId.value = existing.windowId;
    refreshWindowDataSources(existing.windowId, refreshDataSources);
  }
}

export default function MenuBar({ approvals, onToggleSidebar }) {
  const {
    items = [],
    pendingCount = 0,
    open,
    selected,
    setOpen,
    setSelected,
    decide
  } = approvals || {};
  const [user, setUser] = useState(null);
  const [userMenuOpen, setUserMenuOpen] = useState(false);

  useEffect(() => {
    client.getAuthMe().then((me) => {
      if (me) {
        setUser(me);
        if (me.username || me.subject) {
          try { window.localStorage?.setItem('agently.userName', me.username || me.subject); } catch (_) {}
        }
      }
    }).catch(() => {});
  }, []);

  const displayName = user?.displayName || user?.username || user?.email || user?.subject || '';

  const handleLogout = useCallback(async () => {
    setUserMenuOpen(false);
    try {
      await client.logout();
    } catch (_) {}
    try { window.localStorage?.removeItem('agently.userName'); } catch (_) {}
    window.location.reload();
  }, []);

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
            onClick={() => openWindow('schedule', 'Automation', ['schedules'])}
          />
          <Button
            minimal
            icon="history"
            text="Runs"
            className="app-topbar-nav-btn"
            data-testid="runs-nav"
            onClick={() => openWindow('schedule/history', 'Runs', ['runs'])}
          />
          <Button
            minimal
            icon="notifications"
            intent={pendingCount > 0 ? 'warning' : 'none'}
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
          ) : items.map((item) => (
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
      ) : null}

      <Dialog isOpen={!!selected} onClose={() => setSelected?.(null)} title="Approval detail">
        <div className="app-approval-dialog">
          <div><strong>Tool:</strong> {selected?.toolName}</div>
          <pre>{JSON.stringify(selected?.arguments || {}, null, 2)}</pre>
          <div className="app-approval-dialog-actions">
            <Button intent="danger" onClick={() => decide?.(selected, 'reject')}>Reject</Button>
            <Button intent="primary" onClick={() => decide?.(selected, 'approve')}>Approve</Button>
          </div>
        </div>
      </Dialog>
    </header>
  );
}
