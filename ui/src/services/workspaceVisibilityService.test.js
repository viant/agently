// @vitest-environment jsdom
import {afterEach,expect,test,vi} from 'vitest';
import {resolveConversationSelection} from './conversationWindow';
vi.mock('./conversationWindow',()=>({MAIN_CHAT_WINDOW_ID:'chat',resolveConversationSelection:vi.fn()}));
import {getReportRestoreScope} from './agentlyClient';
import {request} from './httpClient';
import {setWorkspaceVisibility} from './workspaceVisibilityService';
import {browserWorkspaceVisibility,workspaceVisibilityScope,workspaceApplicationKey} from './personalWorkspace';
vi.mock('./agentlyClient',()=>({getReportRestoreScope:vi.fn()}));
vi.mock('./httpClient',()=>({request:vi.fn()}));
afterEach(()=>{localStorage.clear();vi.resetAllMocks();});
const app={id:'example',providerIdentity:'publisher',title:'Example',menus:[]};
function setup(){resolveConversationSelection.mockReturnValue('chat-a');getReportRestoreScope.mockReturnValue('session-a');request.mockImplementation(async path=>path==='/v1/api/auth/me'?{subject:'alice'}:{workspaceId:'workspace',layout:{applications:[app]}});}
test('uses the same browser writer without a server mutation',async()=>{
 setup(); const result=await setWorkspaceVisibility({conversationId:'chat-a',expectedSubject:'alice',action:'set_application_visibility',applicationId:workspaceApplicationKey(app),hidden:true});
 expect(result).toMatchObject({status:'saved',storage:'browser'});
 expect(browserWorkspaceVisibility(workspaceVisibilityScope('workspace',{subject:'alice'})).hiddenApplications).toEqual([workspaceApplicationKey(app)]);
 expect(request.mock.calls.map(([path])=>path)).toEqual(['/v1/workspace/layout','/v1/api/auth/me']);
});
test('rejects stale sessions before saving after metadata arrives',async()=>{
 setup();let deliver;request.mockImplementation(()=>new Promise(resolve=>{deliver=resolve;}));
 const operation=setWorkspaceVisibility({conversationId:'chat-a',expectedSubject:'alice',action:'set_application_visibility',applicationId:workspaceApplicationKey(app),hidden:true});
 await Promise.resolve();getReportRestoreScope.mockReturnValue('session-b');deliver({workspaceId:'workspace',layout:{applications:[app]}});
 await expect(operation).rejects.toThrow('session changed');expect(localStorage.length).toBe(0);
});
test('rejects undiscovered targets without changing browser preferences',async()=>{
 setup(); await expect(setWorkspaceVisibility({conversationId:'chat-a',expectedSubject:'alice',action:'set_application_visibility',applicationId:'missing',hidden:true})).rejects.toThrow('not available');expect(localStorage.length).toBe(0);
});

test('leaving the chat while the catalog loads prevents a visibility write',async()=>{
 setup();
 let deliver;
 request.mockImplementation(()=>new Promise(resolve=>{deliver=resolve;}));
 const pending=setWorkspaceVisibility({conversationId:'chat-a',expectedSubject:'alice',action:'set_application_visibility',applicationId:workspaceApplicationKey(app),hidden:true});
 await Promise.resolve();
 resolveConversationSelection.mockReturnValue('chat-b');
 deliver({workspaceId:'workspace',layout:{applications:[app]}});
 await expect(pending).rejects.toThrow();
 expect(localStorage.length).toBe(0);
});
test('missing workspace identity cannot acknowledge or save visibility',async()=>{
 setup();
 request.mockResolvedValueOnce({layout:{applications:[app]}});
 await expect(setWorkspaceVisibility({conversationId:'chat-a',expectedSubject:'alice',action:'set_application_visibility',applicationId:workspaceApplicationKey(app),hidden:true})).rejects.toThrow('identify your workspace');
 expect(localStorage.length).toBe(0);
});

test('an old chat cannot write another account’s visibility after a cookie switch',async()=>{
 setup();
 const action={action:'set_application_visibility',applicationId:workspaceApplicationKey(app),hidden:true,conversationId:'chat-a',expectedSubject:'alice'};
 await setWorkspaceVisibility(action);
 const currentLayout={workspaceId:'workspace',layout:{applications:[app]}};
 request.mockImplementation(async path=>path==='/v1/api/auth/me'?{subject:'bob'}:currentLayout);
 await expect(setWorkspaceVisibility(action)).rejects.toThrow('session changed');
 expect(browserWorkspaceVisibility(workspaceVisibilityScope('workspace',{subject:'bob'})).hiddenApplications ?? []).toEqual([]);
 expect(browserWorkspaceVisibility(workspaceVisibilityScope('workspace',{subject:'alice'})).hiddenApplications).toEqual([action.applicationId]);
});
