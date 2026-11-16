import {getReportRestoreScope} from './agentlyClient';
import {request} from './httpClient';
import {MAIN_CHAT_WINDOW_ID,resolveConversationSelection} from './conversationWindow';
import {saveBrowserWorkspaceVisibility,workspaceVisibilityScope,workspaceApplicationKey,workspaceMenuKey} from './personalWorkspace';

export async function setWorkspaceVisibility(input){
 const conversationId=input.conversationId;
 const generation=getReportRestoreScope();
 const current=()=>getReportRestoreScope()===generation && resolveConversationSelection(MAIN_CHAT_WINDOW_ID)===conversationId;
 if(!conversationId || !current())throw new Error('Open this chat in the current browser and retry.');
 const layout=await request('/v1/workspace/layout',{notify:false,retries:0});
 if(!current())throw new Error('The session changed. Retry in the current workspace.');
 // A cookie can switch accounts in another tab without a local session event.
 // Read the current principal just before touching that user's browser storage.
 const principal=await request('/v1/api/auth/me',{notify:false,retries:0});
 if(!current() || !input.expectedSubject || principal?.subject!==input.expectedSubject){
  throw new Error('The session changed. Retry in the current workspace.');
 }
 const scope=workspaceVisibilityScope(layout?.workspaceId,principal);
 if(!scope)throw new Error('Could not identify your workspace. Reload it and try again.');
 const applications=layout?.layout?.applications || [];
 const menus=new Set();
 for(const app of applications){
  const visit=(items,path=[])=>{for(const item of items || []){const next=[...path,item.id];menus.add(workspaceMenuKey(app,next));visit(item.children,next);}};
  visit(app.menus);
 }
 if(input.action==='set_application_visibility'){
  if(!applications.some(app=>workspaceApplicationKey(app)===input.applicationId))throw new Error('That application is not available in this workspace.');
 }else if(input.action==='set_menu_visibility'){
  if(!menus.has(input.menuId))throw new Error('That menu is not available in this workspace.');
 }else throw new Error('Unsupported workspace visibility action.');
 saveBrowserWorkspaceVisibility(scope,input);
 return {status:'saved',storage:'browser',action:input.action,hidden:input.hidden};
}
