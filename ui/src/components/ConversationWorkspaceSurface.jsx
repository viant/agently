import InlineReportWorkspace from './InlineReportWorkspace.jsx';
import {WorkspaceRuntimeTabs,WorkspaceRuntimePanels,shouldShowWorkspaceTabs as sharedShouldShowWorkspaceTabs} from 'forge/components/WorkspaceRuntimeSurface.jsx';
import {resolveWorkspaceWindowTitle} from '../services/workspaceWindowTitle.js';
import {useWorkspaceScroll} from '../services/useWorkspaceScroll.js';
import { findMetadataSignal } from 'forge/core';
import ToolFeedDetail from './ToolFeedDetail.jsx';
import React, { useState, useEffect, useRef } from 'react';
import {createPortal} from 'react-dom';
import SavedOutputInventory from './SavedOutputInventory.jsx';
import {ShellIconButton} from './ShellHeader.jsx';
import { Button, Icon, Tooltip } from '@blueprintjs/core';
import WindowContent from './WorkspaceWindowContent.jsx';
import { Icon as SharedIcon, SelectControl, typographyProps } from 'forge/components';
import AppRenderer from './mcpApps/AppRenderer.jsx';

const ICONS = new Set(['application', 'chart', 'dashboard', 'document', 'folder-open', 'grid-view']);

function humanize(value = '') {
  return String(value || '')
    .replace(/[\/_-]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

export function resolveWorkspaceNavigation(windowEntry = null) {
  const navigation = windowEntry?.navigation && typeof windowEntry.navigation === 'object'
    ? windowEntry.navigation
    : {};
  const windowKey = String(windowEntry?.windowKey || '').trim();
  const metadata = findMetadataSignal(String(windowEntry?.windowId || '').trim())?.value;
  const windowTitle = resolveWorkspaceWindowTitle(windowEntry, metadata);
  const explicitLabel = String(navigation?.label || '').trim();
  const label = explicitLabel
    || (windowTitle && windowTitle.toLowerCase() !== windowKey.toLowerCase() ? windowTitle : '')
    || humanize(windowKey)
    || 'Workspace';
  const candidateIcon = String(navigation?.icon || '').trim().toLowerCase();
  return {
    label,
    chipName: String(navigation?.chipName || '').trim() || label,
    chipIcon: ICONS.has(String(navigation?.chipIcon || '').trim().toLowerCase())
      ? String(navigation.chipIcon).trim().toLowerCase()
      : '',
    icon: ICONS.has(candidateIcon) ? candidateIcon : 'application',
    subtitle: String(navigation?.subtitle || '').trim(),
    supportingText: String(navigation?.supportingText || '').trim(),
    tooltip: String(navigation?.tooltip || '').trim(),
    accent: String(navigation?.accent || '').trim(),
  };
}

export function resolveWorkspaceBreadcrumbs(windowEntry = null) {
  if (!windowEntry) return [];
  const trail = Array.isArray(windowEntry?.navigationTrail) ? windowEntry.navigationTrail : [];
  const ancestors = trail
    .map((entry, index) => {
      const navigation = resolveWorkspaceNavigation(entry);
      const windowId = String(entry?.windowId || '').trim();
      return windowId ? { index, windowId, label: navigation.chipName, icon: navigation.chipIcon, current: false } : null;
    })
    .filter(Boolean);
  const currentNavigation = resolveWorkspaceNavigation(windowEntry);
  return [
    ...ancestors,
    {
      index: trail.length,
      windowId: String(windowEntry?.windowId || '').trim(),
      label: currentNavigation.chipName,
      icon: currentNavigation.chipIcon,
      current: true,
    },
  ];
}

export function resolveChatWindowRenderKey(chatWindow = null) {
  const windowId = String(chatWindow?.windowId || 'chat').trim() || 'chat';
  const instanceVersion = Math.max(0, Number(chatWindow?.conversationInstanceVersion || 0));
  return `${windowId}:${instanceVersion}`;
}

export const shouldShowWorkspaceTabs=sharedShouldShowWorkspaceTabs;

export default function ConversationWorkspaceSurface({
  presentation,
  savedOutputs = [],
  conversationId,
  onOpenSavedOutput,
  headerMount,
  headerActive = true,
  activeSurface = 'conversation',
  unreadCount = 0,
  chatRunning = false,
  workspaceMode = 'focus',
  onChangeWorkspaceMode,
  chatWindow = null,
  renderConversation,
  renderWorkspace,
  workspaceWindow = null,
  workspaceTabs = [],
  workspaceTabsVisibility = 'auto',
  workspaceWindows = [],
  onCloseWorkspaceTab,
  onWorkspaceLifecycle,
  suppressConversationWorkspaceLink = false,
  onOpenWorkspace,
  onBackToConversation,
  onNavigateWorkspaceBreadcrumb,
  onCloseWorkspace,
  onSelectWorkspaceTab,
}) {
  const documentPresentation = presentation === 'document';
  const hasWorkspace = !!workspaceWindow;
  const showObjectTabs = shouldShowWorkspaceTabs(workspaceTabs.length, workspaceTabsVisibility);
  const workspaceActive = hasWorkspace && activeSurface === 'workspace';
  const contentRef = useWorkspaceScroll(workspaceWindow?.windowId, workspaceActive);
  const [compact, setCompact] = useState(false);
  useEffect(() => {
    const query = window.matchMedia(documentPresentation ? '(max-width: 1179px)' : '(max-width: 600px)');
    const update = () => setCompact(query.matches);
    update(); query.addEventListener('change', update);
    return () => query.removeEventListener('change', update);
  }, [documentPresentation]);
  const capabilities = workspaceWindow?.workspaceObject?.capabilities || {};
  const effectiveMode = compact || capabilities.split === false ? 'focus' : workspaceMode;
  const navigation = resolveWorkspaceNavigation(workspaceWindow);
  const breadcrumbs = resolveWorkspaceBreadcrumbs(workspaceWindow);
  const headingRef = useRef(null);
  const keyboardNavigationRef = useRef(false);
  const invokingRef = useRef(null);
  const wasActiveRef = useRef(false);
  useEffect(() => {
    if (workspaceActive) {
      if (!wasActiveRef.current) invokingRef.current = document.activeElement;
      const marker = [...document.querySelectorAll('[data-testid="workspace-attachment-card"]')]
        .find((element) => element.dataset.workspaceWindowId === workspaceWindow?.windowId);
      if (marker) invokingRef.current = marker;
      if (!keyboardNavigationRef.current) headingRef.current?.focus({ preventScroll: true });
      keyboardNavigationRef.current = false;
    } else if (invokingRef.current?.isConnected) {
      invokingRef.current.focus({ preventScroll: true });
      invokingRef.current.scrollIntoView?.({ block: 'nearest' });
    }
    wasActiveRef.current = workspaceActive;
  }, [workspaceActive, workspaceWindow?.windowId]);
  const [composerExpanded, setComposerExpanded] = useState(false);
  useEffect(() => {
    if (workspaceActive && effectiveMode === 'focus') setComposerExpanded(false);
  }, [workspaceActive, effectiveMode, workspaceWindow?.windowId]);

  return (
    <div className={`app-summary-surface-shell${documentPresentation ? ' is-document' : ''}${workspaceActive ? ` is-workspace${effectiveMode === 'split' ? ' is-split' : ''}` : ' is-conversation'}`} data-active-surface={workspaceActive ? 'workspace' : 'conversation'}>
      {documentPresentation && headerMount && headerActive && (hasWorkspace || savedOutputs.length > 0) ? createPortal(<>
        {workspaceActive ? <WorkspaceRuntimeTabs id="conversation-workspace"
          entries={(workspaceTabs.length ? workspaceTabs : [{windowId: workspaceWindow.windowId, label: navigation.label}]).map(tab => ({...tab, closable: (workspaceWindows.find(entry => entry.windowId === tab.windowId) || workspaceWindow)?.workspaceObject?.capabilities?.close !== false}))}
          selectedId={workspaceWindow.windowId} onSelect={onSelectWorkspaceTab} onClose={onCloseWorkspaceTab || onCloseWorkspace}
          visibility={workspaceTabsVisibility === 'never' ? 'never' : 'always'} ariaLabel="Open workspace tabs"
          className="app-shell-viewer-tabs" itemClassName="app-shell-viewer-tab"
          onKeyboardSelect={() => {keyboardNavigationRef.current = true;}}
          renderLabel={tab => <><SharedIcon name="lucide:file-text"/><span {...typographyProps('label')} title={tab.label}>{tab.label}</span></>}
          renderClose={(tab, {close, disabled}) => <button type="button" className="app-shell-tab-close" aria-label={`Close ${tab.label}`} disabled={disabled} onClick={close}><SharedIcon name="lucide:x"/></button>}/> : null}
        {workspaceActive && workspaceTabsVisibility === 'never' ? <SelectControl {...typographyProps('label')} aria-label="Choose workspace" value={workspaceWindow.windowId} onChange={event => onSelectWorkspaceTab?.(event.target.value)}>
          {(workspaceTabs.length ? workspaceTabs : [{windowId: workspaceWindow.windowId, label: navigation.label}]).map(tab => <option key={tab.windowId} value={tab.windowId}>{tab.label}</option>)}
        </SelectControl> : null}
        <SavedOutputInventory entries={savedOutputs} conversationId={conversationId} onOpen={onOpenSavedOutput}/>
        <div className="app-shell-viewer-controls">
          <span className="app-shell-viewer-status" {...typographyProps('metadata')} role="status" aria-live="polite">{chatRunning ? 'Working…' : unreadCount > 0 ? `${unreadCount} new` : ''}</span>
          {workspaceActive ? <ShellIconButton icon={effectiveMode === 'focus' ? 'lucide:minimize' : 'lucide:maximize'}
            className="app-shell-viewer-expand" label={effectiveMode === 'focus' ? 'Restore split view' : 'Expand workspace'}
            aria-pressed={effectiveMode === 'focus'} disabled={capabilities.split === false}
            onClick={() => onChangeWorkspaceMode?.(effectiveMode === 'focus' ? 'split' : 'focus')}/> : null}
          {hasWorkspace ? <ShellIconButton icon="lucide:panel-right" label={workspaceActive ? 'Hide workspace' : 'Show workspace'}
            aria-expanded={workspaceActive} aria-controls="conversation-workspace-viewer" onClick={workspaceActive ? onBackToConversation : onOpenWorkspace}/> : null}
        </div>
      </>, headerMount) : null}
      {!documentPresentation && !workspaceActive && hasWorkspace && !suppressConversationWorkspaceLink ? (
        <div className="app-summary-surface-navigation">
          <Button
            minimal
            small
            icon={navigation.icon}
            text={navigation.label}
            aria-label={`Return to ${navigation.label}`}
            onClick={onOpenWorkspace}
          />
        </div>
      ) : null}

      {hasWorkspace ? (
        <section id="conversation-workspace-viewer" className={`app-summary-workspace${workspaceActive ? '' : ' is-surface-hidden'}`} aria-label={`${navigation.label} workspace`} data-workspace-window-id={workspaceWindow?.windowId || ''} aria-hidden={!workspaceActive} inert={!workspaceActive ? '' : undefined}>
          {!documentPresentation ? <header className="app-summary-workspace-header">
            <div className="app-summary-workspace-header-actions">
              <nav className="app-workspace-breadcrumbs" aria-label="Workspace navigation">
                <Tooltip content={unreadCount > 0 ? `Chat · ${unreadCount} new` : 'Chat'} placement="bottom" hoverOpenDelay={200}>
                  <Button
                    minimal
                    small
                    icon="chat"
                    className="app-summary-workspace-chat-action app-workspace-breadcrumb-chip is-icon-only"
                    aria-label="Return to chat"
                    title={unreadCount > 0 ? `Chat · ${unreadCount} new` : 'Chat'}
                    onClick={onBackToConversation}
                  />
                </Tooltip>
                {breadcrumbs.map((breadcrumb) => (
                  <React.Fragment key={`${breadcrumb.windowId}:${breadcrumb.index}`}>
                    <span className="app-workspace-breadcrumb-separator" aria-hidden="true">›</span>
                    {breadcrumb.current ? (
                      <Tooltip content={breadcrumb.label} placement="bottom" hoverOpenDelay={200}>
                        <span className={`app-workspace-breadcrumb-chip is-current${breadcrumb.icon ? ' is-icon-only' : ''}`} aria-current="page" aria-label={breadcrumb.label} title={breadcrumb.label}>
                          {breadcrumb.icon ? <Icon icon={breadcrumb.icon} size={12} /> : null}
                          {!breadcrumb.icon ? breadcrumb.label : null}
                        </span>
                      </Tooltip>
                    ) : (
                      <Tooltip content={breadcrumb.label} placement="bottom" hoverOpenDelay={200}>
                        <button
                          type="button"
                          className={`app-workspace-breadcrumb-chip${breadcrumb.icon ? ' is-icon-only' : ''}`}
                          aria-label={breadcrumb.label}
                          title={breadcrumb.label}
                          onClick={() => onNavigateWorkspaceBreadcrumb?.(breadcrumb.index)}
                        >
                          {breadcrumb.icon ? <Icon icon={breadcrumb.icon} size={12} /> : null}
                          {!breadcrumb.icon ? breadcrumb.label : null}
                        </button>
                      </Tooltip>
                    )}
                  </React.Fragment>
                ))}
              </nav>
            </div>
            <div className="app-summary-workspace-identity">
              <span className="app-summary-workspace-title-copy">
                <span ref={headingRef} tabIndex={-1} role="heading" aria-level={1} className="app-summary-workspace-title">{navigation.label}</span>
                {!showObjectTabs && workspaceTabs.length > 1 ? <select aria-label="Choose workspace"
                  value={workspaceWindow?.windowId || ''} onChange={(event) => { keyboardNavigationRef.current = true; onSelectWorkspaceTab?.(event.target.value); }}>
                  {workspaceTabs.map((tab) => <option key={tab.windowId} value={tab.windowId}>{tab.label}</option>)}
                </select> : null}
              </span>
            </div>
            <div className="app-summary-workspace-context">
              <span className="app-workspace-conversation-status" role="status" aria-live="polite">
                {chatRunning ? 'Working…' : unreadCount > 0 ? `${unreadCount} new` : ''}
              </span>
              <div className="app-workspace-window-controls" role="group" aria-label="Workspace controls">
                <button type="button" className="app-workspace-window-control is-close"
                  disabled={capabilities.close === false}
                  aria-label={`Close ${navigation.label}`} title={`Close ${navigation.label}`} onClick={onCloseWorkspace}>
                  <span className="app-workspace-control-dot" aria-hidden="true">
                    <svg viewBox="0 0 10 10"><path d="M2 2l6 6M8 2L2 8" /></svg>
                  </span>
                </button>
                <button type="button" className="app-workspace-window-control is-layout"
                  disabled={compact || capabilities.split === false}
                  aria-label={compact ? 'Workspace is maximized on small screens' : effectiveMode === 'focus' ? 'Restore split view' : 'Maximize workspace'}
                  aria-pressed={effectiveMode === 'focus'}
                  title={compact ? 'Workspace is maximized on small screens' : capabilities.split === false ? 'Split layout unavailable' : effectiveMode === 'focus' ? 'Restore split view' : 'Maximize workspace'}
                  onClick={() => onChangeWorkspaceMode?.(effectiveMode === 'focus' ? 'split' : 'focus')}>
                  <span className="app-workspace-control-dot" aria-hidden="true">
                    <svg viewBox="0 0 10 10"><rect x="1.5" y="1.5" width="7" height="7" rx="0.6" />{effectiveMode === 'focus' ? <path d="M5 1.5v7" /> : null}</svg>
                  </span>
                </button>
              </div>
            </div>
          </header> : <span ref={headingRef} tabIndex={-1} role="heading" aria-level={2} className="app-shell-focus-heading">{navigation.label}</span>}
          {documentPresentation && breadcrumbs.length > 1 ? <nav aria-label="Workspace navigation" className="app-shell-ancestor-navigation">{breadcrumbs.filter(entry => !entry.current).map(entry => <button type="button" {...typographyProps('label')} key={entry.windowId} onClick={() => onNavigateWorkspaceBreadcrumb?.(entry.index)}>{entry.label}<SharedIcon name="lucide:chevron-right"/></button>)}</nav> : null}
          {!documentPresentation ? <WorkspaceRuntimeTabs id="conversation-workspace" entries={workspaceTabs.map(tab=>({...tab,closable:workspaceWindows.find(entry=>entry.windowId===tab.windowId)?.workspaceObject?.capabilities?.close!==false}))}
            selectedId={workspaceWindow?.windowId} onSelect={onSelectWorkspaceTab} onClose={onCloseWorkspaceTab}
            visibility={workspaceTabsVisibility} className="app-window-split-workspace-tabs" itemClassName="app-workspace-object-chip" tabClassName="app-window-split-workspace-tab"
            onKeyboardSelect={()=>{keyboardNavigationRef.current=true;}}
            renderLabel={tab=><><Icon icon="application" size={12}/> {tab.label}</>}/> : null}
          <div ref={contentRef} className="app-summary-workspace-content">
            <WorkspaceRuntimePanels id="conversation-workspace" entries={workspaceWindows.length?workspaceWindows:[workspaceWindow]} selectedId={workspaceWindow?.windowId} active={workspaceActive}
              preloadContent={entry=>entry.hostOpenState==='fresh' && entry.workspaceObject?.lifecycle?.state==='opening'}
              className="app-workspace-renderer" panelProps={entry=>({'data-workspace-renderer-id':entry.windowId})}
              getPresentation={entry=>({label:resolveWorkspaceNavigation(entry).label,kind:entry.workspaceObject?.kind || 'resource'})}
              renderContent={entry=>renderWorkspace ? renderWorkspace(entry) : entry.workspaceObject?.content?.renderer==='toolFeed'?<ToolFeedDetail key={`${entry.windowId}:${entry.workspaceObject?.lifecycle?.attempt || 0}`} variant="rail" placement="workspace"
                hostedFeedId={entry.workspaceObject.content.feedId} conversationId={entry.conversationId} onReturn={onBackToConversation} onLifecycle={state=>onWorkspaceLifecycle?.(entry.windowId,state)}/>
                :entry.workspaceObject?.content?.renderer==='inlineReport'?<InlineReportWorkspace presentation={presentation} key={`${entry.windowId}:${entry.workspaceObject?.lifecycle?.attempt || 0}`} entry={entry} onReturn={onBackToConversation} onLifecycle={state=>onWorkspaceLifecycle?.(entry.windowId,state)}/>
                :entry.mcpUI?.uri?<AppRenderer uri={entry.mcpUI.uri} title={entry.mcpUI.title || navigation.label} conversationId={entry.conversationId || ''} hosted onLifecycle={state=>onWorkspaceLifecycle?.(entry.windowId,state)}/>
                :<WindowContent window={entry} isInTab/>}/>

          </div>
        </section>
      ) : null}

      <section className={`app-summary-conversation${!documentPresentation && workspaceActive && effectiveMode !== 'split' ? ` is-composer-only${composerExpanded ? ' is-composer-expanded' : ''}` : ''}`} aria-label="Conversation">
        {!documentPresentation && workspaceActive && effectiveMode === 'focus' ? (
          <Button
            minimal
            small
            icon={composerExpanded ? 'chevron-down' : 'chevron-up'}
            className="app-workspace-composer-toggle"
            aria-expanded={composerExpanded}
            aria-label={composerExpanded ? 'Hide composer' : 'Show composer'}
            title={composerExpanded ? 'Hide composer' : 'Show composer'}
            onClick={() => setComposerExpanded((expanded) => !expanded)}
          />
        ) : null}
        {renderConversation ? renderConversation() : (chatWindow ? <WindowContent key={resolveChatWindowRenderKey(chatWindow)} window={chatWindow} isInTab /> : null)}
      </section>
    </div>
  );
}
