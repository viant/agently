/**
 * Singleton AgentlyClient instance for the v1 UI.
 *
 * Base URL is derived from the single source of truth in endpoint.js,
 * which reads DATA_URL from .env (injected by Vite at build time).
 * All services and components should import `client` from this module
 * instead of using raw fetch / httpClient.
 */
import { AgentlyClient } from 'agently-core-ui-sdk';
import { sdkBaseURL } from '../endpoint';
import { showToast, redirectToLogin } from './httpClient';

let authRecoveryInFlight = null;
let oauthConfigInFlight = null;
let authMeInFlight = null;
let authMeCacheReady = false;
let authMeCacheValue = null;
const workspaceMetadataInFlight = new Map();
const workspaceMetadataCache = new Map();
const AUTH_REQUEST_TIMEOUT_MS = 4500;

function workspaceMetadataCacheKey(targetContext = null) {
  const platform = String(targetContext?.platform || '').trim();
  const formFactor = String(targetContext?.formFactor || '').trim();
  const surface = String(targetContext?.surface || '').trim();
  const capabilities = Array.isArray(targetContext?.capabilities)
    ? targetContext.capabilities.map((entry) => String(entry || '').trim()).filter(Boolean).sort()
    : [];
  return JSON.stringify({ platform, formFactor, surface, capabilities });
}

function dispatchAuthRecovered() {
  if (typeof window === 'undefined') return;
  try {
    window.dispatchEvent(new CustomEvent('agently:authorized'));
  } catch (_) {}
}

function clearAuthMeCache() {
  authMeInFlight = null;
  authMeCacheReady = false;
  authMeCacheValue = null;
}

async function fetchAuthEndpoint(path = '', { timeoutMs = AUTH_REQUEST_TIMEOUT_MS } = {}) {
  const controller = typeof AbortController !== 'undefined' && timeoutMs > 0
    ? new AbortController()
    : null;
  const timer = controller && typeof globalThis.setTimeout === 'function'
    ? globalThis.setTimeout(() => controller.abort(), timeoutMs)
    : null;
  try {
    return await fetch(`${sdkBaseURL}${path}`, {
      method: 'GET',
      credentials: 'include',
      headers: {
        Accept: 'application/json'
      },
      signal: controller?.signal,
    });
  } finally {
    if (timer) {
      clearTimeout(timer);
    }
  }
}

export async function getAuthProvidersSilently() {
  try {
    const response = await fetchAuthEndpoint('/api/auth/providers');
    if (!response.ok) return [];
    const payload = await response.json().catch(() => null);
    if (Array.isArray(payload?.providers)) return payload.providers;
    if (Array.isArray(payload)) return payload;
  } catch (_) {}
  return [];
}

async function probeAuthMe() {
  const response = await fetchAuthEndpoint('/api/auth/me');
  if (response.status === 200) return true;
  if (response.status === 401 || response.status === 403) return false;
  return false;
}

export async function getAuthMeSilently() {
  if (authMeCacheReady) {
    return authMeCacheValue;
  }
  if (authMeInFlight) {
    return authMeInFlight;
  }
  authMeInFlight = (async () => {
    const response = await fetchAuthEndpoint('/api/auth/me');
    if (response.status === 200) {
      try {
        authMeCacheValue = await response.json();
      } catch (_) {
        authMeCacheValue = null;
      }
      authMeCacheReady = true;
      return authMeCacheValue;
    }
    if (response.status === 401 || response.status === 403) {
      authMeCacheValue = null;
      authMeCacheReady = true;
      return null;
    }
    authMeCacheValue = null;
    authMeCacheReady = true;
    return null;
  })().finally(() => {
    authMeInFlight = null;
  });
  return authMeInFlight;
}

export async function recoverSessionSilently() {
  if (authRecoveryInFlight) return authRecoveryInFlight;
  authRecoveryInFlight = (async () => {
    try {
      if (await probeAuthMe()) {
        clearAuthMeCache();
        dispatchAuthRecovered();
        return true;
      }
      await new Promise((resolve) => setTimeout(resolve, 250));
      if (await probeAuthMe()) {
        clearAuthMeCache();
        dispatchAuthRecovered();
        return true;
      }
      return false;
    } catch (_) {
      return false;
    } finally {
      authRecoveryInFlight = null;
    }
  })();
  return authRecoveryInFlight;
}

async function getOAuthConfigCached() {
  if (oauthConfigInFlight) return oauthConfigInFlight;
  oauthConfigInFlight = client.getOAuthConfig()
    .catch(() => null)
    .finally(() => {
      oauthConfigInFlight = null;
    });
  return oauthConfigInFlight;
}

export async function beginLogin() {
  if (typeof window === 'undefined') return false;
  clearAuthMeCache();
  const oauthConfig = await getOAuthConfigCached();
  if (oauthConfig?.usePopupLogin === true) {
    return client.loginWithPopup({
      onSuccess: () => dispatchAuthRecovered(),
      onPopupBlocked: () => client.loginWithRedirect()
    });
  }
  client.loginWithRedirect();
  return true;
}

export const client = new AgentlyClient({
  baseURL: sdkBaseURL,
  useCookies: true,
  retries: 2,
  retryDelayMs: 250,
  retryStatuses: [408, 425, 429, 500, 502, 503, 504],
  timeoutMs: 0, // No timeout — agent queries can take minutes (tool elicitations, long chains)
  onUnauthorized: async () => {
    clearAuthMeCache();
    const recovered = await recoverSessionSilently();
    if (!recovered) {
      redirectToLogin();
    }
  },
  onError: (err) => {
    const status = err?.status || 0;
    const transient = [408, 425, 429, 500, 502, 503, 504].includes(status);
    showToast(
      status > 0
        ? (transient
            ? `Temporary API failure (${status}).`
            : `Request failed (${status}).`)
        : 'Network error — check your connection.',
      { intent: transient ? 'warning' : 'danger' }
    );
  },
});

const rawGetWorkspaceMetadata = client.getWorkspaceMetadata.bind(client);
client.getWorkspaceMetadata = async function getWorkspaceMetadataCached(targetContext) {
  const key = workspaceMetadataCacheKey(targetContext);
  if (workspaceMetadataCache.has(key)) {
    return workspaceMetadataCache.get(key);
  }
  if (workspaceMetadataInFlight.has(key)) {
    return workspaceMetadataInFlight.get(key);
  }
  const request = rawGetWorkspaceMetadata(targetContext)
    .then((payload) => {
      workspaceMetadataCache.set(key, payload);
      return payload;
    })
    .finally(() => {
      workspaceMetadataInFlight.delete(key);
    });
  workspaceMetadataInFlight.set(key, request);
  return request;
};
