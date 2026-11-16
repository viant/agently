import React,{useState} from 'react';
import {Switch} from '@blueprintjs/core';
import WorkspaceLabelEditor from './WorkspaceLabelEditor';
import {workspaceApplicationKey,workspaceMenuKey} from '../services/personalWorkspace';

function Application({app,preferences,saving,visibilityDisabled,onChange}) {
 const [menusOpen,setMenusOpen]=useState(false);
 const appKey=workspaceApplicationKey(app);
 const menus=(items,parents=[]) => <ul className="workspace-application-menus">{(items || []).map(item=>{
  const path=[...parents,item.id],key=workspaceMenuKey(app,path);
  return <li key={item.id}><Switch label={item.title} aria-label={`Show ${item.title} menu in ${app.title}`} checked={!(preferences.hiddenMenus || []).includes(key)} disabled={saving || visibilityDisabled} onChange={event=>onChange({action:'set_menu_visibility',menuId:key,hidden:!event.target.checked})}/>
   <WorkspaceLabelEditor label={`Menu label for ${item.title}`} value={preferences.menuLabels?.[key] || ''} original={item.title} disabled={saving} onSave={label=>onChange({action:'rename_menu',menuId:key,label})}/>
   {item.children?.length ? menus(item.children,path) : null}</li>;
 })}</ul>;
 return <section className="app-ui-settings-card workspace-application-settings"><div>
  <h3 className="app-ui-settings-title">{app.title}</h3>
  <Switch label={`Show ${app.title} application tab`} checked={!(preferences.hiddenApplications || []).includes(appKey)} disabled={saving || visibilityDisabled} onChange={event=>onChange({action:'set_application_visibility',applicationId:appKey,hidden:!event.target.checked})}/>
  <WorkspaceLabelEditor label={`Application label for ${app.title}`} value={preferences.applicationLabels?.[appKey] || ''} original={app.title} disabled={saving} onSave={label=>onChange({action:'rename_application',applicationId:appKey,label})}/>
  <details onToggle={event=>setMenusOpen(event.currentTarget.open)}><summary>Menu labels and visibility</summary>{menusOpen && menus(app.menus)}</details>
 </div></section>;
}
export default function WorkspaceApplicationSettings({applications,preferences,saving,visibilityDisabled=false,onChange}) {
 return <><p>Choose the application tabs and menus you see. Hidden items remain available when you ask the assistant.</p>{applications.map(app=><Application key={workspaceApplicationKey(app)} app={app} preferences={preferences} saving={saving} visibilityDisabled={visibilityDisabled} onChange={onChange}/>)}</>;
}
