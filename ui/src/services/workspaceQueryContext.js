import { activeWindows, findViewSignal, findMetadataSignal, findInputSignal, findSelectionSignal } from 'forge/core';
import { getScopedActiveSurface, getScopedWorkspaceSelection } from './conversationWindow.js';

// View hints are conversational context, never authorization or resource grants.
export function buildWorkspaceQueryContext(conversationId) {
  if (!conversationId || getScopedActiveSurface(conversationId) !== 'workspace') return {};
  const windowId = getScopedWorkspaceSelection(conversationId);
  const entry = (activeWindows.peek() || []).find((item) => item.windowId === windowId && item.conversationId === conversationId);
  if (!entry || entry.workspaceObject?.lifecycle?.state === 'failed') return {};
  const metadata = findMetadataSignal(windowId)?.peek?.() || {};
  const view = findViewSignal(windowId)?.peek?.() || {};
  const sources = {};
  for (const ref of Object.keys(metadata.dataSource || {})) {
    const input = findInputSignal(`${windowId}DS${ref}`)?.peek?.() || {};
    const selection = findSelectionSignal(`${windowId}DS${ref}`)?.peek?.() || {};
    const keys = metadata.dataSource[ref]?.uniqueKey || [];
    const selectedKeys = {};
    for (const key of keys) {
      const field = typeof key === 'string' ? key : key.field;
      if (field && selection.selected?.[field] !== undefined) selectedKeys[field] = selection.selected[field];
    }
    sources[ref] = { filter: input.filter || {}, selectedKeys };
  }
  return { workspace: {
    objectId: entry.workspaceObject?.objectId || `workspace:${windowId}`,
    windowId, renderer: entry.workspaceObject?.content?.renderer || 'forgeWindow',
    internalTabs: view.tabs || {}, dataSources: sources,
  } };
}
