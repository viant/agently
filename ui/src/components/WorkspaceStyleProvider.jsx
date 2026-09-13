import React, {createContext, useContext, useEffect, useRef, useSyncExternalStore} from 'react';
import {ForgeThemeProvider} from 'forge/components';
import {getWorkspaceStyleManager} from '../services/workspaceStyles';

const WorkspaceStyleContext = createContext(null);
export const useWorkspaceStyle = () => useContext(WorkspaceStyleContext);

export default function WorkspaceStyleProvider({metadata, accountKey = '', workspaceKey = '', fetchAsset, onRefresh, nonce, children}) {
  const resolvedNonce = nonce ?? (typeof document === 'undefined' ? '' :
    (document.querySelector?.('meta[name="agently-style-nonce"]')?.content || document.querySelector?.('script[nonce]')?.nonce || ''));
  const managerRef = useRef(null);
  if (!managerRef.current) managerRef.current = getWorkspaceStyleManager({fetchAsset, nonce: resolvedNonce});
  const manager = managerRef.current;
  const state = useSyncExternalStore(manager.subscribe, manager.getSnapshot, manager.getSnapshot);
  useEffect(() => { manager.retain(); return () => manager.release(); }, [manager]);
  useEffect(() => {
    if (fetchAsset) manager.fetchAsset = fetchAsset;
    manager.nonce = resolvedNonce;
    manager.refresh(metadata, {accountKey, workspaceKey});
  }, [manager, metadata, accountKey, workspaceKey, fetchAsset, resolvedNonce]);
  return <WorkspaceStyleContext.Provider value={{state, manager, refresh: onRefresh}}><ForgeThemeProvider themeId={state.themeId} mode={state.mode}>
    {typeof children === 'function' ? children(state, manager) : children}
  </ForgeThemeProvider></WorkspaceStyleContext.Provider>;
}
