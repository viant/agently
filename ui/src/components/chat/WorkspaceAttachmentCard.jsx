import React, {useContext} from 'react';
import {ConversationViewContext} from '../../context/ConversationViewContext.js';
import {Icon, typographyProps} from 'forge/components';
import { resolveWorkspaceNavigation } from '../ConversationWorkspaceSurface.jsx';

export default function WorkspaceAttachmentCard({ workspaceWindow = null, onOpen }) {
  const documentAppearance = useContext(ConversationViewContext)?.presentation === 'document';
  if (!workspaceWindow) return null;
  const navigation = resolveWorkspaceNavigation(workspaceWindow);
  const lifecycle = workspaceWindow.workspaceObject?.lifecycle?.state || 'ready';
  const object = workspaceWindow.workspaceObject;
  const unavailable = object?.capabilities?.reopen === false;
  const caption = lifecycle === 'ready' || lifecycle === 'closed'
    ? object?.kind === 'report' ? `Report${object.revision ? ` · Version ${object.revision}` : ''}` : 'Ready'
    : lifecycle === 'opening' ? 'Preparing' : lifecycle === 'failed' ? 'Needs attention' : 'Refresh when opened';
  const action = lifecycle === 'closed' || lifecycle === 'stale' ? 'Reopen' : lifecycle === 'failed' ? 'Retry' : 'Show';
  return (
    <button
      type="button"
      className={`app-workspace-attachment${documentAppearance ? ' is-document' : ''}`}
      disabled={unavailable}
      data-testid="workspace-attachment-card"
      data-workspace-window-id={workspaceWindow?.windowId || ''}
      data-workspace-object-id={workspaceWindow?.workspaceObject?.objectId || ''}
      aria-label={`${action} ${navigation.label}`}
      title={navigation.tooltip || `Open ${navigation.label}`}
      onClick={onOpen}
    >
      <span className="app-workspace-attachment-icon" aria-hidden="true">
        <Icon name="lucide:file-text"/>
      </span>
      <span className="app-workspace-attachment-copy">
        <strong {...typographyProps(documentAppearance ? 'label' : 'body')}>{navigation.label}</strong>
        <span {...typographyProps('metadata')}>{unavailable ? 'Unavailable' : caption}</span>
      </span>
      <span className="app-workspace-attachment-arrow" aria-hidden="true">{documentAppearance ? <Icon name="lucide:arrow-right"/> : action}</span>
    </button>
  );
}
