// Convenience helpers (not strictly necessary since AuthProvider handles calls)
import { endpoints } from '../endpoint';

function joinURL(base, path) {
  const b = (base || '').replace(/\/+$/, '');
  const p = (path || '').replace(/^\/+/, '');
  return `${b}/${p}`;
}

const base = (endpoints?.appAPI?.baseURL || '').replace(/\/+$/, '');

export async function getMe() {
  const resp = await fetch(joinURL(base, '/v1/api/auth/me'), { credentials: 'include' });
  return resp.json().catch(() => ({}));
}

export async function getProviders() {
  const resp = await fetch(joinURL(base, '/v1/api/auth/providers'), { credentials: 'include' });
  return resp.json().catch(() => ({}));
}

export async function loginLocal(name) {
  const resp = await fetch(joinURL(base, '/v1/api/auth/local/login'), { method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name }) });
  return resp.json().catch(() => ({}));
}

export async function logout() {
  const resp = await fetch(joinURL(base, '/v1/api/auth/logout'), { method: 'POST', credentials: 'include' });
  return resp.json().catch(() => ({}));
}

