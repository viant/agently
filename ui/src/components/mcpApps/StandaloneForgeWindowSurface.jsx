import React from 'react';
import {WindowManager} from 'forge/components';
import {windowMetadataFetchKey} from 'forge/components/WindowContent.jsx';
import {activeWindows, addWindow, removeWindow, selectedTabId, selectedWindowId} from 'forge/core';
import {mountStandaloneRoot, disposeStandaloneScope, closeStandaloneBranch, createStandaloneScopeId, applyStandaloneAdmission} from './standaloneWindowScope.js';

export function windowCatalogRouting(window) {
  const match = /^window:\/\/([^/]+)\/.+$/.exec(String(window?.resource?.uri || ''));
  return match ? {namespace: match[1], ...(window?.resource?.providerIdentity ? {providerIdentity: window.resource.providerIdentity} : {})} : {};
}

export default function StandaloneForgeWindowSurface({selectedWindow, loadWindow, targetContext}) {
  const reactId = React.useId();
  const baseScopeId = `standalone:${reactId}`;
  const selectionKey = windowMetadataFetchKey(selectedWindow || {}, targetContext);
  const [mountedScope, setMountedScope] = React.useState(null);
  React.useEffect(() => {
    const store = {activeWindows, addWindow, removeWindow, selectedTabId, selectedWindowId};
    const scopeId = createStandaloneScopeId(baseScopeId);
    const focus = {selectedTabId: selectedTabId.peek(), selectedWindowId: selectedWindowId.peek()};
    const root = mountStandaloneRoot(store, scopeId, selectedWindow);
    const pending = new Map();
    let active = true;
    // Existing window actions create scoped child entries. Admit each original
    // key/parameter selection once before its real WindowContent can mount.
    const admitChildren = () => {
      const windows = activeWindows.peek();
      for (const id of pending.keys()) { if (!windows.some(entry => entry.windowId === id && entry.windowScopeId === scopeId)) pending.delete(id); }
      for (const entry of activeWindows.peek()) {
        if (entry.windowScopeId !== scopeId || entry.windowId === root.windowId || pending.has(entry.windowId) || entry.standaloneAdmissionComplete) continue;
        const parent = windows.find(window => window.windowId === entry.parentKey && window.windowScopeId === scopeId) || selectedWindow;
        const request = {parameters: structuredClone(entry.parameters || {}), routing: windowCatalogRouting(parent)};
        pending.set(entry.windowId, request);
        Promise.resolve().then(() => loadWindow(entry.windowKey, request.parameters, targetContext, request.routing))
          .then(selected => {
            if (!active || pending.get(entry.windowId) !== request) return;
            if (!selected?.inlineMetadata?.view) throw new Error('Selected window frame is incomplete');
            applyStandaloneAdmission(store, scopeId, entry.windowId, selected);
          })
          .catch(error => {
            if (!active || pending.get(entry.windowId) !== request) return;
            const current = activeWindows.peek().find(window => window.windowId === entry.windowId && window.windowScopeId === scopeId);
            if (current) activeWindows.value = activeWindows.peek().map(window => window === current ? {...current, standaloneOpenError: error?.message || 'Failed to load workspace window'} : window);
          });
      }
    };
    const stop = activeWindows.subscribe(admitChildren);
    setMountedScope({rootId: root.windowId, scopeId});
    return () => {
      active = false;
      stop();
      disposeStandaloneScope(store, scopeId, focus);
    };
  }, [baseScopeId, selectionKey, loadWindow]);
  const windowFilter = React.useCallback(entry => entry?.windowScopeId === mountedScope?.scopeId, [mountedScope?.scopeId]);
  const onCloseWindow = React.useCallback(id => closeStandaloneBranch({activeWindows, removeWindow}, mountedScope?.scopeId, id), [mountedScope?.scopeId]);
  const renderWindowContent = React.useCallback(({window: entry, defaultContent}) => {
    if (entry.standaloneOpenError) return <div role="alert" style={{padding: 20}}>{entry.standaloneOpenError}</div>;
    if (!entry.standaloneAdmissionComplete || !entry.inlineMetadata?.view) return <div role="status" style={{padding: 20}}>Loading workspace window...</div>;
    // The frame came from the completed selected GET. This delivery input
    // avoids a duplicate metadata request; data execution stays server managed.
    return React.cloneElement(defaultContent, {selectedMetadata: entry.inlineMetadata});
  }, []);
  return mountedScope?.rootId ? <WindowManager includeHosted windowFilter={windowFilter} onCloseWindow={onCloseWindow} renderWindowContent={renderWindowContent} /> : null;
}
