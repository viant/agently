import {expect,it,vi} from 'vitest';
vi.mock('./agentlyClient',()=>({client:{applyPermission:vi.fn().mockResolvedValue({admitted:true})}}));
vi.mock('forge/core',()=>({ensureUIBridgeClientId:vi.fn(()=> 'current-client'),getActiveUIBridgeTransportContext:vi.fn(()=>null)}));
import {client} from './agentlyClient';
import {applyPermission} from './permissionService';
it('forwards only exact local framework routing with the admitted definition',async()=>{
 const resource={uri:'window://agently/chat-new'};
 await applyPermission({windowKey:'chat/new',completeMetadata:{resource},conversationId:'conv-1'});
 expect(client.applyPermission).toHaveBeenLastCalledWith('chat/new',expect.objectContaining({clientId:'current-client',windowId:'chat/new',resolvedResource:resource,conversationId:'conv-1'}));
 await applyPermission({windowKey:'other',completeMetadata:{resource}});
 const input=client.applyPermission.mock.lastCall[1];expect(input).not.toHaveProperty('clientId');expect(input).not.toHaveProperty('windowId');
});
