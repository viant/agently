import React from 'react';
import { Icon } from '@blueprintjs/core';
import { resolveWorkspaceNavigation } from '../ConversationWorkspaceSurface.jsx';

export default function WorkspaceAttachmentCard({ workspaceWindow = null, onOpen }) {
  if (!workspaceWindow) return null;
  const navigation = resolveWorkspaceNavigation(workspaceWindow);
  const detail = navigation.supportingText
    || navigation.subtitle
    || `Open the ${navigation.label} workspace.`;
  return (
    <button
      type="button"
      className="app-workspace-attachment"
      data-testid="workspace-attachment-card"
      data-workspace-window-id={workspaceWindow?.windowId || ''}
      aria-label={`Open ${navigation.label}`}
      title={navigation.tooltip || `Open ${navigation.label}`}
      onClick={onOpen}
    >
      <span className="app-workspace-attachment-icon" aria-hidden="true">
        <Icon icon={navigation.icon} size={17} />
      </span>
      <span className="app-workspace-attachment-copy">
        <strong>{navigation.label}</strong>
        <span>{detail}</span>
      </span>
      <span className="app-workspace-attachment-arrow" aria-hidden="true">→</span>
    </button>
  );
}
