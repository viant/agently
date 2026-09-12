import {useWorkspaceScroll} from '../services/useWorkspaceScroll.js';
import { WorkspacePresentationProvider } from 'forge/core';
import ToolFeedDetail from './ToolFeedDetail.jsx';
import React, { useState, useEffect, useRef } from 'react';
import { Button, Icon } from '@blueprintjs/core';
import { WindowContent } from 'forge/components';
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
  const windowTitle = String(windowEntry?.windowTitle || '').trim();
  const explicitLabel = String(navigation?.label || '').trim();
  const label = explicitLabel
    || (windowTitle && windowTitle.toLowerCase() !== windowKey.toLowerCase() ? windowTitle : '')
    || humanize(windowKey)
    || 'Workspace';
  const candidateIcon = String(navigation?.icon || '').trim().toLowerCase();
  return {
    label,
    icon: ICONS.has(candidateIcon) ? candidateIcon : 'application',
    subtitle: String(navigation?.subtitle || '').trim(),
    supportingText: String(navigation?.supportingText || '').trim(),
    tooltip: String(navigation?.tooltip || '').trim(),
    accent: String(navigation?.accent || '').trim(),
  };
}

export function resolveChatWindowRenderKey(chatWindow = null) {
  const windowId = String(chatWindow?.windowId || 'chat').trim() || 'chat';
  const instanceVersion = Math.max(0, Number(chatWindow?.conversationInstanceVersion || 0));
  return `${windowId}:${instanceVersion}`;
}

export function shouldShowWorkspaceTabs(count, visibility = 'auto') {
  if (count < 1 || visibility === 'never') return false;
  return visibility === 'always' || count > 1;
}

export default function ConversationWorkspaceSurface({
  activeSurface = 'conversation',
  unreadCount = 0,
  chatRunning = false,
  workspaceMode = 'focus',
  onChangeWorkspaceMode,
  chatWindow = null,
  renderConversation,
  workspaceWindow = null,
  workspaceTabs = [],
  workspaceTabsVisibility = 'auto',
  workspaceWindows = [],
  onCloseWorkspaceTab,
  onWorkspaceLifecycle,
  suppressConversationWorkspaceLink = false,
  onOpenWorkspace,
  onBackToConversation,
  onCloseWorkspace,
  onSelectWorkspaceTab,
}) {
  const hasWorkspace = !!workspaceWindow;
  const showObjectTabs = shouldShowWorkspaceTabs(workspaceTabs.length, workspaceTabsVisibility);
  const workspaceActive = hasWorkspace && activeSurface === 'workspace';
  const contentRef = useWorkspaceScroll(workspaceWindow?.windowId, workspaceActive);
  const [compact, setCompact] = useState(false);
  useEffect(() => {
    const query = window.matchMedia('(max-width: 600px)');
    const update = () => setCompact(query.matches);
    update(); query.addEventListener('change', update);
    return () => query.removeEventListener('change', update);
  }, []);
  const capabilities = workspaceWindow?.workspaceObject?.capabilities || {};
  const effectiveMode = compact || capabilities.split === false ? 'focus' : workspaceMode;
  const navigation = resolveWorkspaceNavigation(workspaceWindow);
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

  return (
    <div className={`app-summary-surface-shell${workspaceActive ? ` is-workspace${effectiveMode === 'split' ? ' is-split' : ''}` : ' is-conversation'}`} data-active-surface={workspaceActive ? 'workspace' : 'conversation'}>
      {!workspaceActive && hasWorkspace && !suppressConversationWorkspaceLink ? (
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
        <section className={`app-summary-workspace${workspaceActive ? '' : ' is-surface-hidden'}`} aria-label={`${navigation.label} workspace`} data-workspace-window-id={workspaceWindow?.windowId || ''} aria-hidden={!workspaceActive} inert={!workspaceActive ? '' : undefined}>
          <header className="app-summary-workspace-header">
            <div className="app-summary-workspace-header-actions">
              <Button
                minimal
                small
                icon="chat"
                text={unreadCount > 0 ? `Chat · ${unreadCount}` : 'Chat'}
                className="app-summary-workspace-chat-action"
                aria-label="Return to chat"
                title="Return to chat"
                onClick={onBackToConversation}
              />
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
                  aria-label={compact ? 'Focus layout on small screens' : effectiveMode === 'focus' ? 'Show chat and workspace' : 'Focus workspace'}
                  title={compact ? 'Focus layout on small screens' : capabilities.split === false ? 'Split layout unavailable' : effectiveMode === 'focus' ? 'Split view — show chat and workspace' : 'Focus workspace'}
                  onClick={() => onChangeWorkspaceMode?.(effectiveMode === 'focus' ? 'split' : 'focus')}>
                  <span className="app-workspace-control-dot" aria-hidden="true">
                    <svg viewBox="0 0 10 10"><rect x="1.5" y="1.5" width="7" height="7" rx="0.6" />{effectiveMode === 'focus' ? <path d="M5 1.5v7" /> : null}</svg>
                  </span>
                </button>
              </div>
            </div>
          </header>
          {showObjectTabs ? (
            <div className="app-window-split-workspace-tabs" role="tablist" aria-label="Open workspaces">
              {workspaceTabs.map((tab, index) => (
                <span key={tab.windowId} className="app-workspace-object-chip">
                <button
                  key={tab.windowId}
                  type="button"
                  role="tab"
                  aria-selected={tab.isActive}
                  tabIndex={tab.isActive ? 0 : -1}
                  onKeyDown={(event) => {
                    const delta = event.key === 'ArrowRight' ? 1 : event.key === 'ArrowLeft' ? -1 : 0;
                    if (!delta && event.key !== 'Home' && event.key !== 'End') return;
                    event.preventDefault();
                    const next = event.key === 'Home' ? 0 : event.key === 'End' ? workspaceTabs.length - 1 : (index + delta + workspaceTabs.length) % workspaceTabs.length;
                    keyboardNavigationRef.current = true;
                    onSelectWorkspaceTab?.(workspaceTabs[next].windowId);
                    event.currentTarget.closest('[role="tablist"]')?.querySelectorAll('[role="tab"]')[next]?.focus();
                  }}
                  className={`app-window-split-workspace-tab${tab.isActive ? ' is-active' : ''}`}
                  onClick={() => onSelectWorkspaceTab?.(tab.windowId)}
                >
                  <Icon icon="application" size={12} /> {tab.label}
                </button>
                <button type="button" disabled={workspaceWindows.find((entry) => entry.windowId === tab.windowId)?.workspaceObject?.capabilities?.close === false} aria-label={`Close ${tab.label}`} onClick={() => onCloseWorkspaceTab?.(tab.windowId)}>×</button>
                </span>
              ))}
            </div>
          ) : null}
          <div ref={contentRef} className="app-summary-workspace-content">
            {(workspaceWindows.length ? workspaceWindows : [workspaceWindow]).map((entry) => {
              const visible = workspaceActive && entry.windowId === workspaceWindow?.windowId;
              return <WorkspacePresentationProvider key={entry.windowId} value={{label: resolveWorkspaceNavigation(entry).label, kind: entry.workspaceObject?.kind || 'resource'}}><div data-workspace-renderer-id={entry.windowId} hidden={!visible} inert={!visible ? '' : undefined} className="app-workspace-renderer">
                {entry.workspaceObject?.content?.renderer === 'toolFeed' ? <ToolFeedDetail
                  hostedFeedId={entry.workspaceObject.content.feedId} conversationId={entry.conversationId}
                  onLifecycle={(state) => onWorkspaceLifecycle?.(entry.windowId, state)} />
                  : entry.mcpUI?.uri ? <AppRenderer uri={entry.mcpUI.uri} title={entry.mcpUI.title || navigation.label}
                  conversationId={entry.conversationId || ''} hosted onLifecycle={(state) => onWorkspaceLifecycle?.(entry.windowId, state)} />
                  : <WindowContent window={entry} isInTab />}
              </div></WorkspacePresentationProvider>;
            })}
          </div>
        </section>
      ) : null}

      <section className={`app-summary-conversation${workspaceActive && effectiveMode !== 'split' ? ` is-composer-only${composerExpanded ? ' is-composer-expanded' : ''}` : ''}`} aria-label="Conversation">
        {workspaceActive ? (
          <Button
            minimal
            small
            icon={composerExpanded ? 'chevron-down' : 'chevron-up'}
            className="app-workspace-composer-toggle"
            aria-label={composerExpanded ? 'Collapse composer options' : 'Expand composer options'}
            title={composerExpanded ? 'Collapse composer options' : 'Expand composer options'}
            onClick={() => setComposerExpanded((expanded) => !expanded)}
          />
        ) : null}
        {renderConversation ? renderConversation() : <WindowContent key={resolveChatWindowRenderKey(chatWindow)} window={chatWindow} isInTab />}
      </section>
    </div>
  );
}
