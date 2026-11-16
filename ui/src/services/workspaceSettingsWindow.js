import {addWindow} from 'forge/core';
import {MAIN_CHAT_WINDOW_ID, setScopedWorkspaceSelection} from './conversationWindow';
import {WORKSPACE_SETTINGS_WINDOW_KEY} from './workspaceSettingsIdentity';

// Host-owned utility window, using the same inline-metadata/render hook as
// other Agently settings/audit surfaces. It has no datasource or report binding.
export function openWorkspaceSettingsWindow(conversationId='') {
 const windowId=`workspace-settings:${encodeURIComponent(conversationId || 'main')}`;
 const entry = addWindow('Manage Workspace',MAIN_CHAT_WINDOW_ID,WORKSPACE_SETTINGS_WINDOW_KEY,null,true,{}, {
  windowId,conversationId:conversationId || undefined,presentation:'hosted',region:'chat.top',
  inlineMetadata:{windowKey:WORKSPACE_SETTINGS_WINDOW_KEY,dataSource:{},view:{content:{id:'workspace-settings'}}},
  workspaceObject:{version:1,objectId:`workspace:${windowId}`,conversationId:conversationId || undefined,kind:'window',origin:{},
   content:{renderer:'workspaceSettings',windowId,windowKey:WORKSPACE_SETTINGS_WINDOW_KEY},navigation:{label:'Manage Workspace',icon:'application'},
   placement:{preferred:'workspace',allowed:['workspace'],initialWorkspaceMode:'split'},lifecycle:{state:'ready',restorePolicy:'conversation',refreshPolicy:'explicit'},capabilities:{close:true,focus:true,split:true}},
 });
 // Record the explicit selection before transcript reconciliation can restore
 // the previous conversation workspace. Reuse its existing selection owner.
 setScopedWorkspaceSelection(conversationId,entry.windowId);
 return entry;
}
