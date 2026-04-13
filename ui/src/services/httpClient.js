import { isConnectivityError } from './networkError';

const RETRYABLE_STATUSES = new Set([408, 425, 429, 500, 502, 503, 504]);
const DEFAULT_RETRIES = 2;
const TOAST_DEDUP_MS = 6000;

let authRedirectInFlight = false;
const recentToasts = new Map();

export class APIRequestError extends Error {
  constructor(message, { status = 0, payload = null, method = 'GET', path = '', transient = false } = {}) {
    super(message);
    this.name = 'APIRequestError';
    this.status = status;
    this.payload = payload;
    this.method = method;
    this.path = path;
    this.transient = !!transient;
  }
}

function apiPath(path) {
  return path.startsWith('/v1/') ? path : `/v1/${path.replace(/^\/+/, '')}`;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function shouldRetryMethod(method = 'GET') {
  const upper = String(method || 'GET').toUpperCase();
  return upper === 'GET' || upper === 'HEAD' || upper === 'OPTIONS';
}

function isRetryableStatus(status) {
  return RETRYABLE_STATUSES.has(Number(status || 0));
}

function parseResponseBody(response, isJSON) {
  if (response.status === 204) return null;
  return isJSON ? response.json() : response.text();
}

function toastColor(intent = 'danger') {
  if (intent === 'warning') return '#f59e0b';
  if (intent === 'success') return '#16a34a';
  if (intent === 'info') return '#2563eb';
  return '#dc2626';
}

export function showToast(message, { intent = 'danger', key = '', ttlMs = 4200 } = {}) {
  if (typeof window === 'undefined' || typeof document === 'undefined') return;
  const text = String(message || '').trim();
  if (!text) return;
  const dedupeKey = key || text;
  const now = Date.now();
  const seenAt = Number(recentToasts.get(dedupeKey) || 0);
  if (now - seenAt < TOAST_DEDUP_MS) return;
  recentToasts.set(dedupeKey, now);

  const id = `toast:${now}:${Math.random().toString(36).slice(2, 9)}`;
  const rootId = 'agently-toast-root';
  let root = document.getElementById(rootId);
  if (!root) {
    root = document.createElement('div');
    root.id = rootId;
    root.style.position = 'fixed';
    root.style.right = '18px';
    root.style.bottom = '22px';
    root.style.display = 'flex';
    root.style.flexDirection = 'column';
    root.style.gap = '8px';
    root.style.zIndex = '99999';
    document.body.appendChild(root);
  }
  const item = document.createElement('div');
  item.id = id;
  item.textContent = text;
  item.style.maxWidth = '420px';
  item.style.background = '#111827';
  item.style.color = '#f9fafb';
  item.style.border = `1px solid ${toastColor(intent)}`;
  item.style.borderLeft = `4px solid ${toastColor(intent)}`;
  item.style.borderRadius = '10px';
  item.style.padding = '10px 12px';
  item.style.fontSize = '13px';
  item.style.lineHeight = '1.35';
  item.style.boxShadow = '0 8px 24px rgba(0,0,0,0.24)';
  item.style.opacity = '0';
  item.style.transform = 'translateY(8px)';
  item.style.transition = 'opacity 120ms ease, transform 120ms ease';
  root.appendChild(item);
  requestAnimationFrame(() => {
    item.style.opacity = '1';
    item.style.transform = 'translateY(0)';
  });
  window.setTimeout(() => {
    item.style.opacity = '0';
    item.style.transform = 'translateY(8px)';
    window.setTimeout(() => item.remove(), 140);
  }, ttlMs);
}

export function redirectToLogin(path = '') {
  if (typeof window === 'undefined') return;
  try {
    window.dispatchEvent(new CustomEvent('agently:unauthorized', { detail: { path, status: 401 } }));
  } catch (_) {}
  if (authRedirectInFlight) return;
  authRedirectInFlight = true;

  // Full-page redirect to the IDP login via the SDK.
  // loginWithRedirect() saves the current URL in sessionStorage, then
  // navigates to the backend's /v1/api/auth/idp/login which 307-redirects
  // to the identity provider. After auth, the IDP redirects back to
  // /v1/api/auth/oauth/callback, handled by the OAuthCallback SPA route.
  import('./agentlyClient').then(({ beginLogin }) => {
    return beginLogin();
  }).catch(() => {
    authRedirectInFlight = false;
  });
}

export async function request(path, options = {}) {
  const method = String(options.method || 'GET').toUpperCase();
  const maxRetries = Number.isFinite(Number(options.retries))
    ? Math.max(0, Number(options.retries))
    : (shouldRetryMethod(method) ? DEFAULT_RETRIES : 0);
  const shouldNotify = options.notify === undefined ? method !== 'GET' : !!options.notify;
  const skipAuthRedirect = !!options.skipAuthRedirect;

  const finalPath = apiPath(path);
  for (let attempt = 0; attempt <= maxRetries; attempt++) {
    try {
      const response = await fetch(finalPath, {
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
          ...(options.headers || {})
        },
        ...options,
      });
      const isJSON = String(response.headers.get('content-type') || '').includes('application/json');
      const payload = await parseResponseBody(response, isJSON);
      if (response.status === 401) {
        if (!skipAuthRedirect) {
          redirectToLogin(path);
        }
        throw new APIRequestError(`${method} ${path} failed (401): unauthorized`, {
          status: 401,
          payload,
          method,
          path,
          transient: false
        });
      }
      if (response.ok) {
        return payload;
      }

      const retryable = isRetryableStatus(response.status) && attempt < maxRetries;
      if (retryable) {
        await sleep(250 * (attempt + 1) + Math.floor(Math.random() * 100));
        continue;
      }
      const detail = isJSON ? JSON.stringify(payload || {}) : String(payload || '');
      const err = new APIRequestError(`${method} ${path} failed (${response.status}): ${detail}`, {
        status: response.status,
        payload,
        method,
        path,
        transient: isRetryableStatus(response.status)
      });
      if (shouldNotify || err.transient) {
        showToast(
          err.transient
            ? `Temporary API failure (${response.status}) for ${path}.`
            : `Request failed (${response.status}) for ${path}.`,
          {
            intent: err.transient ? 'warning' : 'danger',
            key: `http:${method}:${path}:${response.status}`
          }
        );
      }
      throw err;
    } catch (err) {
      if (err instanceof APIRequestError) throw err;
      const retryable = shouldRetryMethod(method) && attempt < maxRetries;
      if (retryable) {
        await sleep(250 * (attempt + 1) + Math.floor(Math.random() * 120));
        continue;
      }
      const wrapped = new APIRequestError(`${method} ${path} failed: ${String(err?.message || err || 'network error')}`, {
        status: 0,
        payload: null,
        method,
        path,
        transient: isConnectivityError(err)
      });
      if (shouldNotify) {
        showToast(`Network error while calling ${path}.`, {
          intent: 'warning',
          key: `net:${method}:${path}`
        });
      }
      throw wrapped;
    }
  }
  throw new APIRequestError(`${method} ${path} failed: retries exhausted`, {
    status: 0,
    payload: null,
    method,
    path,
    transient: true
  });
}
