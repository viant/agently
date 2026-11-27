import {installWorkspaceThemeBridge} from '../services/workspaceThemeBridge';
import React, {useCallback, useEffect, useRef, useState} from 'react';
import {flushSync} from 'react-dom';
import WorkspaceStyleProvider from './WorkspaceStyleProvider';
import {fetchWorkspaceStyleAsset, getAuthMeSilently, refreshWorkspaceMetadata} from '../services/agentlyClient';
import {subscribeWorkspaceMetadata} from '../services/workspaceMetadata';
import {request} from '../services/httpClient';
import {getWorkspaceStyleManager,workspaceStyleAccountKey} from '../services/workspaceStyles';
import {sdkBaseURL} from '../endpoint';
import {ApplicationRecovery} from './ApplicationRecovery.jsx';

export default function AppWorkspaceStyles({children}) {
  const [metadata, setMetadata] = useState(null);
  const [accountKey, setAccountKey] = useState('');
  const [phase, setPhase] = useState('loading');
  const [error, setError] = useState(null);
  const generation = useRef(0);
  const authorized = useRef(true);
  const refresh = useCallback(async () => {
    if (!authorized.current) return;
    const current = ++generation.current;
    try {
      const [next, me] = await Promise.all([refreshWorkspaceMetadata(), getAuthMeSilently()]);
      if (current !== generation.current || !authorized.current) return;
      setAccountKey(workspaceStyleAccountKey(me,sdkBaseURL));
      setMetadata({...next});
      setError(null);
      setPhase('loaded');
    } catch (error) {
      if (current !== generation.current) return;
      // Keep a same-workspace snapshot on transient failure. Auth events clear it.
      if ([401, 403].includes(Number(error?.status))) {
        getWorkspaceStyleManager().clear(); setMetadata(null); setAccountKey('');
        setPhase('auth-required');
      } else {
        setError(error);
      }
    }
  }, []);
  useEffect(() => {
    const clear = () => {
      authorized.current = false; generation.current++;
      getWorkspaceStyleManager().clear(); setMetadata(null); setAccountKey('');
      setError(null); setPhase('auth-required');
    };
    const recover = () => {
      // The external style store can publish synchronously. Hide the routes
      // before clearing it, rather than waiting for a batched React render.
      flushSync(() => {setPhase('loading'); setMetadata(null); setError(null);});
      getWorkspaceStyleManager().clear(); authorized.current = true; refresh();
    };
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
    fetchAsset={fetchWorkspaceStyleAsset} onRefresh={refresh}>{(_state, _manager, appearance) => {
      // Authentication callbacks must finish even when workspace reads require login.
      const callback = typeof window !== 'undefined' && window.location.pathname === '/v1/api/auth/oauth/callback';
      const failure = error || appearance.error;
      const pending = !callback && phase !== 'auth-required' && (phase !== 'loaded' || !appearance.ready || !!failure);
      return <>
        {pending && (failure ? <ApplicationRecovery error={failure}/> :
          <div className="app-application-recovery" role="status" aria-busy="true">Loading workspace appearance…</div>)}
        {/* Keep routes mounted so native auth discovery and callbacks can run. */}
        <div hidden={pending} style={{display: pending ? 'none' : 'contents'}}>{children}</div>
      </>;
    }}</WorkspaceStyleProvider>;
}
