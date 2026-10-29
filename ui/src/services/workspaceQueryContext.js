import { activeWindows, selectedWindowId, findViewSignal, findMetadataSignal, findInputSignal, findSelectionSignal } from 'forge/core';
import { getScopedWorkspaceSelection } from './conversationWindow.js';
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

let lastFocusedWindowId = '';
const isWorkspaceWindow = entry => entry && entry.windowKey !== 'chat/new' && entry.windowId !== 'chat/new'
  && !['failed', 'closed'].includes(entry.workspaceObject?.lifecycle?.state);

// Keep only an identity pointer. Submission reads current state, never a stale
// copy of a previous user's window or another conversation's data.
export function trackWorkspaceQueryFocus() {
  const remember = () => {
    const windows = activeWindows.peek() || [];
    const focused = windows.find(entry => entry.windowId === selectedWindowId.peek());
    if (isWorkspaceWindow(focused)) lastFocusedWindowId = focused.windowId;
    else if (!windows.some(entry => entry.windowId === lastFocusedWindowId)) lastFocusedWindowId = '';
  };
  const stopWindows = activeWindows.subscribe(remember);
  const stopSelection = selectedWindowId.subscribe(remember);
  return () => { stopWindows(); stopSelection(); lastFocusedWindowId = ''; };
}

// Read live signals for each submission. View hints are untrusted conversational
// context, never authorization, tool permissions, or datasource resource grants.
export function buildWorkspaceQueryContext(conversationId) {
  if (!conversationId) return {};
  const windows = activeWindows.peek() || [];
  const eligible = entry => isWorkspaceWindow(entry) && (!entry.conversationId || entry.conversationId === conversationId);
  const entry = windows.find(item => item.windowId === selectedWindowId.peek() && eligible(item))
    || windows.find(item => item.windowId === lastFocusedWindowId && eligible(item))
    || windows.find(item => item.windowId === getScopedWorkspaceSelection(conversationId) && item.conversationId === conversationId && eligible(item));
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
    ...((entry.resourceRef?.uri || metadata.resource?.uri) ? {resourceUri: entry.resourceRef?.uri || metadata.resource.uri} : {}),
    title: resolveWorkspaceWindowTitle(entry, metadata) || entry.title || '',
    renderer: entry.workspaceObject?.content?.renderer || 'forgeWindow',
    ...(Object.keys(parameters).length ? {parameters: snapshot(parameters)} : {}),
    ...(Object.keys(carousels).length ? {carousels: snapshot(carousels)} : {}),
    ...(Object.keys(cardRails).length ? {cardRails: snapshot(cardRails)} : {}),
    internalTabs: scalarEntries(view.tabs), dataSources: sources,
  } };
}
