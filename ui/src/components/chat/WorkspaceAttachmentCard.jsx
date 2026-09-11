import React from 'react';
import { Icon } from '@blueprintjs/core';
import { resolveWorkspaceNavigation } from '../ConversationWorkspaceSurface.jsx';

export default function WorkspaceAttachmentCard({ workspaceWindow = null, onOpen }) {
  if (!workspaceWindow) return null;
  const navigation = resolveWorkspaceNavigation(workspaceWindow);
  const lifecycle = workspaceWindow.workspaceObject?.lifecycle?.state || 'ready';
  const action = lifecycle === 'opening' ? 'Opening…' : lifecycle === 'closed' || lifecycle === 'stale' ? 'Reopen' : lifecycle === 'failed' ? 'Retry' : 'Show';
  return (
    <button
      type="button"
      className="app-workspace-attachment"
      data-testid="workspace-attachment-card"
      data-workspace-window-id={workspaceWindow?.windowId || ''}
      data-workspace-object-id={workspaceWindow?.workspaceObject?.objectId || ''}
      aria-label={`${action} ${navigation.label}`}
      title={navigation.tooltip || `Open ${navigation.label}`}
      disabled={lifecycle === 'opening'}
      onClick={onOpen}
    >
      <span className="app-workspace-attachment-icon" aria-hidden="true">
        <Icon icon={navigation.icon} size={17} />
      </span>
      <span className="app-workspace-attachment-copy">
        <strong>{navigation.label}</strong>
        <span>{lifecycle === 'ready' ? 'Ready' : lifecycle === 'failed' ? 'Needs attention' : lifecycle}</span>
      </span>
      <span className="app-workspace-attachment-arrow">{action}</span>
    </button>
  );
}
