import React, { useEffect, useRef, useState } from 'react';
import { Button, Icon } from '@blueprintjs/core';
import { useSignals } from '@preact/signals-react/runtime';
import { activeWindows, selectedWindowId, selectedTabId, getFormSignal } from 'forge/core';
import { resolveNavigationParameters } from 'forge/utils/navigationParameters';
import { sdkBaseURL } from '../endpoint';
import { getAuthMeSilently } from '../services/agentlyClient';
import Sidebar from './Sidebar';
import SectionNavigation, {SectionDestinationButton} from './SectionNavigation.jsx';
import {Icon as SharedIcon, Typography, typographyProps} from 'forge/components';
import { openWindow } from './menuBarHelpers';
import { MAIN_CHAT_WINDOW_ID } from '../services/conversationWindow';

import { openLayoutURL, layoutWindowDestinations } from './layoutNavigation';
import { workspaceResources } from '../services/workspaceResources';
import { getWorkspaceStyleManager } from '../services/workspaceStyles';
import { request } from '../services/httpClient';

const DEFAULT_SPLIT = 0.5;

export function resolveLayoutWindowKey(action = {}) {
  if (action.resourceUri) return action.resourceUri;
  if (!action?.windowKey) return '';
  return action.provider && action.provider !== 'workspace'
    ? `provider:${action.provider}:${action.windowKey}` : action.windowKey;
}

export function resolveLayoutWindowOptions(action = {}, conversationId = '', current = {}) {
  const id = String(conversationId || '').trim();
  const {parameters, missing} = resolveNavigationParameters(action, current);
  if (missing) return null;
  return { parameters, conversationId: id || undefined, parentKey: MAIN_CHAT_WINDOW_ID, presentation: 'hosted', region: 'chat.top', ...(action.providerIdentity ? {providerIdentity: action.providerIdentity} : {}) };
}

export function selectedWindowInputs() {
  const id = selectedWindowId.value;
  const opened = activeWindows.value.find(entry => entry.windowId === id);
  const form = id ? getFormSignal(`${id}:windowForm`).value : {};
  return {...opened?.parameters, ...form};
}

export default function WorkspaceSidebar({ discoveryReady = true, conversationId = '', collapsed = false, onNavigate, onExpand, onOpenWorkspace, onTopbarActionsChange, section = 'chat', destination = '', onSectionChange, onSectionWindow, onSectionPresentationChange, brandRef, profileRef, drawerHidden = false, drawerMode = false, onDismiss, returnFocusRef }) {
  useSignals();
  const windowInputs = selectedWindowInputs();
  const [layout, setLayout] = useState(null);
  const [searchMount, setSearchMount] = useState(null);
  const [activeApp, setActiveApp] = useState('');
  const [collapsedGroups, setCollapsedGroups] = useState({});
  const [fraction, setFraction] = useState(DEFAULT_SPLIT);
  const [error, setError] = useState('');
  const [reloadVersion, setReloadVersion] = useState(0);
  const [preferenceKey, setPreferenceKey] = useState('');
  const [sidebarHeight, setSidebarHeight] = useState(0);
  const shellRef = useRef(null);
  const tabsRef = useRef(null);
  const [tabScroll, setTabScroll] = useState({left: false, right: false});
  const updateTabScroll = () => {
    const node = tabsRef.current;
    if (!node) return;
    const next = {left: node.scrollLeft > 1, right: node.scrollLeft + node.clientWidth < node.scrollWidth - 1};
    setTabScroll(previous => previous.left === next.left && previous.right === next.right ? previous : next);
  };
  const scrollTabs = direction => tabsRef.current?.scrollBy({left: direction * Math.max(100, tabsRef.current.clientWidth * 0.75), behavior: 'smooth'});

  useEffect(() => {
    const node = tabsRef.current;
    if (!node) return undefined;
    updateTabScroll();
    const observer = typeof ResizeObserver !== 'undefined' ? new ResizeObserver(updateTabScroll) : null;
    observer?.observe(node);
    return () => observer?.disconnect();
  }, [layout, collapsed]);

  useEffect(() => {
    tabsRef.current?.querySelector('[aria-selected="true"]')?.scrollIntoView?.({block: 'nearest', inline: 'nearest'});
    updateTabScroll();
  }, [layout, activeApp, collapsed]);

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
    let requestSequence = 0;
    if (!discoveryReady) { setLayout(null); onTopbarActionsChange?.([]); return undefined; }
    const load = async () => {
      const sequence = ++requestSequence;
      try {
        if (reloadVersion > 0) workspaceResources.clear();
        const response = await fetch(`${sdkBaseURL}/workspace/layout`, { credentials: 'include', headers: { Accept: 'application/json' } });
        if (!live || sequence !== requestSequence) return;
        if (!response.ok) throw new Error(`Workspace layout unavailable (${response.status})`);
        const payload = await response.json();
        if (!live || sequence !== requestSequence) return;
        const applications = Array.isArray(payload?.layout?.applications) ? payload.layout.applications : [];
        const archives = payload?.layout?.resourceArchives || [];
        if (archives.length || applications.some(app => app.styleResources?.length)) {
          const principal = await getAuthMeSilently();
          if (!live || sequence !== requestSequence) return;
          const scope = JSON.stringify([payload.workspaceId, principal]);
          workspaceResources.configure(scope, archives);
          void getWorkspaceStyleManager().loadApplicationStyles(applications, {
            accountKey: scope, force: reloadVersion > 0,
            readResource: ({server, uri}) => request(`/api/mcp/resources/read?${new URLSearchParams({server, uri})}`, {retries: 0}),
          });
        } else {
          workspaceResources.clear(); getWorkspaceStyleManager().clearApplicationStyles();
        }
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
        if (error) window.dispatchEvent(new CustomEvent('forge:retry-window-metadata'));
          return;
        }
        const me = await getAuthMeSilently().catch(() => null);
        if (!live || sequence !== requestSequence) return;
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
        if (error) window.dispatchEvent(new CustomEvent('forge:retry-window-metadata'));
      } catch (cause) {
        if (live && sequence === requestSequence) { setLayout(null); onTopbarActionsChange?.([]); setError(String(cause?.message || cause)); }
      }
    };
    void load();
    const onAuthorized = () => { setLayout(null); void load(); };
    window.addEventListener('agently:authorized', onAuthorized);
    return () => { live = false; window.removeEventListener('agently:authorized', onAuthorized); };
  }, [reloadVersion, discoveryReady]);

  useEffect(() => {
    if (!preferenceKey) return;
    try {
      const current = JSON.parse(window.localStorage?.getItem(preferenceKey) || '{}') || {};
      window.localStorage?.setItem(preferenceKey, JSON.stringify({ ...current, activeApp, split: fraction }));
    } catch (_) {}
  }, [preferenceKey, activeApp, fraction]);

  const applications = layout?.applications || [];
  const navigationEnabled = layout?.left?.navigation?.enabled !== false;
  const historyEnabled = layout ? layout.left?.history?.enabled !== false : !!error;
  const history = layout?.left?.history || {};
  const sections = navigationEnabled && layout?.left?.navigation?.presentation === 'sections';
  useEffect(() => { onSectionPresentationChange?.(sections); }, [sections, onSectionPresentationChange]);
  const sectionWindowRef = useRef(onSectionWindow);
  sectionWindowRef.current = onSectionWindow;
  useEffect(() => {
    if (!sections || section === 'chat') return;
    const selected = section === 'workspaces'
      ? layoutWindowDestinations(layout).find(entry => entry.destination === destination)
      : layout.topbar?.actions?.find(entry => `action:${entry.id}` === section);
    const publish = (entry, title = '', error = '') => sectionWindowRef.current?.(entry, title, section, destination, error);
    if (section === 'workspaces' && !destination) { publish(null); return; }
    if (!selected || selected.disabled || selected.action?.type !== 'window') {
      publish(null, '', 'This destination is unavailable. Choose another destination from the menu.');
      return;
    }
    const options = resolveLayoutWindowOptions(selected.action, '', selectedWindowInputs());
    if (!options) {
      publish(null, selected.title, 'Select the required context before opening this destination.');
      return;
    }
    if (selected.applicationId) setActiveApp(selected.applicationId);
    const entry = openWindow(resolveLayoutWindowKey(selected.action), selected.title, selected.action.refreshDataSources || [],
      {...options, parentKey: null, region: 'app.section'});
    publish(entry, selected.title);
  }, [sections, layout, section, destination]);
  if (layout && !navigationEnabled && historyEnabled) {
    return <Sidebar hidden={drawerHidden} history={history} collapsed={collapsed} onNavigate={onNavigate} onExpand={onExpand} showNewConversation={history.newConversation !== false} />;
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

  const renderMenu = (menu, parentPath = []) => {
    const groupKey = JSON.stringify([active?.id, parentPath, menu.id]);
    if (Array.isArray(menu.children) && menu.children.length) {
      const expanded = !collapsedGroups[groupKey];
      return <div className={`app-layout-menu-group ${menu.className || ''}`} key={menu.id}>
        <Button minimal fill alignText="left" className="app-layout-menu-group-title" aria-expanded={expanded} {...(sections ? typographyProps('label') : {})}
          icon={sections ? <SharedIcon name={menu.icon || 'lucide:layout-grid'}/> : <Icon icon={menu.icon || 'folder-close'} size={14} className="app-layout-group-icon" />}
          endIcon={<Icon icon={expanded ? 'minus' : 'plus'} size={12} />}
          onClick={() => setCollapsedGroups(previous => ({...previous, [groupKey]: !previous[groupKey]}))}>{menu.title}</Button>
        {expanded ? <div className="app-layout-menu-children">{menu.children.map(child => renderMenu(child, [...parentPath, menu.id]))}</div> : null}
      </div>;
    }
    const missingInputs = menu.action?.type === 'window' && resolveNavigationParameters(menu.action, windowInputs).missing;
    const destinationId = JSON.stringify([active?.id, ...parentPath, menu.id]);
    const unavailable = sections && layoutWindowDestinations(layout).find(entry => entry.destination === destinationId)?.disabled;
    const MenuButton = sections ? SectionDestinationButton : Button;
    return <MenuButton key={menu.id} minimal fill alignText="left" className={`app-layout-menu ${menu.className || ''}`} {...(sections ? {selected: section === 'workspaces' && destination === destinationId} : {})} icon={menu.icon ? sections ? <SharedIcon name={menu.icon}/> : <Icon icon={menu.icon} size={14} className="app-layout-menu-icon" /> : undefined} disabled={menu.disabled || active?.disabled || unavailable || missingInputs}
      onClick={() => {
        const action = menu.action;
        if (openLayoutURL(action)) { onNavigate?.(); return; }
        if (action?.type === 'window') {
          const windowKey = resolveLayoutWindowKey(action);
          const options = resolveLayoutWindowOptions(action, conversationId, selectedWindowInputs());
          if (!options) return;
          if (sections) {
            onSectionChange?.('workspaces', destinationId);
          } else {
            openWindow(windowKey, menu.title, action.refreshDataSources || [], options);
            onOpenWorkspace?.();
          }
          onNavigate?.();
        }
      }}>{menu.title}</MenuButton>;
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

  if (sections) {
    const actions = (layout?.topbar?.actions || []).map(entry => ({...entry, disabled: entry.disabled || (entry.action?.type === 'window' && resolveNavigationParameters(entry.action, windowInputs).missing)}));
    const action = actions.find(entry => `action:${entry.id}` === section);
    const selectSection = next => {
      const destination = actions.find(entry => `action:${entry.id}` === next);
      if (destination?.disabled) return;
      if (destination && openLayoutURL(destination.action)) return;
      if (collapsed) onExpand?.();
      onSectionChange?.(next);
      if (next === 'chat') {
        selectedWindowId.value = MAIN_CHAT_WINDOW_ID;
        selectedTabId.value = MAIN_CHAT_WINDOW_ID;
      }
      if (next !== 'workspaces') onNavigate?.();
    };
    return <div className={`app-layout-sidebar is-sections${collapsed ? ' is-collapsed' : ''}`} hidden={drawerHidden} inert={drawerHidden ? '' : undefined}>
      <SectionNavigation section={section} actions={actions} onSelect={selectSection} brandRef={brandRef} headingActionsRef={setSearchMount} profileRef={profileRef} drawerOpen={drawerMode && !drawerHidden} onDismiss={onDismiss} returnFocusRef={returnFocusRef}>
        <div className="app-section-history" hidden={section !== 'chat'} data-section-hidden={section !== 'chat' ? '' : undefined}>
          {historyEnabled ? <Sidebar appearance="document" searchMount={searchMount} history={history} onNavigate={onNavigate} showNewConversation={history.newConversation !== false}/> : null}
        </div>
        {section !== 'chat' ? <>
          <div className="app-section-menu-heading"><Typography as="h2" typeRole="section">{action?.title || 'Workspaces'}</Typography>
            <Button minimal small icon={<SharedIcon name="lucide:refresh-cw"/>} aria-label="Reload navigation" onClick={() => setReloadVersion(value => value + 1)}/>
          </div>
          {section === 'workspaces' ? <>
            <SectionDestinationButton icon={<SharedIcon name="lucide:layout-grid"/>} selected={!destination}
              onClick={() => {onSectionChange?.('workspaces'); onNavigate?.();}}>Overview</SectionDestinationButton>
            {applications.length > 1 ? <div className="app-layout-tabs" role="tablist" aria-label="Applications">{applications.map(app => <Button key={app.id} minimal role="tab" aria-selected={app.id === active?.id} disabled={app.disabled} onClick={() => setActiveApp(app.id)}>{app.title}</Button>)}</div> : null}
            <div className="app-layout-menus">{active?.menus?.map(menu => renderMenu(menu))}</div>
          </> : null}
        </> : null}
        {error ? <div role="alert">{error}<Button minimal onClick={() => setReloadVersion(value => value + 1)}>Retry</Button></div> : null}
      </SectionNavigation>
    </div>;
  }

  return <div className={`app-layout-sidebar${collapsed ? ' is-collapsed' : ''}${shortSidebar ? ' is-short' : ''}`} ref={shellRef} hidden={drawerHidden} inert={drawerHidden ? '' : undefined}>
    {collapsed && navigationEnabled && applications.length > 0 ? <div className="app-layout-rail" aria-label="Applications">
      {applications.map((app) => <Button key={app.id} minimal small disabled={app.disabled} icon={app.icon || undefined} className={app.className || undefined}
        aria-label={`Open ${app.title} application menu`} title={app.title}
        onClick={() => { setActiveApp(app.id); onExpand?.(); }}>{app.icon ? null : Array.from(String(app.title || '?'))[0]}</Button>)}
    </div> : null}
    {!collapsed && navigationEnabled && applications.length > 0 ? <>
      <section className={`app-layout-navigation ${active?.className || ''}`} style={canSplit ? { '--app-navigation-basis': `calc((100% - 8px) * ${clampFraction(fraction)})`, '--app-navigation-min-height': `${minHeight}px` } : undefined} aria-label="Applications">
        {applications.length > 1 ? <div className="app-layout-tabbar">
          {tabScroll.left || tabScroll.right ? <Button minimal small icon="chevron-left" aria-label="Scroll applications left" disabled={!tabScroll.left} onClick={() => scrollTabs(-1)} /> : null}
          <div className="app-layout-tabs" role="tablist" aria-label="Applications" ref={tabsRef} onScroll={updateTabScroll}>
          {applications.map((app) => <Button key={app.id} minimal small role="tab" aria-selected={app.id === active?.id} icon={app.icon || undefined} className={app.className || undefined}
            active={app.id === active?.id} disabled={app.disabled} onClick={() => setActiveApp(app.id)}>{app.title}</Button>)}
          </div>
          {tabScroll.left || tabScroll.right ? <Button minimal small icon="chevron-right" aria-label="Scroll applications right" disabled={!tabScroll.right} onClick={() => scrollTabs(1)} /> : null}
          <Button minimal small icon="refresh" aria-label="Reload navigation" title="Reload navigation" onClick={() => setReloadVersion((value) => value + 1)} />
        </div> : <div className="app-layout-heading"><span>{active?.icon ? <Icon icon={active.icon} /> : null}{active?.title}</span><Button minimal small icon="refresh" aria-label="Reload navigation" title="Reload navigation" onClick={() => setReloadVersion((value) => value + 1)} /></div>}
        <div className="app-layout-menus">{active?.menus?.map(menu => renderMenu(menu))}</div>
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
