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

export const client = new AgentlyClient({
  baseURL: sdkBaseURL,
  useCookies: true,
  retries: 2,
  retryDelayMs: 250,
  retryStatuses: [408, 425, 429, 500, 502, 503, 504],
  timeoutMs: 0, // No timeout — agent queries can take minutes (tool elicitations, long chains)
  onUnauthorized: () => {
    redirectToLogin();
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
