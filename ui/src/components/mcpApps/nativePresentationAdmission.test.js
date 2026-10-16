import {beforeEach, expect, it, vi} from 'vitest';
vi.mock('forge/core',()=>({getActiveUIBridgeTransportContext:vi.fn(),waitForUIBridgeReady:vi.fn()}));
import {getActiveUIBridgeTransportContext,waitForUIBridgeReady} from 'forge/core';
import {admitNativePresentation} from './nativePresentationAdmission.js';
const resource={uri:'window://workspace/advertiserList',providerIdentity:'internal',authorityBinding:'public-local-definition',contentFingerprint:'initial-selector'};
const selected={windowKey:'advertiserList',title:'Advertisers',resource,resourceTarget:{platform:'web',selectionToken:'initial-token'}};
beforeEach(()=>{vi.clearAllMocks();waitForUIBridgeReady.mockResolvedValue(true);getActiveUIBridgeTransportContext.mockReturnValue({clientId:'current-client',sessionId:'negotiated-session'});});
it('binds the allocated instance at the exact first selector without changing original params, target or title',async()=>{
 const parameters={AdvertiserId:[990001]};const target={platform:'web'};const load=vi.fn().mockResolvedValue(selected);
 const admitted=await admitNativePresentation(selected,'allocated-root',load,parameters,target,{namespace:'workspace',providerIdentity:'internal'});
 expect(load).toHaveBeenCalledWith('advertiserList',parameters,target,{namespace:'workspace',providerIdentity:'internal',clientId:'current-client',windowId:'allocated-root',resolvedResource:resource});
 expect(admitted.title).toBe('Advertisers');expect(selected.resourceTarget.selectionToken).toBe('initial-token');expect(load.mock.calls[0][3].resolvedResource).not.toBe(resource);
});
it('rejects before any second GET/data render when the existing transport is unavailable',async()=>{
 waitForUIBridgeReady.mockResolvedValue(false);const load=vi.fn();
 await expect(admitNativePresentation(selected,'allocated-root',load,{},{})).rejects.toThrow('UI transport');expect(load).not.toHaveBeenCalled();
});
it('keeps ordinary external MCP standalone windows on their existing one-GET path',async()=>{
 const external={...selected,resource:{...resource,providerIdentity:'external',uri:'window://vendor/advertiserList'}};const load=vi.fn();
 expect(await admitNativePresentation(external,'allocated-root',load,{},{})).toBe(external);expect(load).not.toHaveBeenCalled();expect(waitForUIBridgeReady).not.toHaveBeenCalled();
});
