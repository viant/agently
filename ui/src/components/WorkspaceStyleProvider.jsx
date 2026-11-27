import React, {createContext, useContext, useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore} from 'react';
import {ForgeThemeProvider} from 'forge/components';
import {applyApplicationThemeBoundary, clearApplicationThemeBoundary, getWorkspaceStyleManager} from '../services/workspaceStyles';

const WorkspaceStyleContext = createContext(null);
export const useWorkspaceStyle = () => useContext(WorkspaceStyleContext);
const useClientLayoutEffect = typeof document === 'undefined' ? useEffect : useLayoutEffect;

export default function WorkspaceStyleProvider({metadata, accountKey = '', workspaceKey = '', fetchAsset, readResource, onRefresh, nonce, children}) {
  const resolvedNonce = nonce ?? (typeof document === 'undefined' ? '' :
    (document.querySelector?.('meta[name="agently-style-nonce"]')?.content || document.querySelector?.('script[nonce]')?.nonce || ''));
  const managerRef = useRef(null);
  if (!managerRef.current) managerRef.current = getWorkspaceStyleManager({fetchAsset, nonce: resolvedNonce});
  const manager = managerRef.current;
  const state = useSyncExternalStore(manager.subscribe, manager.getSnapshot, manager.getSnapshot);
  const [settled, setSettled] = useState(null);
  const completed = !!metadata && settled?.metadata === metadata && settled?.accountKey === accountKey && settled?.workspaceKey === workspaceKey;
  const ready = completed && (!settled.themed || !!state.catalog);
  const appearance = {ready, error: completed ? settled.error : null};
  useEffect(() => { manager.retain(); return () => manager.release(); }, [manager]);
  useEffect(() => {
    if (fetchAsset) manager.fetchAsset = fetchAsset;
    manager.nonce = resolvedNonce;
    let active=true;
    if (metadata) void (async () => {
      let error = null;
      try {
        await manager.refresh(metadata, {accountKey, workspaceKey});
        if (!active) return;
        if (metadata.uiStyleSource === 'managed') {
          await manager.loadApplicationStyles([], {accountKey, styleSource:'managed', globalStyle:metadata.globalStyle, readResource});
        }
        const snapshot = manager.getSnapshot();
        if (!snapshot.catalog && snapshot.diagnostics.length) error = new Error(snapshot.diagnostics.join(' · '));
      } catch (failure) { error = failure; }
      if (active) setSettled({metadata, accountKey, workspaceKey, error, themed: !!manager.getSnapshot().catalog});
    })();
    return()=>{active=false;};
  }, [manager, metadata, accountKey, workspaceKey, fetchAsset, readResource, resolvedNonce]);
  useClientLayoutEffect(() => {
    const root = typeof document === 'undefined' ? null : document.documentElement;
    applyApplicationThemeBoundary(root, state);
    return () => clearApplicationThemeBoundary(root);
  }, [state.themeId, state.mode]);
  return <WorkspaceStyleContext.Provider value={{state, manager, refresh: onRefresh, ...appearance}}><ForgeThemeProvider themeId={state.themeId} mode={state.mode}>
    {typeof children === 'function' ? children(state, manager, appearance) : children}
  </ForgeThemeProvider></WorkspaceStyleContext.Provider>;
}
