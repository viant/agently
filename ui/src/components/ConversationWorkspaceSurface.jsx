import React from 'react';
import { Button } from '@blueprintjs/core';
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

export default function ConversationWorkspaceSurface({
  activeSurface = 'conversation',
  chatWindow = null,
  workspaceWindow = null,
  workspaceTabs = [],
  suppressConversationWorkspaceLink = false,
  onOpenWorkspace,
  onBackToConversation,
  onCloseWorkspace,
  onSelectWorkspaceTab,
}) {
  const hasWorkspace = !!workspaceWindow;
  const workspaceActive = hasWorkspace && activeSurface === 'workspace';
  const navigation = resolveWorkspaceNavigation(workspaceWindow);

  return (
    <div className={`app-summary-surface-shell${workspaceActive ? ' is-workspace' : ' is-conversation'}`} data-active-surface={workspaceActive ? 'workspace' : 'conversation'}>
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
        <section className={`app-summary-workspace${workspaceActive ? '' : ' is-surface-hidden'}`} aria-label={`${navigation.label} workspace`} data-workspace-window-id={workspaceWindow?.windowId || ''} aria-hidden={!workspaceActive}>
          <header className="app-summary-workspace-header">
            <div className="app-summary-workspace-header-actions">
              <button
                type="button"
                className="app-window-dot app-window-dot-close"
                aria-label={`Close ${navigation.label}`}
                title={`Close ${navigation.label}`}
                onClick={onCloseWorkspace}
              />
              <Button minimal small icon="arrow-left" text="Conversation" aria-label="Back to Conversation" onClick={onBackToConversation} />
            </div>
            <div className="app-summary-workspace-title">{navigation.label}</div>
          </header>
          {workspaceTabs.length > 1 ? (
            <div className="app-window-split-workspace-tabs" role="tablist" aria-label="Workspace tabs">
              {workspaceTabs.map((tab) => (
                <button
                  key={tab.windowId}
                  type="button"
                  role="tab"
                  aria-selected={tab.isActive}
                  className={`app-window-split-workspace-tab${tab.isActive ? ' is-active' : ''}`}
                  onClick={() => onSelectWorkspaceTab?.(tab.windowId)}
                >
                  {tab.label}
                </button>
              ))}
            </div>
          ) : null}
          <div className="app-summary-workspace-content">
            {workspaceWindow?.mcpUI?.uri ? (
              <AppRenderer
                uri={workspaceWindow.mcpUI.uri}
                title={workspaceWindow.mcpUI.title || navigation.label}
                conversationId={workspaceWindow.conversationId || ''}
                hosted
              />
            ) : (
              <WindowContent key={String(workspaceWindow?.windowId || 'workspace')} window={workspaceWindow} isInTab />
            )}
          </div>
        </section>
      ) : null}

      <section className={`app-summary-conversation${workspaceActive ? ' is-composer-only' : ''}`} aria-label="Conversation">
        <WindowContent key={String(chatWindow?.windowId || 'chat')} window={chatWindow} isInTab />
      </section>
    </div>
  );
}
