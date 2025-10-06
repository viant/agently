import React, {createContext, useCallback, useEffect, useMemo, useRef, useState} from 'react';
import { endpoints } from './endpoint';

export const AuthContext = createContext({});

function joinURL(base, path) {
  const b = (base || '').replace(/\/+$/, '');
  const p = (path || '').replace(/^\/+/, '');
  return `${b}/${p}`;
}

export const AuthProvider = ({children}) => {
  const defaultAuthProvider = 'default';
  const base = (endpoints?.appAPI?.baseURL || '').replace(/\/+$/, '');
  const baseAgently = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');

  const [profile, setProfile] = useState(null);
  const [providers, setProviders] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [bearerToken, setBearerToken] = useState('');

  // Forge connector expects {authStates[default].jwtToken.id_token}
  const authStates = useMemo(() => ({
    [defaultAuthProvider]: {
      jwtToken: bearerToken ? { id_token: bearerToken } : undefined,
    },
  }), [bearerToken]);

  // Avoid duplicating /v1/api segment when base already includes it
  const apiURL = useCallback((path) => {
    const p = String(path || '').replace(/^\/+/, '');
    const hasApiInBase = /\/v1\/api\/?$/.test(base);
    const full = hasApiInBase && p.startsWith('v1/api/') ? p.replace(/^v1\/api\//, '') : p;
    return joinURL(base, full);
  }, [base]);

  const fetchJSON = async (url, opts={}) => {
    const resp = await fetch(url, {
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', ...(bearerToken ? { Authorization: `Bearer ${bearerToken}` } : {}) },
      ...opts,
    });
    const text = await resp.text();
    let data = null;
    try { data = text ? JSON.parse(text) : null; } catch(_) {}
    return { ok: resp.ok, status: resp.status, body: data };
  };

  const getMe = useCallback(async () => {
    const url = apiURL('/v1/api/auth/me');
    return await fetchJSON(url, { method: 'GET' });
  }, [apiURL, bearerToken]);

  const getProviders = useCallback(async () => {
    const url = apiURL('/v1/api/auth/providers');
    return await fetchJSON(url, { method: 'GET' });
  }, [apiURL]);

  // SPA login: accept a token from SDK and set bearer
  const loginSPAWithToken = useCallback(async (idToken) => {
    if (!idToken) return false;
    setBearerToken(idToken);
    try { window.AGENTLY_BEARER = idToken; } catch(_) {}
    const me = await getMe();
    if (me.ok) { setProfile(me.body?.data || {}); return true; }
    return false;
  }, [getMe]);

  // OIDC (SPA) — start Code+PKCE by opening provider authorize URL
  const loginSPA = useCallback(async () => {
    try {
      const prov = await getProviders();
      const oidc = (prov.ok && Array.isArray(prov.body?.data)) ? prov.body.data.find(p => p.type === 'oidc') : null;
      if (!oidc) return false;
      const disc = String(oidc.discoveryURL || '').trim();
      const clientID = String(oidc.clientID || '').trim();
      if (!disc || !clientID) return false;
      const redirect = (String(oidc.redirectURI || '').trim()) || (window.location.origin + '/');

      // Build PKCE params
      const rand = (n) => btoa(String.fromCharCode(...crypto.getRandomValues(new Uint8Array(n)))).replace(/[^a-zA-Z0-9]/g, '').slice(0, n);
      const base64url = (buf) => btoa(String.fromCharCode(...new Uint8Array(buf))).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
      const codeVerifier = rand(96);
      const enc = new TextEncoder().encode(codeVerifier);
      const digest = await crypto.subtle.digest('SHA-256', enc);
      const codeChallenge = base64url(digest);
      const state = rand(32);

      // Fetch discovery to get authorization_endpoint
      const dResp = await fetch(disc);
      const d = await dResp.json();
      const authURL = d.authorization_endpoint;
      if (!authURL) return false;

      // Persist PKCE data in sessionStorage (tab-scoped) and localStorage (popup fallback)
      sessionStorage.setItem('oidc_state', state);
      sessionStorage.setItem('oidc_verifier', codeVerifier);
      sessionStorage.setItem('oidc_redirect', redirect);
      sessionStorage.setItem('oidc_discovery', disc);
      sessionStorage.setItem('oidc_client_id', clientID);
      try {
        localStorage.setItem('agently_oidc_state', state);
        localStorage.setItem('agently_oidc_verifier', codeVerifier);
        localStorage.setItem('agently_oidc_redirect', redirect);
        localStorage.setItem('agently_oidc_discovery', disc);
        localStorage.setItem('agently_oidc_client_id', clientID);
      } catch(_) {}

      const scopes = (oidc.scopes && oidc.scopes.length) ? oidc.scopes.join(' ') : 'openid email profile';
      const params = new URLSearchParams({
        response_type: 'code',
        client_id: clientID,
        redirect_uri: redirect,
        scope: scopes,
        state,
        code_challenge: codeChallenge,
        code_challenge_method: 'S256',
      });
      const url = `${authURL}?${params.toString()}`;
      // Use popup to avoid navigating this window
      const popup = window.open(url, 'oidc_spa', 'width=520,height=640');
      if (!popup) return false;
      return new Promise((resolve) => {
        const onMsg = async (ev) => {
          try {
            if (!ev || !ev.data) return;
            if (ev.data.type === 'oidc' && ev.data.status === 'ok' && ev.data.id_token) {
              window.removeEventListener('message', onMsg);
              await loginSPAWithToken(ev.data.id_token);
              resolve(true);
            }
          } catch(_) {}
        };
        window.addEventListener('message', onMsg);
        const timer = setInterval(() => {
          if (popup && popup.closed) { clearInterval(timer); window.removeEventListener('message', onMsg); resolve(false); }
        }, 500);
      });
    } catch (_) {
      return false;
    }
  }, [getProviders]);

  // Handle redirect: exchange code for tokens and set bearer
  const handleOIDCRedirect = useCallback(async () => {
    try {
      const qs = new URLSearchParams(window.location.search || '');
      const code = (qs.get('code') || '').trim();
      const state = (qs.get('state') || '').trim();
      if (!code || !state) return false;
      let savedState = sessionStorage.getItem('oidc_state') || '';
      let verifier = sessionStorage.getItem('oidc_verifier') || '';
      let disc = sessionStorage.getItem('oidc_discovery') || '';
      let clientID = sessionStorage.getItem('oidc_client_id') || '';
      let redirect = sessionStorage.getItem('oidc_redirect') || (window.location.origin + '/');
      // Fallback to localStorage if opened in popup (different sessionStorage)
      if (!savedState || !verifier || !disc || !clientID) {
        try {
          savedState = savedState || localStorage.getItem('agently_oidc_state') || '';
          verifier = verifier || localStorage.getItem('agently_oidc_verifier') || '';
          disc = disc || localStorage.getItem('agently_oidc_discovery') || '';
          clientID = clientID || localStorage.getItem('agently_oidc_client_id') || '';
          redirect = redirect || localStorage.getItem('agently_oidc_redirect') || (window.location.origin + '/');
        } catch(_) {}
      }
      if (!verifier || !disc || !clientID || state !== savedState) return false;

      // Load token endpoint
      const dResp = await fetch(disc);
      const d = await dResp.json();
      const tokenURL = d.token_endpoint;
      if (!tokenURL) return false;
      const body = new URLSearchParams({
        grant_type: 'authorization_code',
        code,
        redirect_uri: redirect,
        client_id: clientID,
        code_verifier: verifier,
      });
      const resp = await fetch(tokenURL, { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body });
      const tok = await resp.json().catch(() => ({}));
      const idt = tok.id_token || '';
      if (!idt) return false;
      await loginSPAWithToken(idt);
      // Clean URL
      try { window.history.replaceState({}, document.title, window.location.pathname); } catch(_) {}
      // Notify popup opener if present and close
      try { if (window.opener) window.opener.postMessage({type:'oidc', status:'ok', id_token: idt}, window.location.origin); } catch(_) {}
      try { window.close(); } catch(_) {}
      // Clear stored PKCE hints
      try {
        sessionStorage.removeItem('oidc_state');
        sessionStorage.removeItem('oidc_verifier');
        sessionStorage.removeItem('oidc_discovery');
        sessionStorage.removeItem('oidc_client_id');
        sessionStorage.removeItem('oidc_redirect');
        localStorage.removeItem('agently_oidc_state');
        localStorage.removeItem('agently_oidc_verifier');
        localStorage.removeItem('agently_oidc_discovery');
        localStorage.removeItem('agently_oidc_client_id');
        localStorage.removeItem('agently_oidc_redirect');
      } catch(_) {}
      return true;
    } catch (_) {
      return false;
    }
  }, [loginSPAWithToken]);

  const loginLocal = useCallback(async (name) => {
    const url = apiURL('/v1/api/auth/local/login');
    const { ok } = await fetchJSON(url, { method: 'POST', body: JSON.stringify({ name }) });
    return ok;
  }, [apiURL]);

  const logout = useCallback(async () => {
    const url = apiURL('/v1/api/auth/logout');
    await fetchJSON(url, { method: 'POST' });
    setBearerToken('');
    setProfile(null);
    try { window.AGENTLY_BEARER = ''; } catch(_) {}
  }, [apiURL]);

  // BFF OAuth popup init
  const loginBFF = useCallback(async () => {
    // Open a popup immediately on user gesture to avoid blockers
    const popup = window.open('about:blank', 'oauth_bff', 'width=520,height=640');
    try {
      const url = apiURL('/v1/api/auth/oauth/initiate');
      const { ok, body } = await fetchJSON(url, { method: 'POST' });
      const authURL = body?.data?.authURL || '';
      if (!ok || !authURL || !/^https?:/i.test(authURL)) {
        try { popup && popup.close(); } catch(_) {}
        return false;
      }
      try { if (popup) popup.location.href = authURL; } catch(_) {}
      return new Promise((resolve) => {
        const handler = async (ev) => {
          try {
            if (!ev || !ev.data) return;
            if (ev.data.type === 'oauth' && ev.data.status === 'ok') {
              window.removeEventListener('message', handler);
              const me = await getMe();
              if (me.ok) { setProfile(me.body?.data || {}); resolve(true); return; }
              resolve(false);
            }
          } catch(_) {}
        };
        window.addEventListener('message', handler);
        const timer = setInterval(() => {
          if (popup && popup.closed) { clearInterval(timer); window.removeEventListener('message', handler); resolve(false); }
        }, 500);
      });
    } catch (e) {
      try { popup && popup.close(); } catch(_) {}
      return false;
    }
  }, [apiURL, fetchJSON, getMe, setProfile]);

  // (moved above) loginSPAWithToken

  // Bootstrap: try me; if 401 attempt silent local login when defaultUsername present
  useEffect(() => {
    (async () => {
      setLoading(true); setError(null);
      const me = await getMe();
      if (me.ok) { setProfile(me.body?.data || {}); setLoading(false); return; }
      // If we returned from OIDC redirect, attempt exchange
      const handled = await handleOIDCRedirect();
      if (handled) { const me2 = await getMe(); if (me2.ok) { setProfile(me2.body?.data || {}); setLoading(false); return; } }
      const prov = await getProviders();
      if (prov.ok) {
        const items = prov.body?.data || [];
        setProviders(items);
        const local = items.find(p => p.type === 'local' && p.defaultUsername);
        if (local && local.defaultUsername) {
          const ok = await loginLocal(local.defaultUsername);
          if (ok) {
            const me2 = await getMe();
            if (me2.ok) { setProfile(me2.body?.data || {}); setLoading(false); return; }
          }
        }
      }
      setLoading(false);
    })().catch(e => { setError(e?.message || String(e)); setLoading(false); });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Keep a global hint for non-auth-aware datasources (Forge) to attach Authorization
  useEffect(() => {
    try { window.AGENTLY_BEARER = bearerToken || ''; } catch(_) {}
  }, [bearerToken]);

  const value = useMemo(() => ({
    ready: !loading,
    error,
    profile,
    providers,
    authStates,
    defaultAuthProvider,
    // actions
    loginLocal,
    loginBFF,
    loginSPA,
    loginSPAWithToken,
    logout,
  }), [loading, error, profile, providers, authStates, defaultAuthProvider, loginLocal, loginBFF, loginSPAWithToken, logout]);

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  );
};
