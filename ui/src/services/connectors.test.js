import { afterEach, expect, it, vi } from 'vitest';
vi.mock('./mcpAuth', () => ({ beginBrowserMCPAuth: vi.fn(), resumePendingMCPAuth: vi.fn(), clearPendingMCPAuth: vi.fn() }));
import { listConnectors, connectProvider, disconnectProvider } from './connectors';
import { beginBrowserMCPAuth, clearPendingMCPAuth } from './mcpAuth';
afterEach(() => { vi.unstubAllGlobals(); vi.clearAllMocks(); });
it('loads optional connectors with the signed-in session', async () => {
  const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ connections: [{ provider: 'asana', server: 'asana', connected: false, available: true }] }) });
  vi.stubGlobal('fetch', fetch);
  expect(await listConnectors()).toHaveLength(1);
  expect(fetch).toHaveBeenCalledWith('/v1/api/auth/mcp/connectors', expect.objectContaining({ credentials: 'same-origin', cache: 'no-store' }));
});
it('uses the same OAuth initiation path as tools', async () => {
  beginBrowserMCPAuth.mockResolvedValue({ status: 'connected' });
  expect(await connectProvider('asana')).toEqual({ status: 'connected' });
  expect(beginBrowserMCPAuth).toHaveBeenCalledWith('asana');
});
it('does not expose raw server failures', async () => {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 500, json: async () => ({ error: 'secret-file-path' }) }));
  await expect(listConnectors()).rejects.toThrow('Connections could not be loaded. Try again.');
});

it('disconnects with session CSRF and verifies the saved connection is gone', async () => {
  const fetch = vi.fn()
    .mockResolvedValueOnce({ ok: true, json: async () => ({ csrfToken: 'test-csrf' }) })
    .mockResolvedValueOnce({ ok: true })
    .mockResolvedValueOnce({ ok: true, json: async () => ({ connections: [{ server: 'asana', connected: false, available: true }] }) });
  vi.stubGlobal('fetch', fetch);
  expect(await disconnectProvider('asana')).toEqual([{ server: 'asana', connected: false, available: true }]);
  expect(fetch).toHaveBeenNthCalledWith(2, '/v1/api/auth/mcp/asana', {
    method: 'DELETE', credentials: 'same-origin', headers: { 'X-Agently-Csrf': 'test-csrf' },
  });
  expect(clearPendingMCPAuth).toHaveBeenCalledWith('asana');
});
it('does not disconnect without CSRF', async () => {
  const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) });
  vi.stubGlobal('fetch', fetch);
  await expect(disconnectProvider('asana')).rejects.toThrow('Could not disconnect');
  expect(fetch).toHaveBeenCalledTimes(1);
});
it('does not claim success if the backend retained the connection', async () => {
  vi.stubGlobal('fetch', vi.fn()
    .mockResolvedValueOnce({ ok: true, json: async () => ({ csrfToken: 'test-csrf' }) })
    .mockResolvedValueOnce({ ok: true })
    .mockResolvedValueOnce({ ok: true, json: async () => ({ connections: [{ server: 'asana', connected: true, available: true }] }) }));
  await expect(disconnectProvider('asana')).rejects.toThrow('connection is still active');
  expect(clearPendingMCPAuth).not.toHaveBeenCalled();
});
