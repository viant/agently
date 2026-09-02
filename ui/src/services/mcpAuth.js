import { client } from './agentlyClient';

const PENDING_MCP_AUTH_KEY = 'agently.pendingMCPAuth';

function sessionStore() {
  return typeof window !== 'undefined' ? window.sessionStorage : null;
}

export function rememberPendingMCPAuth(server, returnURL, blocker = {}) {
  const store = sessionStore();
  if (!store) return;
  try {
    store.setItem(PENDING_MCP_AUTH_KEY, JSON.stringify({
      server: String(server || '').trim(),
      returnURL: String(returnURL || ''),
      conversationId: String(blocker?.conversationId || '').trim(),
      elicitationId: String(blocker?.elicitationId || '').trim(),
      startedAt: Date.now(),
    }));
  } catch (_) {}
}

export function currentPendingMCPAuth() {
  const store = sessionStore();
  if (!store) return null;
  try {
    const value = JSON.parse(store.getItem(PENDING_MCP_AUTH_KEY) || 'null');
    return value?.server ? pendingMCPAuth(value.server) : null;
  } catch (_) {
    return null;
  }
}

export function pendingMCPAuth(server) {
  const store = sessionStore();
  if (!store) return null;
  try {
    const value = JSON.parse(store.getItem(PENDING_MCP_AUTH_KEY) || 'null');
    if (!value || String(value.server || '').trim() !== String(server || '').trim()) return null;
    if (!Number.isFinite(value.startedAt) || Date.now() - value.startedAt > 10 * 60 * 1000) {
      store.removeItem(PENDING_MCP_AUTH_KEY);
      return null;
    }
    return value;
  } catch (_) {
    store.removeItem(PENDING_MCP_AUTH_KEY);
    return null;
  }
}

export function clearPendingMCPAuth(server) {
  const store = sessionStore();
  if (!store || !pendingMCPAuth(server)) return;
  try { store.removeItem(PENDING_MCP_AUTH_KEY); } catch (_) {}
}

export async function listEagerMCPConnections() {
  const result = await client.listMCPAuthConnections();
  return Array.isArray(result?.connections) ? result.connections : [];
}

export async function beginEagerMCPAuth(options = {}) {
  const pending = currentPendingMCPAuth();
  if (pending) return { status: 'pending', pending: true, server: pending.server };
  const connections = await listEagerMCPConnections();
  const target = connections.find((connection) => (
    connection?.connected !== true && String(connection?.server || '').trim()
  ));
  if (!target) return null;
  return beginBrowserMCPAuth(String(target.server).trim(), options);
}

export function currentMCPAuthReturnURL(locationValue = null) {
  const location = locationValue || (typeof window !== 'undefined' ? window.location : null);
  if (!location) return '/';
  const path = `${location.pathname || '/'}${location.search || ''}${location.hash || ''}`;
  return path.startsWith('/') && !path.startsWith('//') ? path : '/';
}

export async function beginBrowserMCPAuth(server, {
  returnURL = '', navigate = null, conversationId = '', elicitationId = '', forceRestart = false,
} = {}) {
  const normalizedServer = String(server || '').trim();
  if (!normalizedServer) throw new Error('MCP server is required');
  const status = await client.getMCPAuthStatus(normalizedServer);
  if (status?.connected === true) return { status: 'connected', connected: true };
  const resolvedReturnURL = returnURL || currentMCPAuthReturnURL();
  const initiateOptions = { returnURL: resolvedReturnURL, restart: status?.pending === true };
  if (forceRestart === true) initiateOptions.forceRestart = true;
  const result = await client.initiateMCPAuth(
    normalizedServer,
    String(status?.csrfToken || ''),
    initiateOptions,
  );
  if (result?.status === 'connect' && result?.authorizationURL) {
    const assign = navigate || (typeof window !== 'undefined' ? window.location.assign.bind(window.location) : null);
    if (!assign) throw new Error('Browser navigation is unavailable');
    rememberPendingMCPAuth(normalizedServer, resolvedReturnURL, { conversationId, elicitationId });
    assign(result.authorizationURL);
  }
  return result;
}

export async function resumePendingMCPAuth({ onConnected = null } = {}) {
  const pending = currentPendingMCPAuth();
  if (!pending) return null;
  const status = await client.getMCPAuthStatus(pending.server);
  if (status?.connected !== true) return null;
  if (typeof onConnected === 'function') onConnected(pending);
  if (pending.conversationId && pending.elicitationId) {
    await client.resolveElicitation(pending.conversationId, pending.elicitationId, {
      action: 'accept',
      payload: { connected: true },
    });
  }
  clearPendingMCPAuth(pending.server);
  return pending;
}
