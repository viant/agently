import {Icon as UnifiedIcon} from 'forge/components';
import {applicationScope} from '../services/applicationScope';
import React, { useEffect, useMemo, useRef, useState } from 'react';
import {Button, Menu, MenuItem} from '@blueprintjs/core';
import { useSignals } from '@preact/signals-react/runtime';
import { activeWindows, selectedWindowId, selectedTabId, getFormSignal } from 'forge/core';
import { resolveNavigationParameters } from 'forge/utils/navigationParameters';
import { sdkBaseURL } from '../endpoint';
import { getAuthMeSilently } from '../services/agentlyClient';
import Sidebar from './Sidebar';
import SectionNavigation, {SectionDestinationButton} from './SectionNavigation.jsx';
import NavigationFlyout, {NavigationSubmenu} from 'forge/components/NavigationFlyout.jsx';
import {Button as SharedButton, Icon as SharedIcon, Typography, typographyProps} from 'forge/components';
import { openWindow } from './menuBarHelpers';
import { MAIN_CHAT_WINDOW_ID } from '../services/conversationWindow';

import { openLayoutURL, layoutWindowDestinations } from './layoutNavigation';
import { workspaceResources } from '../services/workspaceResources';
import { getWorkspaceStyleManager,workspaceStyleAccountKey } from '../services/workspaceStyles';
import { request } from '../services/httpClient';
import {useWorkspaceStyle} from './WorkspaceStyleProvider.jsx';
import {iconNames} from 'forge/components/Icon.jsx';

import {personalWorkspaceLayout, browserWorkspaceVisibility, workspaceVisibilityScope, visibilityKey, visibleWorkspaceApplications, PERSONAL_WORKSPACE_ID, WORKSPACE_PREFERENCE_STORAGE_KEY, workspaceShowsTiles, workspaceShowsMenu, visibleWorkspaceMenus} from '../services/personalWorkspace';

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
  return { parameters, conversationId: id || undefined, parentKey: MAIN_CHAT_WINDOW_ID, presentation: 'hosted', region: 'chat.top', ...(action.providerIdentity ? {providerIdentity: action.providerIdentity} : {}), ...(action.providerIdentity && action.resourceUri ? {resourceRef: {uri: action.resourceUri}} : {}) };
}

export function resolveLayoutReportOptions(action = {}, conversationId = '', current = {}, applicationId) {
  if (action.type !== 'report' || !String(action.resourceUri || '').startsWith('report://')) return null;
  const options = resolveLayoutWindowOptions(action, conversationId, current);
  if (!options) return null;
  // A personal pin records the report publisher. Published application menus
  // may instead carry the identity of the connection that supplies the menu.
  if (applicationId !== PERSONAL_WORKSPACE_ID) delete options.providerIdentity;
  else if (Object.keys(action.parameters || {}).some(key => key !== 'executeOnOpen')) options.reviewShortcutInputs = true;
  return {...options, resourceRef: {uri: action.resourceUri, ...(action.revision != null ? {revision: String(action.revision)} : {})}};
}

export function selectedWindowInputs() {
  const id = selectedWindowId.value;
  const opened = activeWindows.value.find(entry => entry.windowId === id);
  const form = id ? getFormSignal(`${id}:windowForm`).value : {};
  return {...opened?.parameters, ...form};
}

export default function WorkspaceSidebar({ discoveryReady = true, conversationId = '', collapsed = false, onNavigate, onExpand, onOpenWorkspace, onManageWorkspace, onTopbarActionsChange, section = 'chat', destination = '', onSectionChange, onSectionWindow, onSectionPresentationChange, brandRef, profileRef, drawerHidden = false, drawerMode = false, onDismiss, returnFocusRef }) {
  useSignals();
  const workspaceStyle = useWorkspaceStyle();
  const themeId = workspaceStyle?.state.themeId;
  const fixedRail = themeId === 'design-system-2';
  const fixedSecondary = fixedRail && section === 'workspaces';
  const nextColorMode = workspaceStyle?.state.mode === 'dark' ? 'light' : 'dark';
  const colorModeAction = `Switch to ${nextColorMode} mode`;
  const navigationIcon = (icon, fallback = 'lucide:folder') => {
    const alias = {'volume-up': 'megaphone', exchange: 'handshake', media: 'image', shop: 'building', 'group-objects': 'users', tags: 'tag', 'timeline-events': 'calendar', 'timeline-line-chart': 'chart-no-axes-combined', 'predictive-analysis': 'trending-up'}[icon] || icon;
    const name = `lucide:${String(alias || '').replace(/^lucide:/, '')}`;
    return iconNames.includes(name) ? name : fallback;
  };
  const applicationIcon = app => fixedRail ? navigationIcon(app?.icon, 'lucide:layout-grid') : app?.icon;
  applicationScope.revision.value;
  const windowInputs = selectedWindowInputs();
  const [layout, setLayout] = useState(null);
  const [personalScope, setPersonalScope] = useState('');
  const [preferenceRefresh, setPreferenceRefresh] = useState(0);
  const [personalPreferences, setPersonalPreferences] = useState({});
  const personalPreferenceOwner = useRef('');
  const serverPreferences = useRef({});
  const presentedLayout = useMemo(() => personalWorkspaceLayout(layout, personalPreferences, themeId), [layout, personalPreferences, themeId]);
  useEffect(() => {
    const controller = new AbortController();
    const owner = personalScope ? JSON.parse(personalScope)[0] : '';
    if (personalPreferenceOwner.current !== owner) {serverPreferences.current={};setPersonalPreferences(browserWorkspaceVisibility(owner,{}));personalPreferenceOwner.current = owner;}
    if (personalScope) request('/v1/api/me/preferences', {signal:controller.signal, notify:false}).then(value => {if (!controller.signal.aborted) {serverPreferences.current=value?.workspace || {};setPersonalPreferences(browserWorkspaceVisibility(owner,serverPreferences.current));}}).catch(() => {});
    return () => controller.abort();
  }, [personalScope, preferenceRefresh]);
  const layoutRef = useRef(layout);
  layoutRef.current = layout;
  const [searchMount, setSearchMount] = useState(null);
  const [activeApp, setActiveApp] = useState('');
  // A direct destination can belong to another application without replacing
  // the navigation collection the user is browsing. Native activeApp still
  // owns scoped metadata loading and window registration.
  const [navigationAppId, setNavigationAppId] = useState('');
  // Reuse the declared application's retained context, never focused-window
  // inputs or values held by another application.
  const scopedApplication = layout?.applications?.find(app => app.id === activeApp) || layout?.applications?.[0];
  const layoutContext = scopedApplication ? {applicationId: scopedApplication.id, providerIdentity: scopedApplication.providerIdentity || '', parameters: applicationScope.inputs(scopedApplication)} : null;
  const layoutContextKey = JSON.stringify(layoutContext);
  const layoutContextRef = useRef(null), loadedContextKey = useRef('null'), refreshLayoutRef = useRef(null);
  const scopeSessionRef = useRef('');
  layoutContextRef.current = layoutContext;
  const [collapsedGroups, setCollapsedGroups] = useState({});
  const [fraction, setFraction] = useState(DEFAULT_SPLIT);
  const [error, setError] = useState('');
  const [reloadVersion, setReloadVersion] = useState(0);
  const [destinationVisit, setDestinationVisit] = useState(0);
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
  }, [layout, collapsed, section]);

  useEffect(() => {
    tabsRef.current?.querySelector('[aria-selected="true"]')?.scrollIntoView?.({block: 'nearest', inline: 'nearest'});
    updateTabScroll();
  }, [layout, activeApp, collapsed, section]);

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
    let inFlight = false;
    let revision = null;
    if (!discoveryReady) { setLayout(null); onTopbarActionsChange?.([]); return undefined; }
    const load = async (background = false, contextChanged = false, refreshIdentity = false) => {
      if (background && !contextChanged && (inFlight || document.hidden)) return;
      inFlight = true;
      const sequence = ++requestSequence;
      const context = layoutContextRef.current;
      const current = () => live && sequence === requestSequence && JSON.stringify(context) === JSON.stringify(layoutContextRef.current);
      try {
        // A recovered credential can reveal a different verified owner. Let
        // auth/me's existing session-reset fence cancel this read before any
        // retained scope parameters are sent with the new credential.
        const principal = await getAuthMeSilently({refresh:refreshIdentity});
        if (!current()) return;
        if (!background && reloadVersion > 0) workspaceResources.clear();
        const query = context ? '?' + new URLSearchParams({applicationId: context.applicationId, providerIdentity: context.providerIdentity, parameters: JSON.stringify(context.parameters)}) : '';
        const response = await fetch(`${sdkBaseURL}/workspace/layout${query}`, { credentials: 'include', headers: { Accept: 'application/json' } });
        if (!current()) return;
        if (!response.ok) throw Object.assign(new Error(`Workspace layout unavailable (${response.status})`), {status: response.status});
        const payload = await response.json();
        if (!current()) return;
        const scopeSession = JSON.stringify([payload.workspaceId, principal?.subject || principal?.username || principal?.id || 'anonymous']);
        setPersonalScope(JSON.stringify([workspaceVisibilityScope(payload.workspaceId,principal),reloadVersion]));
        const nextRevision = JSON.stringify([scopeSession, payload.layoutRevision || payload.layout, payload.catalogRevisions, context]);
        if (background && nextRevision === revision) { setError(''); return; }
        let applications = Array.isArray(payload?.layout?.applications) ? payload.layout.applications : [];
        const previous = context && layoutRef.current?.applications?.find(app => app.id === context.applicationId && (app.providerIdentity || '') === context.providerIdentity);
        if (previous?.scope?.keys?.some(key => Object.hasOwn(context.parameters, key)) && !applications.some(app => app.id === previous.id && (app.providerIdentity || '') === (previous.providerIdentity || ''))) {
          // Optional scoped publication can omit navigation without removing
          // the open selector's declared UI context. Retain no prior actions.
          const {id, title, scope, providerIdentity, resourceNamespace, resourceConnection} = previous;
          applications = [...applications, {id, title, scope, providerIdentity, resourceNamespace, resourceConnection, menus: []}];
          payload.layout = {...payload.layout, applications};
        }
        if (scopeSessionRef.current !== scopeSession) sectionRouteSignature.current = null;
        scopeSessionRef.current = scopeSession;
        applicationScope.configure(scopeSession, applications);
        const archives = payload?.layout?.resourceArchives || [];
        if (payload.uiStyleSource==='managed' || payload.globalStyle || archives.length || applications.some(app => app.styleResources?.length)) {
          if (!current()) return;
          const scope = JSON.stringify([payload.workspaceId, principal]);
          workspaceResources.configure(scope, archives);
          void getWorkspaceStyleManager().loadApplicationStyles(applications, {
            accountKey: workspaceStyleAccountKey(principal,sdkBaseURL), globalStyle: payload.globalStyle, styleSource: payload.uiStyleSource || 'local', force: !background && reloadVersion > 0,
            readResource: ({server, uri}) => request(`/api/mcp/resources/read?${new URLSearchParams({server, uri})}`, {retries: 0}),
          });
        } else {
          workspaceResources.clear(); getWorkspaceStyleManager().clearApplicationStyles();
        }
        onTopbarActionsChange?.(payload?.layout?.topbar?.actions || []);
        revision = nextRevision;
        const nextActive = applications.find(app => app.id === context?.applicationId && (app.providerIdentity || '') === context.providerIdentity) || applications[0];
        // A response acknowledges its requested inputs, not a retained scope
        // that may have changed while this request or sidebar was opening.
        loadedContextKey.current = JSON.stringify(nextActive ? {applicationId:nextActive.id,providerIdentity:nextActive.providerIdentity || '',parameters:context?.applicationId === nextActive.id && context.providerIdentity === (nextActive.providerIdentity || '') ? context.parameters : {}} : null);
        if (background) {
          setLayout(payload.layout);
          setActiveApp(current => (current === PERSONAL_WORKSPACE_ID || applications.some(app => app.id === current)) ? current : (applications[0]?.id || ''));
          setError('');
          return;
        }
        const widthSpec = payload?.layout?.left?.width || {};
        const initialWidth = Number(widthSpec.default) || 320;
        if (!current()) return;
        const principalId = String(principal?.subject || principal?.username || principal?.email || principal?.id || 'anonymous');
        const key = `agently.layoutPreferences.v1:${JSON.stringify([payload.workspaceId, principalId, payload.layout?.id || 'main'])}`;
        let prefs = {};
        try { prefs = JSON.parse(window.localStorage?.getItem(key) || '{}') || {}; } catch (_) {}
        setPreferenceKey(key);
        setCollapsedGroups(prefs.collapsedGroups || {});
        setLayout(payload.layout);
        const initialSplit = Number(payload.layout?.left?.split?.initial);
        const savedSplit = Number(prefs.split);
        setFraction(savedSplit > 0 && savedSplit < 1 ? savedSplit : (initialSplit > 0 && initialSplit < 1 ? initialSplit : DEFAULT_SPLIT));
        const globalWidth = widthSpec.preferenceScope === 'global';
        const widthKey = globalWidth ? String(widthSpec.preferenceKey || '') : key;
        const savedWidth = Number(globalWidth ? window.localStorage?.getItem(widthKey) : prefs.width);
        window.dispatchEvent(new CustomEvent('agently:layout-width', { detail: { width: savedWidth > 0 ? savedWidth : initialWidth, min: widthSpec.min, max: widthSpec.max, preferenceKey: widthKey, preferenceFormat: globalWidth ? 'number' : 'object' } }));
        setActiveApp((prefs.activeApp === PERSONAL_WORKSPACE_ID || applications.some((app) => app.id === prefs.activeApp)) ? prefs.activeApp : (applications[0]?.id || ''));
        setError('');
        if (error) window.dispatchEvent(new CustomEvent('forge:retry-window-metadata'));
      } catch (cause) {
        if (current()) {
          // A failed context request invalidates its menus, while the same
          // verified actor's open selector can still supply another context.
          const transient = !cause?.status || cause.status >= 500 || cause.status === 408 || cause.status === 429;
          if (background && revision !== null && contextChanged && context && cause?.status !== 401) {
            // A valid return to the previous context must rebuild its menus.
            revision = '';
            setLayout(previous => previous && {...previous, applications: (previous.applications || []).map(app =>
              app.id === context.applicationId && (app.providerIdentity || '') === context.providerIdentity ? {...app, menus: []} : app)});
          } else if (!(background && revision !== null && transient)) {
            layoutRef.current = null;
            revision = null; setLayout(null); onTopbarActionsChange?.([]);
          }
          setError(String(cause?.message || cause));
        }
      } finally {
        if (sequence === requestSequence) inFlight = false;
      }
    };
    refreshLayoutRef.current = () => load(true, true);
    void load();
    const clearIdentity = () => {
      // A response started by the previous session must never repopulate the
      // retained layout, even when it completes after an account switch.
      ++requestSequence;
      inFlight = false;
      revision = null;
      sectionRouteSignature.current = null;
      scopeSessionRef.current = '';
      setNavigationAppId('');
      setPersonalScope(''); setPersonalPreferences({});
      setPreferenceKey(''); setCollapsedGroups({});
      layoutContextRef.current = null; loadedContextKey.current = 'null';
      applicationScope.clear();
      layoutRef.current = null;
      setLayout(null);
      setError('');
      onTopbarActionsChange?.([]);
    };
    // Credential recovery is not an identity change. The auth client's
    // session-reset event and the native scope's configure identity fence own
    // clearing; retain mounted windows' scope ownership during recovery.
    const onAuthorized = () => { void load(); };
    const refresh = event => { void load(true, false, event?.type==='focus' || event?.type==='visibilitychange'); };
    const timer = window.setInterval(refresh, 15000);
    window.addEventListener('focus', refresh);
    document.addEventListener('visibilitychange', refresh);
    window.addEventListener('agently:authorized', onAuthorized);
    const onPreferencesChanged = event => {
      if(event?.detail?.localVisibility){
        if(event.detail.scope===personalPreferenceOwner.current)setPersonalPreferences(previous=>browserWorkspaceVisibility(personalPreferenceOwner.current,serverPreferences.current));
        return;
      }
      setPreferenceRefresh(value => value + 1);
    };
    const onPreferenceStorage = event => {if (event.key === null || event.key === visibilityKey(personalPreferenceOwner.current))setPersonalPreferences(previous=>browserWorkspaceVisibility(personalPreferenceOwner.current,serverPreferences.current));else if (event.key === WORKSPACE_PREFERENCE_STORAGE_KEY) onPreferencesChanged();};
    window.addEventListener('storage', onPreferenceStorage);
    window.addEventListener('agently:workspace-preferences-changed', onPreferencesChanged);
    window.addEventListener('agently:session-reset', clearIdentity);
    window.addEventListener('agently:logout', clearIdentity);
    return () => { window.removeEventListener('storage', onPreferenceStorage); window.removeEventListener('agently:workspace-preferences-changed', onPreferencesChanged); live = false; refreshLayoutRef.current = null; window.clearInterval(timer); window.removeEventListener('focus', refresh); document.removeEventListener('visibilitychange', refresh); window.removeEventListener('agently:authorized', onAuthorized); window.removeEventListener('agently:session-reset', clearIdentity); window.removeEventListener('agently:logout', clearIdentity); };
  }, [reloadVersion, discoveryReady]);

  useEffect(() => {
    if (discoveryReady && layoutContext && layoutContextKey !== loadedContextKey.current) {
      loadedContextKey.current = layoutContextKey;
      void refreshLayoutRef.current?.();
    }
  }, [layoutContextKey, discoveryReady]);

  useEffect(() => {
    if (!preferenceKey) return;
    try {
      const current = JSON.parse(window.localStorage?.getItem(preferenceKey) || '{}') || {};
      window.localStorage?.setItem(preferenceKey, JSON.stringify({ ...current, activeApp, split: fraction }));
    } catch (_) {}
  }, [preferenceKey, activeApp, fraction]);

  const applications = presentedLayout?.applications || [];
  const displayedApplications = visibleWorkspaceApplications(presentedLayout, personalPreferences);
  const active = displayedApplications.find((app) => app.id === activeApp) || displayedApplications[0];
  const navigationApp = fixedRail
    ? displayedApplications.find(app => app.id === navigationAppId)
      || (active?.id === PERSONAL_WORKSPACE_ID ? active : displayedApplications[0])
    : active;
  const personalTiles = active?.id === PERSONAL_WORKSPACE_ID && workspaceShowsTiles(personalPreferences);
  const personalMenu = active?.id !== PERSONAL_WORKSPACE_ID || workspaceShowsMenu(personalPreferences);
  const displayedMenus = useMemo(() => visibleWorkspaceMenus(navigationApp, personalPreferences), [navigationApp, personalPreferences]);
  const applicationLinks = fixedRail && navigationApp?.id !== PERSONAL_WORKSPACE_ID
    ? displayedApplications.flatMap(app => {
      if (app.id === navigationApp?.id) return [];
      // A published scope-label destination is the application's existing
      // context selector. Do not guess a landing page from report ordering.
      const selectors = visibleWorkspaceMenus(app, personalPreferences).filter(menu =>
        menu.scopeLabelKey && !menu.children?.length && ['window', 'report'].includes(menu.action?.type));
      return selectors.length === 1 ? [{application: app, menu: selectors[0]}] : [];
    }) : [];
  const navigationEnabled = layout?.left?.navigation?.enabled !== false;
  const historyEnabled = layout ? layout.left?.history?.enabled !== false : !!error;
  const history = layout?.left?.history || {};
  const sections = navigationEnabled && layout?.left?.navigation?.presentation === 'sections';
  useEffect(() => {
    onSectionPresentationChange?.(layout || error ? sections : null);
  }, [layout, error, sections, onSectionPresentationChange]);
  const sectionRouteSignature = useRef(null);
  const overviewPublished = useRef(false);
  const sectionWindowRef = useRef(onSectionWindow);
  sectionWindowRef.current = onSectionWindow;
  useEffect(() => {
    const personalHome = navigationEnabled && section === 'workspaces' && !destination && personalTiles;
    if (!sections && !personalHome || section === 'chat') {
      if (overviewPublished.current) sectionWindowRef.current?.(null, '', section, destination, '');
      overviewPublished.current = false;sectionRouteSignature.current = null;return;
    }
    const selected = section === 'workspaces'
      ? layoutWindowDestinations(presentedLayout).find(entry => entry.destination === destination)
      : layout.topbar?.actions?.find(entry => `action:${entry.id}` === section);
    const publish = (entry, title = '', error = '', overview) => overview ? sectionWindowRef.current?.(entry, title, section, destination, error, overview) : sectionWindowRef.current?.(entry, title, section, destination, error);
    if (section === 'workspaces' && !destination) {
      sectionRouteSignature.current = null;overviewPublished.current = personalHome;
      publish(null, personalHome ? active.title : '', '', personalHome ? {title:active.title,groups:active.menus || [],onSelect:activateMenu,onManage:onManageWorkspace} : undefined);return;
    }
    overviewPublished.current = false;
    if (!selected && sectionRouteSignature.current) {
      const {section: openedSection, destination: openedDestination, visit: openedVisit, menu: openedMenu} = sectionRouteSignature.current;
      const application = applications.find(app => app.id === openedMenu.applicationId && app.id === layoutContext?.applicationId && (app.providerIdentity || '') === layoutContext.providerIdentity);
      const retainedContext = application?.menus?.length === 0 && application.scope?.keys?.some(key => Object.hasOwn(applicationScope.inputs(application), key));
      // Unavailable contextual navigation does not close its already-open
      // selector, which can still supply another context without a new open.
      const retainedPersonalPin = openedMenu.applicationId === PERSONAL_WORKSPACE_ID && activeWindows.peek().some(window => window.windowId === sectionRouteSignature.current.windowId);
      if ((error || retainedContext || retainedPersonalPin) && openedSection === section && openedDestination === destination && openedVisit === destinationVisit) return;
    }
    if (!selected || selected.disabled || !['window', 'report'].includes(selected.action?.type)) {
      sectionRouteSignature.current = null;
      publish(null, '', 'This destination is unavailable. Choose another destination from the menu.');
      return;
    }
    const application = applications.find(app => app.id === selected.applicationId);
    const scopeInputs = applicationScope.inputs(application, selectedWindowInputs());
    const resolveOptions = inputs => selected.action.type === 'report'
      ? resolveLayoutReportOptions(selected.action, '', inputs, selected.applicationId)
      : resolveLayoutWindowOptions(selected.action, '', inputs);
    const options = resolveOptions(scopeInputs);
    if (!options) {
      sectionRouteSignature.current = null;
      publish(null, selected.title, 'Select the required context before opening this destination.');
      return;
    }
    const previous = sectionRouteSignature.current;
    const scope = applicationScope.inputs(application);
    const scopeKeys = Array.isArray(application?.scope?.keys) ? application.scope.keys : [];
    const scopeOwner = JSON.stringify([scopeSessionRef.current, application?.providerIdentity || '', application?.id || '', scopeKeys]);
    const sameDestination = previous && previous.section === section && previous.destination === destination
      && previous.visit === destinationVisit && previous.scopeOwner === scopeOwner
      && JSON.stringify(previous.menu.action) === JSON.stringify(selected.action);
    const previousWindowIsLive = previous && activeWindows.peek().some(window => window.windowId === previous.windowId);
    if (sameDestination && (!scopeKeys.length || previousWindowIsLive)
      && JSON.stringify(previous.options) === JSON.stringify(options)) {
      sectionRouteSignature.current = {...previous, scope};
      return;
    }
    // applicationScope.set already retains its selector and invalidates other
    // dependent windows. A scoped menu refresh must keep that live selector:
    // replacing it can let its pending action reparent a child to the old root.
    if (sameDestination && previousWindowIsLive && scopeKeys.length
      && JSON.stringify(previous.scope) !== JSON.stringify(scope)) {
      const previousScopeInputs = {...scopeInputs};
      scopeKeys.forEach(key => { delete previousScopeInputs[key]; });
      Object.assign(previousScopeInputs, previous.scope);
      if (JSON.stringify(resolveOptions(previousScopeInputs)) === JSON.stringify(previous.options)) {
        sectionRouteSignature.current = {...previous, options, scope};
        return;
      }
    }
    if (selected.applicationId) setActiveApp(selected.applicationId);
    const entry = openWindow(resolveLayoutWindowKey(selected.action), selected.title, selected.action.refreshDataSources || [],
      {...options, parentKey: null, region: 'app.section', restoreSelection: !previous && !destinationVisit});
    sectionRouteSignature.current = {section, destination, visit: destinationVisit, menu: selected, options, scope, scopeOwner, windowId: entry?.windowId};
    applicationScope.register(entry, application, selected.action);
    publish(entry, selected.title);
  }, [sections, layout, personalPreferences, section, destination, destinationVisit, activeApp, error]);
  if (layout && !navigationEnabled && historyEnabled) {
    return <Sidebar hidden={drawerHidden} history={history} collapsed={collapsed} onNavigate={onNavigate} onExpand={onExpand} showNewConversation={history.newConversation !== false} />;
  }
  const minHeight = Number(layout?.left?.split?.minSectionHeight) || 160;
  const shortSidebar = !collapsed && navigationEnabled && historyEnabled && displayedApplications.length > 0 && sidebarHeight > 0 && sidebarHeight < 2 * minHeight + 8;
  const canSplit = !collapsed && navigationEnabled && historyEnabled && displayedApplications.length > 0 && !shortSidebar;
  const clampFraction = (value) => {
    if (!sidebarHeight) return Math.max(0.2, Math.min(0.8, value));
    const available = Math.max(1, sidebarHeight - 8);
    return Math.min(1 - minHeight / available, Math.max(minHeight / available, value));
  };

  const toggleGroup = groupKey => {
    const collapsed = !collapsedGroups[groupKey];
    const update = groups => {
      const next = {...groups};
      if (collapsed) next[groupKey] = true;
      else delete next[groupKey];
      return next;
    };
    if (preferenceKey) {
      try {
        const prefs = JSON.parse(window.localStorage.getItem(preferenceKey) || '{}');
        const next = update(prefs.collapsedGroups);
        window.localStorage.setItem(preferenceKey, JSON.stringify({...prefs, collapsedGroups:next}));
        setCollapsedGroups(next);
        return;
      } catch (_) {
        // Collapsing a group still works for this tab when storage is unavailable.
      }
    }
    setCollapsedGroups(update);
  };

  const renderMenu = (menu, parentPath = [], inFlyout = false, application = active) => {
    const groupKey = JSON.stringify([application?.id, parentPath, menu.id]);
    if (Array.isArray(menu.children) && menu.children.length) {
      if (sections && fixedRail) {
        const path = [...parentPath, menu.id];
        const containsDestination = (entry, ids) => entry.children?.length
          ? entry.children.some(child => containsDestination(child, [...ids, child.id]))
          : section === 'workspaces' && destination === JSON.stringify([application?.id, ...ids]);
        const current = containsDestination(menu, path);
        const children = menu.children.map(child => renderMenu(child, path, true, application));
        if (inFlyout) return <NavigationSubmenu key={menu.id} text={menu.title} active={current}
          disabled={menu.disabled || application?.disabled}>
          {children}
        </NavigationSubmenu>;
        return <NavigationFlyout key={menu.id} title={menu.title} icon={navigationIcon(menu.icon)}
          current={current} disabled={menu.disabled || application?.disabled} className={`app-secondary-rail-button ${menu.className || ''}`} >
          <Menu aria-label={menu.title}>{children}</Menu>
        </NavigationFlyout>;
      }
      const expanded = !collapsedGroups[groupKey];
      return <div className={`app-layout-menu-group ${menu.className || ''}`} key={menu.id}>
        <Button minimal fill alignText="left" className="app-layout-menu-group-title" aria-expanded={expanded} {...(sections ? typographyProps('label') : {})}
          icon={sections || menu.iconSvg ? <SharedIcon name={menu.icon || (menu.iconSvg ? undefined : 'lucide:layout-grid')} iconSvg={menu.iconSvg} className="app-layout-group-icon"/> : <UnifiedIcon name={menu.icon || 'folder-close'}  className="app-layout-group-icon" />}
          endIcon={<UnifiedIcon name={expanded ? 'minus' : 'plus'}  />}
          onClick={() => toggleGroup(groupKey)}>{menu.title}</Button>
        {expanded ? <div className="app-layout-menu-children">{menu.children.map(child => renderMenu(child, [...parentPath, menu.id], false, application))}</div> : null}
      </div>;
    }
    const missingInputs = ['window', 'report'].includes(menu.action?.type) && resolveNavigationParameters(menu.action, applicationScope.inputs(application, windowInputs)).missing;
    const destinationId = JSON.stringify([application?.id, ...parentPath, menu.id]);
    const unavailable = sections && layoutWindowDestinations(presentedLayout).find(entry => entry.destination === destinationId)?.disabled;
    const MenuButton = sections ? SectionDestinationButton : Button;
    const title = `${menu.title}${menu.scopeLabelKey && applicationScope.inputs(application)[menu.scopeLabelKey] ? `: ${applicationScope.inputs(application)[menu.scopeLabelKey]}` : ''}`;
    const disabled = menu.disabled || application?.disabled || unavailable || missingInputs;
    if (inFlyout) return <MenuItem key={menu.id} text={title} disabled={disabled}
      active={section === 'workspaces' && destination === destinationId}
      aria-current={section === 'workspaces' && destination === destinationId ? 'page' : undefined}
      className={menu.className || ''} onClick={() => activateMenu(menu, parentPath, application)}/>;
    if (sections && fixedRail) return <Button key={menu.id} minimal {...typographyProps('metadata')}
      className={`forge-navigation-trigger app-secondary-rail-button ${menu.className || ''}`}
      aria-current={section === 'workspaces' && destination === destinationId ? 'page' : undefined}
      disabled={disabled} onClick={() => activateMenu(menu, parentPath, application)}>
      <SharedIcon name={navigationIcon(menu.icon, menu.scopeLabelKey ? 'lucide:sliders-horizontal' : 'lucide:file-text')}/>
      <span className="forge-navigation-label">{title}</span>
    </Button>;
    return <MenuButton key={menu.id} minimal fill alignText="left" className={`app-layout-menu ${menu.className || ''}`} {...(sections ? {selected: section === 'workspaces' && destination === destinationId} : {})} icon={menu.icon || menu.iconSvg ? sections || menu.iconSvg ? <SharedIcon name={menu.icon} iconSvg={menu.iconSvg} className="app-layout-menu-icon"/> : <UnifiedIcon name={menu.icon}  className="app-layout-menu-icon" /> : undefined} disabled={menu.disabled || application?.disabled || unavailable || missingInputs}
      onClick={() => activateMenu(menu, parentPath, application)}>{menu.title}{menu.scopeLabelKey && applicationScope.inputs(application)[menu.scopeLabelKey] ? `: ${applicationScope.inputs(application)[menu.scopeLabelKey]}` : ''}</MenuButton>;
  };

  function activateMenu(menu, parentPath = [], application = active) {
    const destinationId = JSON.stringify([application?.id, ...parentPath, menu.id]);
    if (!sections) onSectionChange?.('chat');
    const action = menu.action;
    if (openLayoutURL(action)) { onNavigate?.(); return; }
    if (action?.type === 'report') {
      const options = resolveLayoutReportOptions(action, conversationId, applicationScope.inputs(application, selectedWindowInputs()), application?.id);
      if (!options) return;
      if (sections) {
        if (section === 'workspaces' && destination === destinationId) setDestinationVisit(value => value + 1);
        onSectionChange?.('workspaces', destinationId);
      } else {
        applicationScope.register(openWindow(action.resourceUri, menu.title, [], options), application, action);
        onOpenWorkspace?.();
      }
      onNavigate?.();
      return;
    }
    if (action?.type === 'window') {
      const windowKey = resolveLayoutWindowKey(action);
      const options = resolveLayoutWindowOptions(action, conversationId, applicationScope.inputs(application, selectedWindowInputs()));
      if (!options) return;
      if (sections) {
        if (section === 'workspaces' && destination === destinationId) setDestinationVisit(value => value + 1);
        onSectionChange?.('workspaces', destinationId);
      } else {
        applicationScope.register(openWindow(windowKey, menu.title, action.refreshDataSources || [], options), application, action);
        onOpenWorkspace?.();
      }
      onNavigate?.();
    }
  }
  function selectApplication(app) {
    setActiveApp(app.id);
    setNavigationAppId(app.id);
    if (app.id === PERSONAL_WORKSPACE_ID && workspaceShowsTiles(personalPreferences)) {
      onSectionChange?.('workspaces', '');onNavigate?.();
    }
  }

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
      if (collapsed && !fixedSecondary) onExpand?.();
      onSectionChange?.(next);
      if (next === 'chat') {
        selectedWindowId.value = MAIN_CHAT_WINDOW_ID;
        selectedTabId.value = MAIN_CHAT_WINDOW_ID;
      }
      if (next !== 'workspaces') onNavigate?.();
    };
    return <div className={`app-layout-sidebar is-sections${fixedSecondary ? ' has-fixed-secondary' : collapsed ? ' is-collapsed' : ''}`} hidden={!fixedSecondary && drawerHidden} inert={!fixedSecondary && drawerHidden ? '' : undefined}>
      <SectionNavigation section={section} actions={actions} onSelect={selectSection} brandRef={brandRef} headingActionsRef={setSearchMount} profileRef={profileRef}
        drawerOpen={!fixedSecondary && drawerMode && !drawerHidden} onDismiss={onDismiss} returnFocusRef={returnFocusRef}
        workspaceNavigation={<>
          <div className="app-secondary-rail-items" aria-label={`${navigationApp?.title || 'Application'} navigation`}>
            {displayedApplications.length > 0 ? <NavigationFlyout title={navigationApp?.title || 'Applications'} icon={applicationIcon(navigationApp)} current={!!navigationApp}>
              <Menu aria-label="Applications">{displayedApplications.map(app => <MenuItem key={app.id} text={app.title}
                active={app.id === navigationApp?.id} aria-current={app.id === navigationApp?.id ? 'true' : undefined} disabled={app.disabled} onClick={() => selectApplication(app)}/>)}</Menu>
            </NavigationFlyout> : null}
            <Button minimal className="forge-navigation-trigger app-secondary-rail-button" {...typographyProps('metadata')}
              aria-current={section === 'workspaces' && !destination ? 'page' : undefined}
              onClick={() => {onSectionChange?.('workspaces'); onNavigate?.();}}>
              <SharedIcon name="lucide:layout-grid"/><span className="forge-navigation-label">Overview</span>
            </Button>
            {personalTiles && <Button minimal fill icon={<SharedIcon name="lucide:layout-grid"/>} onClick={() => {onSectionChange?.('workspaces', '');onNavigate?.();}}>Open My workspace tiles</Button>}
            {personalMenu && displayedMenus.map(menu => renderMenu(menu, [], false, navigationApp))}
            {applicationLinks.map(({application, menu}) => <Button key={application.id} minimal {...typographyProps('metadata')}
              className="forge-navigation-trigger app-secondary-rail-button" aria-label={application.title}
              aria-current={section === 'workspaces' && layoutWindowDestinations({applications:[application]}).some(entry => entry.destination === destination) ? 'page' : undefined}
              disabled={application.disabled || menu.disabled || resolveNavigationParameters(menu.action, applicationScope.inputs(application, windowInputs)).missing}
              onClick={() => activateMenu(menu, [], application)}>
              <SharedIcon name={applicationIcon(application)}/><span className="forge-navigation-label">{application.title}</span>
            </Button>)}
            {personalMenu && !displayedMenus.length && !applicationLinks.length ? <Typography typeRole="small" role="status">No navigation is available.</Typography> : null}
          </div>
          <div className="app-secondary-rail-footer">
            <SharedButton variant="ghost" size="compact" hideLabel icon={`lucide:${nextColorMode === 'dark' ? 'moon' : 'sun'}`}
              aria-label={colorModeAction} title={colorModeAction}
              disabled={!workspaceStyle?.state.catalog?.themes.find(theme => theme.id === workspaceStyle.state.themeId)?.modes[nextColorMode]}
              onClick={() => workspaceStyle.manager.setPreference(workspaceStyle.state.themeId, nextColorMode)}/>
            <Button minimal small aria-label="Manage Workspace" title="Manage Workspace" disabled={!onManageWorkspace}
              icon={<SharedIcon name="lucide:settings"/>} onClick={() => {onManageWorkspace?.();onNavigate?.();}}/>
            <Button minimal small icon={<SharedIcon name="lucide:refresh-cw"/>} aria-label="Reload navigation" onClick={() => setReloadVersion(value => value + 1)}/>
          </div>
          {error ? <div role="alert">{error}</div> : null}
        </>}>
        <div className="app-section-history" hidden={section !== 'chat'} data-section-hidden={section !== 'chat' ? '' : undefined}>
          {historyEnabled ? <Sidebar appearance="document" hidden={drawerHidden || section !== 'chat'} searchMount={searchMount} history={history} onNavigate={onNavigate} showNewConversation={history.newConversation !== false}/> : null}
        </div>
        {section !== 'chat' ? <>
          <div className="app-section-menu-heading"><Typography as="h2" typeRole="section">{action?.title || 'Workspaces'}</Typography>
            <Button minimal small disabled={!onManageWorkspace} onClick={()=>{onManageWorkspace?.();onNavigate?.();}} aria-label="Manage Workspace" icon={<SharedIcon name="lucide:settings"/>}/>
            <Button minimal small icon={<SharedIcon name="lucide:refresh-cw"/>} aria-label="Reload navigation" onClick={() => setReloadVersion(value => value + 1)}/>
          </div>
          {section === 'workspaces' ? <>
            <SectionDestinationButton icon={<SharedIcon name="lucide:layout-grid"/>} selected={!destination}
              onClick={() => {onSectionChange?.('workspaces'); onNavigate?.();}}>Overview</SectionDestinationButton>
            {displayedApplications.length > 0 ? <div className="app-layout-tabbar app-section-application-bar">
              {tabScroll.left || tabScroll.right ? <Button minimal small icon={<SharedIcon name="step-backward"/>} aria-label="Scroll applications left" disabled={!tabScroll.left} onClick={() => scrollTabs(-1)}/> : null}
              <div className="app-layout-tabs app-section-applications" role="tablist" aria-label="Applications" ref={tabsRef} onScroll={updateTabScroll}>
              {displayedApplications.map(app => <Button key={app.id} minimal role="tab" active={app.id === active?.id}
                aria-selected={app.id === active?.id} tabIndex={app.id === active?.id ? 0 : -1}
                icon={<SharedIcon name={app.icon || 'lucide:layout-grid'}/>}
                disabled={app.disabled} onClick={() => selectApplication(app)}
                onKeyDown={event => {
                  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
                  event.preventDefault();
                  const enabled = displayedApplications.filter(item => !item.disabled);
                  const index = enabled.findIndex(item => item.id === app.id);
                  const next = event.key === 'Home' ? 0 : event.key === 'End' ? enabled.length - 1
                    : (index + (event.key === 'ArrowRight' ? 1 : -1) + enabled.length) % enabled.length;
                  selectApplication(enabled[next]);
                  event.currentTarget.parentElement.querySelectorAll('[role="tab"]:not(:disabled)')[next]?.focus();
                }}>{app.title}</Button>)}
              </div>
              {tabScroll.left || tabScroll.right ? <Button minimal small icon={<SharedIcon name="step-forward"/>} aria-label="Scroll applications right" disabled={!tabScroll.right} onClick={() => scrollTabs(1)}/> : null}
            </div> : null}
            <div className="app-layout-menus" aria-label={`${active?.title || 'Application'} navigation`}>
              {personalTiles && <Button minimal fill alignText="left" icon={<SharedIcon name="lucide:layout-grid"/>} onClick={() => {onSectionChange?.('workspaces', '');onNavigate?.();}}>Open My workspace tiles</Button>}
              {personalMenu && (displayedMenus.length ? displayedMenus.map(menu => renderMenu(menu)) : <div role="status" className="app-section-menu-empty">
                <Typography typeRole="body">{active?.id === PERSONAL_WORKSPACE_ID ? 'Ask the assistant to pin a window or report here.' : `No navigation is available for ${active?.title || 'this workspace'}.`}</Typography>
                <Button minimal small icon={<SharedIcon name="lucide:refresh-cw"/>} onClick={() => setReloadVersion(value => value + 1)}>Reload menu</Button>
              </div>)}
            </div>
          </> : null}
        </> : null}
        {error ? <div role="alert">{error}<Button minimal onClick={() => setReloadVersion(value => value + 1)}>Retry</Button></div> : null}
      </SectionNavigation>
    </div>;
  }

  return <div className={`app-layout-sidebar${collapsed ? ' is-collapsed' : ''}${shortSidebar ? ' is-short' : ''}`} ref={shellRef} hidden={drawerHidden} inert={drawerHidden ? '' : undefined}>
    {collapsed && navigationEnabled && displayedApplications.length > 0 ? <div className="app-layout-rail" aria-label="Applications">
      {displayedApplications.map((app) => <Button key={app.id} minimal small disabled={app.disabled} icon={(iconName => iconName ? <UnifiedIcon name={iconName}/> : iconName)(app.icon || undefined)} className={app.className || undefined}
        aria-label={`Open ${app.title} application menu`} title={app.title}
        onClick={() => { selectApplication(app); onExpand?.(); }}>{app.icon ? null : Array.from(String(app.title || '?'))[0]}</Button>)}
    </div> : null}
    {!collapsed && navigationEnabled && displayedApplications.length > 0 ? <>
      <section className={`app-layout-navigation ${active?.className || ''}`} style={canSplit ? { '--app-navigation-basis': `calc((100% - 8px) * ${clampFraction(fraction)})`, '--app-navigation-min-height': `${minHeight}px` } : undefined} aria-label="Applications">
        {displayedApplications.length > 1 ? <div className="app-layout-tabbar">
          {tabScroll.left || tabScroll.right ? <Button minimal small icon={<UnifiedIcon name="chevron-left"/>} aria-label="Scroll applications left" disabled={!tabScroll.left} onClick={() => scrollTabs(-1)} /> : null}
          <div className="app-layout-tabs" role="tablist" aria-label="Applications" ref={tabsRef} onScroll={updateTabScroll}>
          {displayedApplications.map((app) => <Button key={app.id} minimal small role="tab" aria-selected={app.id === active?.id} icon={(iconName => iconName ? <UnifiedIcon name={iconName}/> : iconName)(app.icon || undefined)} className={app.className || undefined}
            active={app.id === active?.id} disabled={app.disabled} onClick={() => selectApplication(app)}>{app.title}</Button>)}
          </div>
          {tabScroll.left || tabScroll.right ? <Button minimal small icon={<UnifiedIcon name="chevron-right"/>} aria-label="Scroll applications right" disabled={!tabScroll.right} onClick={() => scrollTabs(1)} /> : null}
          <Button minimal small disabled={!onManageWorkspace} icon={<SharedIcon name="lucide:settings"/>} aria-label="Manage Workspace" onClick={()=>{onManageWorkspace?.();onNavigate?.();}}/><Button minimal small icon={<UnifiedIcon name="refresh"/>} aria-label="Reload navigation" title="Reload navigation" onClick={() => setReloadVersion((value) => value + 1)} />
        </div> : <div className="app-layout-heading"><span>{active?.icon ? <UnifiedIcon name={active.icon} /> : null}{active?.title}</span><Button minimal small disabled={!onManageWorkspace} icon={<SharedIcon name="lucide:settings"/>} aria-label="Manage Workspace" onClick={()=>{onManageWorkspace?.();onNavigate?.();}}/><Button minimal small icon={<UnifiedIcon name="refresh"/>} aria-label="Reload navigation" title="Reload navigation" onClick={() => setReloadVersion((value) => value + 1)} /></div>}
        <div className="app-layout-menus">{personalTiles && <Button minimal fill alignText="left" icon={<SharedIcon name="lucide:layout-grid"/>} onClick={() => {onSectionChange?.('workspaces', '');onNavigate?.();}}>Open My workspace tiles</Button>}{personalMenu && displayedMenus.map(menu => renderMenu(menu))}{personalMenu && active?.id === PERSONAL_WORKSPACE_ID && !active.menus?.length ? <p>Ask the assistant to pin a window or report here.</p> : null}</div>
      </section>
      {canSplit && layout?.left?.split?.resizable !== false ? <div className="app-layout-divider" role="separator" aria-orientation="horizontal" aria-label="Resize navigation and conversations"
        tabIndex={0} onPointerDown={startResize}
        onKeyDown={(event) => { if (event.key === 'ArrowUp' || event.key === 'ArrowDown') { event.preventDefault(); setFraction((value) => clampFraction(value + (event.key === 'ArrowDown' ? 0.05 : -0.05))); } }} /> : null}
    </> : null}
    {navigationEnabled && (collapsed || displayedApplications.length===0) && <Button minimal icon={<SharedIcon name="lucide:settings"/>} disabled={!onManageWorkspace} onClick={()=>{onManageWorkspace?.();onNavigate?.();}} aria-label="Manage Workspace">Manage Workspace</Button>}
    {!collapsed && navigationEnabled && displayedApplications.length === 0 && !error ? <Button minimal small icon={<UnifiedIcon name="refresh"/>} onClick={() => setReloadVersion((value) => value + 1)}>Reload navigation</Button> : null}
    {error ? <div className="app-layout-error" role="alert">{error} <Button small minimal onClick={() => setReloadVersion((value) => value + 1)}>Retry</Button></div> : null}
    {historyEnabled ? <div className="app-layout-history"><Sidebar hidden={drawerHidden} history={history} collapsed={collapsed} onNavigate={onNavigate} onExpand={onExpand} showNewConversation={history.newConversation !== false} /></div> : null}
  </div>;
}
