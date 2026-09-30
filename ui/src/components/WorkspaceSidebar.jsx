import React, { useEffect, useRef, useState } from 'react';
import { Button, Icon } from '@blueprintjs/core';
import { sdkBaseURL } from '../endpoint';
import { getAuthMeSilently } from '../services/agentlyClient';
import Sidebar from './Sidebar';
import { openWindow } from './MenuBar';
import { MAIN_CHAT_WINDOW_ID } from '../services/conversationWindow';

const DEFAULT_SPLIT = 0.5;

export function resolveLayoutWindowKey(action = {}) {
  if (!action?.windowKey) return '';
  return action.provider && action.provider !== 'workspace'
    ? `provider:${action.provider}:${action.windowKey}` : action.windowKey;
}

export function resolveLayoutWindowOptions(action = {}, conversationId = '') {
  const id = String(conversationId || '').trim();
  const parameters = action.parameters || {};
  return { parameters, conversationId: id || undefined, parentKey: MAIN_CHAT_WINDOW_ID, presentation: 'hosted', region: 'chat.top' };
}

export default function WorkspaceSidebar({ conversationId = '', collapsed = false, onNavigate, onExpand, onOpenWorkspace, onTopbarActionsChange }) {
  const [layout, setLayout] = useState(null);
  const [activeApp, setActiveApp] = useState('');
  const [fraction, setFraction] = useState(DEFAULT_SPLIT);
  const [error, setError] = useState('');
  const [reloadVersion, setReloadVersion] = useState(0);
  const [preferenceKey, setPreferenceKey] = useState('');
  const [sidebarHeight, setSidebarHeight] = useState(0);
  const shellRef = useRef(null);

  useEffect(() => {
    const element = shellRef.current;
    if (!element || typeof ResizeObserver === 'undefined') return undefined;
    const observer = new ResizeObserver(() => setSidebarHeight(element.clientHeight));
    observer.observe(element);
    setSidebarHeight(element.clientHeight);
    return () => observer.disconnect();
  }, [layout, collapsed]);

  useEffect(() => {
    let live = true;
    const load = async () => {
      try {
        const response = await fetch(`${sdkBaseURL}/workspace/layout`, { credentials: 'include', headers: { Accept: 'application/json' } });
        if (!live) return;
        if (!response.ok) throw new Error(`Workspace layout unavailable (${response.status})`);
        const payload = await response.json();
        if (!live) return;
        const applications = Array.isArray(payload?.layout?.applications) ? payload.layout.applications : [];
        onTopbarActionsChange?.(payload?.layout?.topbar?.actions || []);
        const widthSpec = payload?.layout?.left?.width || {};
        const initialWidth = Number(widthSpec.default) || 320;
        if (widthSpec.preferenceScope === 'global') {
          const globalKey = String(widthSpec.preferenceKey || '');
          const savedWidth = Number(window.localStorage?.getItem(globalKey));
          setPreferenceKey('');
          setLayout(payload.layout);
          window.dispatchEvent(new CustomEvent('agently:layout-width', { detail: { width: savedWidth > 0 ? savedWidth : initialWidth, min: widthSpec.min, max: widthSpec.max, preferenceKey: globalKey, preferenceFormat: 'number' } }));
          setActiveApp(applications[0]?.id || '');
          setFraction(Number(payload.layout?.left?.split?.initial) || DEFAULT_SPLIT);
          setError('');
          return;
        }
        const me = await getAuthMeSilently().catch(() => null);
        if (!live) return;
        const principalId = String(me?.subject || me?.username || me?.email || me?.id || 'anonymous');
        const key = `agently.layoutPreferences.v1:${JSON.stringify([payload.workspaceId, principalId, payload.layout?.id || 'main'])}`;
        let prefs = {};
        try { prefs = JSON.parse(window.localStorage?.getItem(key) || '{}') || {}; } catch (_) {}
        setPreferenceKey(key);
        setLayout(payload.layout);
        const initialSplit = Number(payload.layout?.left?.split?.initial);
        const savedSplit = Number(prefs.split);
        setFraction(savedSplit > 0 && savedSplit < 1 ? savedSplit : (initialSplit > 0 && initialSplit < 1 ? initialSplit : DEFAULT_SPLIT));
        const savedWidth = Number(prefs.width);
        window.dispatchEvent(new CustomEvent('agently:layout-width', { detail: { width: savedWidth > 0 ? savedWidth : initialWidth, min: widthSpec.min, max: widthSpec.max, preferenceKey: key, preferenceFormat: 'object' } }));
        setActiveApp(applications.some((app) => app.id === prefs.activeApp) ? prefs.activeApp : (applications[0]?.id || ''));
        setError('');
      } catch (cause) {
        if (live) { setLayout(null); onTopbarActionsChange?.([]); setError(String(cause?.message || cause)); }
      }
    };
    void load();
    const onAuthorized = () => { setLayout(null); void load(); };
    window.addEventListener('agently:authorized', onAuthorized);
    return () => { live = false; window.removeEventListener('agently:authorized', onAuthorized); };
  }, [reloadVersion]);

  useEffect(() => {
    if (!preferenceKey) return;
    try {
      const current = JSON.parse(window.localStorage?.getItem(preferenceKey) || '{}') || {};
      window.localStorage?.setItem(preferenceKey, JSON.stringify({ ...current, activeApp, split: fraction }));
    } catch (_) {}
  }, [preferenceKey, activeApp, fraction]);

  const applications = layout?.applications || [];
  const navigationEnabled = layout?.left?.navigation?.enabled !== false;
  const historyEnabled = layout?.left?.history?.enabled !== false;
  const history = layout?.left?.history || {};
  if (layout && !navigationEnabled && historyEnabled) {
    return <Sidebar history={history} collapsed={collapsed} onNavigate={onNavigate} onExpand={onExpand} showNewConversation={history.newConversation !== false} />;
  }
  const active = applications.find((app) => app.id === activeApp) || applications[0];
  const minHeight = Number(layout?.left?.split?.minSectionHeight) || 160;
  const shortSidebar = !collapsed && navigationEnabled && historyEnabled && applications.length > 0 && sidebarHeight > 0 && sidebarHeight < 2 * minHeight + 8;
  const canSplit = !collapsed && navigationEnabled && historyEnabled && applications.length > 0 && !shortSidebar;
  const clampFraction = (value) => {
    if (!sidebarHeight) return Math.max(0.2, Math.min(0.8, value));
    const available = Math.max(1, sidebarHeight - 8);
    return Math.min(1 - minHeight / available, Math.max(minHeight / available, value));
  };

  const renderMenu = (menu) => {
    if (Array.isArray(menu.children) && menu.children.length) {
      return <div className={`app-layout-menu-group ${menu.className || ''}`} key={menu.id}>
        <span className="app-layout-menu-group-title">{menu.title}</span>
        {menu.children.map(renderMenu)}
      </div>;
    }
    return <Button key={menu.id} minimal fill alignText="left" className={`app-layout-menu ${menu.className || ''}`} icon={menu.icon ? <Icon icon={menu.icon} className="app-layout-menu-icon" /> : undefined} disabled={menu.disabled || active?.disabled}
      onClick={() => {
        const action = menu.action;
        if (action?.type === 'window') {
          const windowKey = resolveLayoutWindowKey(action);
          openWindow(windowKey, menu.title, action.refreshDataSources || [], resolveLayoutWindowOptions(action, conversationId));
          onOpenWorkspace?.();
          onNavigate?.();
        }
      }}>{menu.title}</Button>;
  };

  const startResize = (event) => {
    if (!shellRef.current) return;
    event.preventDefault();
    const bounds = shellRef.current.getBoundingClientRect();
    const move = (next) => setFraction(clampFraction((next.clientY - bounds.top) / Math.max(1, bounds.height - 8)));
    const stop = () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', stop); };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', stop, { once: true });
  };

  return <div className={`app-layout-sidebar${collapsed ? ' is-collapsed' : ''}${shortSidebar ? ' is-short' : ''}`} ref={shellRef}>
    {collapsed && navigationEnabled && applications.length > 0 ? <div className="app-layout-rail" aria-label="Applications">
      {applications.map((app) => <Button key={app.id} minimal small disabled={app.disabled} icon={app.icon || undefined} className={app.className || undefined}
        aria-label={`Open ${app.title} application menu`} title={app.title}
        onClick={() => { setActiveApp(app.id); onExpand?.(); }}>{app.icon ? null : Array.from(String(app.title || '?'))[0]}</Button>)}
    </div> : null}
    {!collapsed && navigationEnabled && applications.length > 0 ? <>
      <section className={`app-layout-navigation ${active?.className || ''}`} style={canSplit ? { '--app-navigation-basis': `calc((100% - 8px) * ${clampFraction(fraction)})`, '--app-navigation-min-height': `${minHeight}px` } : undefined} aria-label="Applications">
        {applications.length > 1 ? <div className="app-layout-tabs" role="tablist" aria-label="Applications">
          {applications.map((app) => <Button key={app.id} minimal small role="tab" aria-selected={app.id === active?.id} icon={app.icon || undefined} className={app.className || undefined}
            active={app.id === active?.id} disabled={app.disabled} onClick={() => setActiveApp(app.id)}>{app.title}</Button>)}
          <Button minimal small icon="refresh" aria-label="Reload navigation" title="Reload navigation" onClick={() => setReloadVersion((value) => value + 1)} />
        </div> : <div className="app-layout-heading"><span>{active?.icon ? <Icon icon={active.icon} /> : null}{active?.title}</span><Button minimal small icon="refresh" aria-label="Reload navigation" title="Reload navigation" onClick={() => setReloadVersion((value) => value + 1)} /></div>}
        <div className="app-layout-menus">{active?.menus?.map(renderMenu)}</div>
      </section>
      {canSplit && layout?.left?.split?.resizable !== false ? <div className="app-layout-divider" role="separator" aria-orientation="horizontal" aria-label="Resize navigation and conversations"
        tabIndex={0} onPointerDown={startResize}
        onKeyDown={(event) => { if (event.key === 'ArrowUp' || event.key === 'ArrowDown') { event.preventDefault(); setFraction((value) => clampFraction(value + (event.key === 'ArrowDown' ? 0.05 : -0.05))); } }} /> : null}
    </> : null}
    {!collapsed && navigationEnabled && applications.length === 0 && !error ? <Button minimal small icon="refresh" onClick={() => setReloadVersion((value) => value + 1)}>Reload navigation</Button> : null}
    {error ? <div className="app-layout-error" role="alert">{error} <Button small minimal onClick={() => setReloadVersion((value) => value + 1)}>Retry</Button></div> : null}
    {historyEnabled ? <div className="app-layout-history"><Sidebar history={history} collapsed={collapsed} onNavigate={onNavigate} onExpand={onExpand} showNewConversation={history.newConversation !== false} /></div> : null}
  </div>;
}
