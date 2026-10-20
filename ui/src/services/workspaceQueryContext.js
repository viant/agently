import { activeWindows, selectedWindowId, findViewSignal, findMetadataSignal, findInputSignal, findSelectionSignal } from 'forge/core';
import { getScopedActiveSurface, getScopedWorkspaceSelection } from './conversationWindow.js';
import { resolveWorkspaceWindowTitle } from './workspaceWindowTitle.js';

const scalar = (value) => typeof value === 'string' || typeof value === 'boolean'
  || (typeof value === 'number' && Number.isFinite(value));
const scalarEntries = (value) => Object.fromEntries(Object.entries(value || {}).filter(([, item]) => scalar(item)));
const snapshot = (value) => JSON.parse(JSON.stringify(value));
const contextParameters = (value) => Object.fromEntries(Object.entries(value || {})
  .sort(([a], [b]) => a.localeCompare(b))
  .filter(([, item]) => scalar(item) || (Array.isArray(item) && item.length > 0 && item.every(scalar)))
  .slice(0, 16)
  .map(([key, item]) => [key, Array.isArray(item) ? item.slice(0, 16) : item]));

// Read live signals for each submission. View hints are untrusted conversational
// context, never authorization, tool permissions, or datasource resource grants.
export function buildWorkspaceQueryContext(conversationId) {
  if (!conversationId || getScopedActiveSurface(conversationId) !== 'workspace') return {};
  const windows = (activeWindows.peek() || []).filter((item) => item.conversationId === conversationId);
  // Tab focus changes synchronously; persisted workspace selection can lag a
  // React effect. Composer focus may select chat, so retain the scoped fallback.
  const entry = windows.find((item) => item.windowId === selectedWindowId.peek() && item.windowKey !== 'chat/new')
    || windows.find((item) => item.windowId === getScopedWorkspaceSelection(conversationId));
  if (!entry || ['failed', 'closed'].includes(entry.workspaceObject?.lifecycle?.state)) return {};
  const windowId = entry.windowId;
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
    const filter = snapshot(input.filter || {});
    if (Object.keys(filter).length || Object.keys(selectedKeys).length || selectedIds.length) {
      sources[ref] = { filter, selectedKeys,
        ...(selectedIds.length > 0 ? {selectedIds} : {}) };
    }

  }
  const parameters = contextParameters(entry.parameters);
  const carousels = Object.fromEntries(Object.entries(view.carousels || {}).sort(([a], [b]) => a.localeCompare(b))
    .slice(0, 16).map(([id, selection]) => [id, scalarEntries(selection)]));
  const cardRails = Object.fromEntries(Object.entries(view.cardRails || {}).sort(([a], [b]) => a.localeCompare(b))
    .slice(0, 8).map(([id, rail]) => [id, {
      ...scalarEntries(rail),
      ...(rail.focusedItem ? {focusedItem: scalarEntries(rail.focusedItem)} : {}),
      ...(Array.isArray(rail.visibleItems) ? {visibleItems: rail.visibleItems.slice(0, 12).map(scalarEntries)} : {}),
    }]));
  return { workspace: {
    objectId: entry.workspaceObject?.objectId || `workspace:${windowId}`,
    windowId, ...(entry.windowKey ? {windowKey: entry.windowKey} : {}),
    title: resolveWorkspaceWindowTitle(entry, metadata) || entry.title || '',
    renderer: entry.workspaceObject?.content?.renderer || 'forgeWindow',
    ...(Object.keys(parameters).length ? {parameters: snapshot(parameters)} : {}),
    ...(Object.keys(carousels).length ? {carousels: snapshot(carousels)} : {}),
    ...(Object.keys(cardRails).length ? {cardRails: snapshot(cardRails)} : {}),
    internalTabs: scalarEntries(view.tabs), dataSources: sources,
  } };
}
