import { beforeEach, describe, expect, it, vi } from 'vitest';
vi.mock('forge/core', () => ({ensureUIBridgeClientId: () => 'legacy-client', getActiveUIBridgeTransportContext: vi.fn(() => null)}));
vi.mock('./agentlyClient', () => ({ client: { applyPermission: vi.fn() } }));
import { getActiveUIBridgeTransportContext } from 'forge/core';
import { client } from './agentlyClient';
import { applyPermission } from './permissionService';

describe('window permission renewal admission', () => {
  beforeEach(() => getActiveUIBridgeTransportContext.mockReturnValue(null));
  it('binds the completed OPEN to the requesting browser and real window instance', async () => {
    getActiveUIBridgeTransportContext.mockReturnValue({clientId:'requesting-client',sessionId:'negotiated-session'});
    await applyPermission({windowId:'record-321',windowKey:'window://provider/record',windowParams:{RecordId:[321]}});
    expect(client.applyPermission).toHaveBeenLastCalledWith('window://provider/record', expect.objectContaining({
      windowId:'record-321',clientId:'requesting-client',bridgeSessionId:'negotiated-session',windowParams:{RecordId:[321]},
    }));
  });
  it('submits the fresh server metadata pin and preserves the original selection', async () => {
    const oldPin = Object.freeze({ uri: 'window://tenant/detail', validUntil: '2000-01-01T00:00:00Z' });
    const freshPin = Object.freeze({ uri: oldPin.uri, validUntil: '2099-01-01T00:00:00Z' });
    const admitted = { resource: freshPin, authorizationSnapshot: { expiresAt: freshPin.validUntil } };
    client.applyPermission.mockResolvedValueOnce(admitted);
    const result = await applyPermission({
      windowKey: 'detail', completeMetadata: { resource: freshPin },
      resource: oldPin, windowParams: { SelectedId: [42] }, renewal: true,
    });
    expect(client.applyPermission).toHaveBeenLastCalledWith('detail', expect.objectContaining({
      resolvedResource: freshPin, windowParams: { SelectedId: [42] },
    }));
    expect(result).toBe(admitted);
    expect(oldPin.validUntil).toBe('2000-01-01T00:00:00Z');
  });

  it('propagates fresh admission denial without retry or lease fallback', async () => {
    const denial = Object.assign(new Error('resource revision denied'), { status: 403 });
    const before = client.applyPermission.mock.calls.length;
    client.applyPermission.mockRejectedValueOnce(denial);
    await expect(applyPermission({
      windowKey: 'detail', completeMetadata: { resource: { uri: 'window://tenant/detail' } },
      windowParams: { SelectedId: [42] }, renewal: true,
    })).rejects.toBe(denial);
    expect(client.applyPermission.mock.calls.length).toBe(before + 1);
  });
});
