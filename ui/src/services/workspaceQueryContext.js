import { activeWindows, findViewSignal, findMetadataSignal, findInputSignal, findSelectionSignal } from 'forge/core';
import { getScopedActiveSurface, getScopedWorkspaceSelection } from './conversationWindow.js';

const scalar = (value) => typeof value === 'string' || typeof value === 'boolean'
  || (typeof value === 'number' && Number.isFinite(value));
const scalarEntries = (value) => Object.fromEntries(Object.entries(value || {}).filter(([, item]) => scalar(item)));
const snapshot = (value) => JSON.parse(JSON.stringify(value));

// Read live signals for each submission. View hints are untrusted conversational
// context, never authorization, tool permissions, or datasource resource grants.
export function buildWorkspaceQueryContext(conversationId) {
  if (!conversationId || getScopedActiveSurface(conversationId) !== 'workspace') return {};
  const windowId = getScopedWorkspaceSelection(conversationId);
  const entry = (activeWindows.peek() || []).find((item) => item.windowId === windowId && item.conversationId === conversationId);
  if (!entry || ['failed', 'closed'].includes(entry.workspaceObject?.lifecycle?.state)) return {};
  const metadata = findMetadataSignal(windowId)?.peek?.() || {};
  const view = findViewSignal(windowId)?.peek?.() || {};
  const sources = {};
  for (const ref of Object.keys(metadata.dataSource || {})) {
    const input = findInputSignal(`${windowId}DS${ref}`)?.peek?.() || {};
    const selection = findSelectionSignal(`${windowId}DS${ref}`)?.peek?.() || {};
    const keys = (metadata.dataSource[ref]?.uniqueKey || []).slice(0, 16);
    const rowKeys = (row) => Object.fromEntries(keys.flatMap((key) => {
      const field = typeof key === 'string' ? key : key?.field;
      return field && scalar(row?.[field]) ? [[field, row[field]]] : [];
    }));
    const selectedKeys = rowKeys(selection.selected);
    const selectedIds = (Array.isArray(selection.selection) ? selection.selection.slice(0, 100) : [])
      .map((item) => {
        const direct = rowKeys(item);
        return Object.keys(direct).length > 0 ? direct : rowKeys(item?.selected);
      })
      .filter((item) => Object.keys(item).length > 0);
    sources[ref] = { filter: snapshot(input.filter || {}), selectedKeys,
      ...(selectedIds.length > 0 ? {selectedIds} : {}) };

  }
  return { workspace: {
    objectId: entry.workspaceObject?.objectId || `workspace:${windowId}`,
    windowId, title: entry.title || metadata.title || '', renderer: entry.workspaceObject?.content?.renderer || 'forgeWindow',
    internalTabs: scalarEntries(view.tabs), dataSources: sources,
  } };
}
