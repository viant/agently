import React, {useEffect, useRef, useState} from 'react';
import {Icon, typographyProps} from 'forge/components';
import {resolveWorkspaceNavigation} from './ConversationWorkspaceSurface.jsx';

export function resolveSavedOutputs(entries = [], conversationId = '') {
  const outputs = new Map();
  for (const entry of entries) {
    const object = entry.workspaceObject;
    if (!entry.windowId || !object?.objectId || object.capabilities?.reopen === false) continue;
    if (conversationId && (object.conversationId || entry.conversationId) !== conversationId) continue;
    const previous = outputs.get(object.objectId);
    if (!previous || Number(object.revision || 0) >= Number(previous.workspaceObject.revision || 0)) outputs.set(object.objectId, entry);
  }
  return [...outputs.values()];
}

export default function SavedOutputInventory({entries = [], conversationId, onOpen}) {
  const outputs = resolveSavedOutputs(entries, conversationId);
  const ref = useRef(null);
  const [open, setOpen] = useState(false);
  useEffect(() => {
    const outside = event => {if (!ref.current?.contains(event.target)) ref.current?.removeAttribute('open');};
    document.addEventListener('pointerdown', outside);
    return () => document.removeEventListener('pointerdown', outside);
  }, []);
  useEffect(() => {ref.current?.removeAttribute('open');}, [conversationId]);
  if (!outputs.length) return null;
  return <details className="app-saved-outputs" ref={ref} onToggle={event => setOpen(event.currentTarget.open)} onKeyDown={event => {
    if (event.key !== 'Escape' || !ref.current?.open) return;
    event.preventDefault(); event.stopPropagation(); ref.current.open = false; ref.current.querySelector('summary')?.focus();
  }}>
    <summary role="button" aria-label={`Saved outputs (${outputs.length})`} aria-expanded={open} {...typographyProps('label')}><Icon name="lucide:file-text"/><span>Outputs</span><span {...typographyProps('metadata')}>{outputs.length}</span></summary>
    <div className="app-saved-output-list" role="group" aria-label="Saved outputs">
      {outputs.map(entry => {
        const object = entry.workspaceObject;
        const state = object.lifecycle?.state || 'ready';
        const name = resolveWorkspaceNavigation(entry).label;
        return <button type="button" key={object.objectId} onClick={() => {ref.current.open = false; onOpen?.(entry);}} aria-label={`Open ${name}`}>
          <Icon name="lucide:file-text"/><span><strong {...typographyProps('body')}>{name}</strong><span {...typographyProps('metadata')}>{state === 'closed' ? 'Ready' : state === 'failed' ? 'Needs attention' : state === 'opening' ? 'Preparing' : state === 'stale' ? 'Refresh when opened' : 'Ready'}{object.revision ? ` · Version ${object.revision}` : ''}</span></span>
        </button>;
      })}
    </div>
  </details>;
}
