import {installWorkspaceThemeBridge} from '../services/workspaceThemeBridge';
import React, {useCallback, useEffect, useRef, useState} from 'react';
import WorkspaceStyleProvider from './WorkspaceStyleProvider';
import {fetchWorkspaceStyleAsset, getAuthMeSilently, refreshWorkspaceMetadata} from '../services/agentlyClient';
import {subscribeWorkspaceMetadata} from '../services/workspaceMetadata';
import {request} from '../services/httpClient';
import {getWorkspaceStyleManager,workspaceStyleAccountKey} from '../services/workspaceStyles';
import {sdkBaseURL} from '../endpoint';

export default function AppWorkspaceStyles({children}) {
  const [metadata, setMetadata] = useState(null);
  const [accountKey, setAccountKey] = useState('');
  const generation = useRef(0);
  const authorized = useRef(true);
  const refresh = useCallback(async () => {
    if (!authorized.current) return;
    const current = ++generation.current;
    try {
      const [next, me] = await Promise.all([refreshWorkspaceMetadata(), getAuthMeSilently().catch(() => null)]);
      if (current !== generation.current || !authorized.current) return;
      setAccountKey(workspaceStyleAccountKey(me,sdkBaseURL));
      setMetadata(next);
    } catch (error) {
      if (current !== generation.current) return;
      // Keep a same-workspace snapshot on transient failure. Auth events clear it.
      if ([401, 403].includes(Number(error?.status))) {
        getWorkspaceStyleManager().clear(); setMetadata(null); setAccountKey('');
      }
    }
  }, []);
  useEffect(() => {
    const clear = () => {
      authorized.current = false; generation.current++;
      getWorkspaceStyleManager().clear(); setMetadata(null); setAccountKey('');
    };
    const recover = () => { getWorkspaceStyleManager().clear(); authorized.current = true; refresh(); };
    const detachBridge = installWorkspaceThemeBridge(getWorkspaceStyleManager(), refresh);
    refresh();
    // A datasource metadata refresh is a trigger; always fetch fresh bytes using
    // the current auth generation instead of trusting a potentially stale event.
    const unsubscribe = subscribeWorkspaceMetadata(refresh);
    window.addEventListener('agently:unauthorized', clear);
    window.addEventListener('agently:logout', clear);
    window.addEventListener('agently:authorized', recover);
    return () => {
      generation.current++;
      unsubscribe();
      detachBridge();
      window.removeEventListener('agently:unauthorized', clear);
      window.removeEventListener('agently:logout', clear);
      window.removeEventListener('agently:authorized', recover);
    };
  }, [refresh]);
  const readResource=useCallback(({server,uri})=>request(`/api/mcp/resources/read?${new URLSearchParams({server,uri})}`,{retries:0}),[]);
  return <WorkspaceStyleProvider readResource={readResource} metadata={metadata} accountKey={accountKey} workspaceKey={sdkBaseURL}
    fetchAsset={fetchWorkspaceStyleAsset} onRefresh={refresh}>{children}</WorkspaceStyleProvider>;
}
