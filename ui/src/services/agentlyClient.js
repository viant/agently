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

function dispatchAuthRecovered() {
  if (typeof window === 'undefined') return;
  try {
    window.dispatchEvent(new CustomEvent('agently:authorized'));
  } catch (_) {}
}

async function probeAuthMe() {
  const response = await fetch(`${sdkBaseURL}/api/auth/me`, {
    method: 'GET',
    credentials: 'include',
    headers: {
      Accept: 'application/json'
    }
  });
  if (response.status === 200) return true;
  if (response.status === 401 || response.status === 403) return false;
  return false;
}

export async function getAuthMeSilently() {
  const response = await fetch(`${sdkBaseURL}/api/auth/me`, {
    method: 'GET',
    credentials: 'include',
    headers: {
      Accept: 'application/json'
    }
  });
  if (response.status === 200) {
    try {
      return await response.json();
    } catch (_) {
      return null;
    }
  }
  if (response.status === 401 || response.status === 403) return null;
  return null;
}

export async function recoverSessionSilently() {
  if (authRecoveryInFlight) return authRecoveryInFlight;
  authRecoveryInFlight = (async () => {
    try {
      if (await probeAuthMe()) {
        dispatchAuthRecovered();
        return true;
      }
      await new Promise((resolve) => setTimeout(resolve, 250));
      if (await probeAuthMe()) {
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
