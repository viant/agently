import { beginBrowserMCPAuth, clearPendingMCPAuth, resumePendingMCPAuth } from './mcpAuth';

export async function listConnectors() {
  const response = await fetch('/v1/api/auth/mcp/connectors', { credentials: 'same-origin', cache: 'no-store' });
  if (!response.ok) {
    throw new Error(response.status === 401 ? 'Sign in to manage your connections.' : 'Connections could not be loaded. Try again.');
  }
  const result = await response.json();
  if (!Array.isArray(result.connections)) throw new Error('The server returned an invalid connector list.');
  return result.connections;
}

export async function connectProvider(server) {
  return beginBrowserMCPAuth(server);
}

export async function finishConnectorReturn() {
  return resumePendingMCPAuth();
}

export async function disconnectProvider(server) {
  const name = String(server || '').trim();
  if (!name) throw new Error('MCP server is required');
  const path = `/v1/api/auth/mcp/${encodeURIComponent(name)}`;
  const statusResponse = await fetch(`${path}/status`, { credentials: 'same-origin', cache: 'no-store' });
  if (!statusResponse.ok) throw new Error('Could not disconnect. Refresh and try again.');
  const status = await statusResponse.json();
  if (!status.csrfToken) throw new Error('Could not disconnect. Refresh and try again.');
  const response = await fetch(path, {
    method: 'DELETE', credentials: 'same-origin',
    headers: { 'X-Agently-Csrf': status.csrfToken },
  });
  if (!response.ok) throw new Error('Could not disconnect. Refresh and try again.');
  // The idempotent backend response alone does not prove deletion succeeded.
  const connections = await listConnectors();
  const connection = connections.find(row => row.server === name);
  if (!connection || !connection.available) throw new Error('Could not verify disconnection. Refresh and try again.');
  if (connection.connected) {
    throw new Error('The connection is still active. Try again.');
  }
  clearPendingMCPAuth(name);
  return connections;
}
