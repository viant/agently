let nextScopeEpoch = 0;
export function createStandaloneScopeId(baseId) {
  return `${baseId}:${++nextScopeEpoch}`;
}

// Presentation ownership only. Never an actor, permission or resource pin.
export function scopeWindows(windows, scopeId) {
  return (Array.isArray(windows) ? windows : []).filter(entry => entry?.windowScopeId === scopeId);
}

export function mountStandaloneRoot(store, scopeId, selection) {
  if (!scopeId || !selection?.windowKey) throw new Error('selected standalone window required');
  const root = store.addWindow(selection.windowTitle || selection.windowKey, null,
    selection.windowKey, undefined, true, structuredClone(selection.parameters || {}), {
      windowScopeId: scopeId,
      presentation: 'hosted',
      region: 'mcpui.standalone',
    });
  return applyStandaloneAdmission(store, scopeId, root.windowId, selection);
}

export function disposeStandaloneScope(store, scopeId, previousFocus = {}) {
  const owned = scopeWindows(store.activeWindows.peek(), scopeId);
  for (const entry of [...owned].reverse()) store.removeWindow(entry.windowId);
  for (const [name, id] of Object.entries(previousFocus)) {
    if (store[name] && store.activeWindows.peek().some(entry => entry.windowId === id)) store[name].value = id;
  }
}

export function closeStandaloneBranch(store, scopeId, windowId) {
  const owned = scopeWindows(store.activeWindows.peek(), scopeId);
  if (!owned.some(entry => entry.windowId === windowId)) return false;
  const ids = new Set([windowId]);
  let changed = true;
  while (changed) {
    changed = false;
    for (const entry of owned) if (ids.has(entry.parentKey) && !ids.has(entry.windowId)) {
      ids.add(entry.windowId); changed = true;
    }
  }
  for (const entry of [...owned].reverse()) if (ids.has(entry.windowId)) store.removeWindow(entry.windowId);
  return true;
}

// Install one complete selected frame without copying authority from another
// window. Store ids/ownership/presentation stay with the native lifecycle.
export function applyStandaloneAdmission(store, scopeId, windowId, selection) {
  const current = store.activeWindows.peek().find(entry => entry.windowId === windowId && entry.windowScopeId === scopeId);
  if (!current) return null;
  const next = {...current, ...structuredClone(selection),
    windowId: current.windowId, windowKey: current.windowKey,
    windowScopeId: scopeId, parentKey: current.parentKey,
    inTab: current.inTab, isModal: current.isModal,
    presentation: current.presentation, region: current.region,
    windowTitle: current.windowTitle || selection.windowTitle,
  };
  store.activeWindows.value = store.activeWindows.peek().map(entry => entry.windowId === windowId ? next : entry);
  return next;
}
