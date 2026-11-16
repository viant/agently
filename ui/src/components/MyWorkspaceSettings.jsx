import React, {useEffect, useLayoutEffect, useMemo, useRef, useState} from 'react';
import {Button, Callout, Dialog, InputGroup, Switch} from '@blueprintjs/core';
import {Button as ForgeButton, ForgeThemeBoundary, Icon, SelectControl} from 'forge/components';
import {request} from '../services/httpClient';
import {beginLogin,getAuthMeSilently} from '../services/agentlyClient';
import {sdkBaseURL} from '../endpoint';
import {uiSettingsReturnHref} from '../services/uiSettingsNavigation';
import PinExistingWorkspaceResource from './PinExistingWorkspaceResource';
import WorkspaceLabelEditor from './WorkspaceLabelEditor';
import WorkspaceApplicationSettings from './WorkspaceApplicationSettings';
import StarterTaskSettings from './StarterTaskSettings';
import {workspaceErrorMessage, workspaceResourceTitles, browserWorkspaceVisibility, visibilityKey, saveBrowserWorkspaceVisibility, workspaceVisibilityScope, notifyWorkspacePreferenceChange, WORKSPACE_PREFERENCE_EVENT, WORKSPACE_PREFERENCE_STORAGE_KEY} from '../services/personalWorkspace';
import './ConnectorSettings.css';
import './MyWorkspaceSettings.css';

const tabs=[{id:'organization',label:'Shortcuts'},{id:'applications',label:'Applications'},{id:'starting-tasks',label:'Starting tasks'}];
export default function MyWorkspaceSettings({embedded=false,initialTab}) {
  const [preferences,setPreferences]=useState(null),[applications,setApplications]=useState([]);
  const visibilityScope=useRef(''),serverPreferences=useRef(null);
  const preferenceOwner=useRef(null);
  const [visibilityReady,setVisibilityReady]=useState(false);
  const generation=useRef(0),preferenceRead=useRef(0),ownNotification=useRef(false),saveInFlight=useRef(false);
  const [pinning,setPinning]=useState(false),[newGroup,setNewGroup]=useState(null),[groupName,setGroupName]=useState('');
  const [removingGroup,setRemovingGroup]=useState(null);
  const [loadVersion,setLoadVersion]=useState(0);
  const [tab,setTab]=useState(()=>{const value=initialTab || (typeof window!=='undefined' && new URLSearchParams(window.location.search).get('tab'));return tabs.some(item=>item.id===value)?value:'organization';});
  const [saving,setSaving]=useState(false),[error,setError]=useState(''),[status,setStatus]=useState('');
  const [signInRequired,setSignInRequired]=useState(false);
  const needsNavigation=!signInRequired && tab!=='starting-tasks';
  useEffect(()=>{
    const controller=new AbortController(),current=++generation.current;
    visibilityScope.current='';setVisibilityReady(false);
    const clear=()=>{preferenceOwner.current=null;visibilityScope.current='';setVisibilityReady(false);++generation.current;controller.abort();serverPreferences.current=null;setPreferences(null);setApplications([]);setStatus('');setError('');setSaving(false);saveInFlight.current=false;setPinning(false);setNewGroup(null);setRemovingGroup(null);};
    const reset=()=>{clear();setLoadVersion(value=>value+1);};
    const refreshIdentity=()=>{if(!document.hidden)void getAuthMeSilently({refresh:true}).catch(()=>{});};
    window.addEventListener('focus',refreshIdentity);
    const signedOut=()=>{clear();setSignInRequired(true);};
    const recover=()=>{setSignInRequired(false);saveInFlight.current=false;setSaving(false);setLoadVersion(value=>value+1);};
    window.addEventListener('agently:session-reset',reset);window.addEventListener('agently:logout',signedOut);window.addEventListener('agently:unauthorized',signedOut);window.addEventListener('agently:authorized',recover);
    const refreshPreferences=()=>{
      if(ownNotification.current || !needsNavigation)return;
      const read=++preferenceRead.current;
      request('/v1/api/me/preferences',{signal:controller.signal,notify:false}).then(value=>{if(!controller.signal.aborted&&current===generation.current&&read===preferenceRead.current){if (!value?.preferenceOwnerId) throw new Error('Workspace account unavailable');
        if (preferenceOwner.current && preferenceOwner.current !== value.preferenceOwnerId) { reset(); return; }
        preferenceOwner.current=value.preferenceOwnerId;
        serverPreferences.current=value?.workspace || {};setPreferences(browserWorkspaceVisibility(visibilityScope.current,serverPreferences.current));}}).catch(cause=>{if(!controller.signal.aborted&&current===generation.current&&read===preferenceRead.current)setError(workspaceErrorMessage(cause, 'Your workspace could not be loaded. Try again.'));});
    };
    const externalChange=event=>{if(event.type==='storage'&&(event.key===null||event.key===visibilityKey(visibilityScope.current))){setPreferences(previous=>previous?browserWorkspaceVisibility(visibilityScope.current,serverPreferences.current || {}):previous);return;}if(event.detail?.localVisibility){if(event.detail.scope===visibilityScope.current&&!ownNotification.current)setPreferences(previous=>previous?browserWorkspaceVisibility(visibilityScope.current,serverPreferences.current || {}):previous);return;}if(event.type!=='storage'||event.key===WORKSPACE_PREFERENCE_STORAGE_KEY)refreshPreferences();};
    window.addEventListener(WORKSPACE_PREFERENCE_EVENT,externalChange);window.addEventListener('storage',externalChange);
    refreshPreferences();
    if(needsNavigation)fetch(`${sdkBaseURL}/workspace/layout`,{credentials:'include',signal:controller.signal,headers:{Accept:'application/json'}}).then(async response=>{if(!response.ok)throw Object.assign(new Error('Applications unavailable'),{status:response.status});return response.json();}).then(value=>{if(!controller.signal.aborted&&current===generation.current){setApplications(value?.layout?.applications || []);if(tab==='applications')void getAuthMeSilently().then(principal=>{if(!controller.signal.aborted&&current===generation.current){visibilityScope.current=workspaceVisibilityScope(value.workspaceId,principal);setVisibilityReady(!!visibilityScope.current);setPreferences(previous=>previous?browserWorkspaceVisibility(visibilityScope.current,serverPreferences.current || {}):previous);}}).catch(cause=>{if(!controller.signal.aborted&&current===generation.current)setError(workspaceErrorMessage(cause, 'Your workspace could not be loaded. Try again.'));});}}).catch(cause=>{if(!controller.signal.aborted&&current===generation.current)setError(workspaceErrorMessage(cause, 'Your workspace could not be loaded. Try again.'));});
    return()=>{++generation.current;controller.abort();window.removeEventListener('focus',refreshIdentity);window.removeEventListener('agently:session-reset',reset);window.removeEventListener('agently:logout',signedOut);window.removeEventListener('agently:unauthorized',signedOut);window.removeEventListener('agently:authorized',recover);window.removeEventListener(WORKSPACE_PREFERENCE_EVENT,externalChange);window.removeEventListener('storage',externalChange);};
  },[loadVersion,needsNavigation,tab]);
  const change=async action=>{
    if(saveInFlight.current)return false;
    saveInFlight.current=true;const current=generation.current;setSaving(true);setError('');setStatus('');
    try{
      if(['set_application_visibility','set_menu_visibility'].includes(action.action)){
        const principal=await getAuthMeSilently({refresh:true});
        if(current!==generation.current)return false;
        const displayedScope=visibilityScope.current;
        if(!displayedScope || workspaceVisibilityScope(JSON.parse(displayedScope)[0],principal)!==displayedScope){
          setLoadVersion(value=>value+1);
          return false;
        }
        ownNotification.current=true;try{saveBrowserWorkspaceVisibility(visibilityScope.current,action);}finally{ownNotification.current=false;}
        setPreferences(previous=>browserWorkspaceVisibility(visibilityScope.current,serverPreferences.current || {}));setStatus('Visibility saved in this browser.');return true;
      }
      const expectedOwnerId = preferenceOwner.current;
      if (!expectedOwnerId) throw new Error('Workspace account unavailable');
      await request('/v1/api/me/preferences',{method:'PATCH',body:JSON.stringify({expectedOwnerId,workspaceAction:action}),notify:false});
      const read=++preferenceRead.current,value=await request('/v1/api/me/preferences',{notify:false});
      if(current!==generation.current)return false;
      if (value?.preferenceOwnerId !== preferenceOwner.current) {
        window.dispatchEvent(new Event('agently:session-reset'));
        return false;
      }
      if(read===preferenceRead.current){serverPreferences.current=value?.workspace || {};setPreferences(browserWorkspaceVisibility(visibilityScope.current,serverPreferences.current));}
      setStatus(action.action==='pin'&&value?.workspace?.enabled===false?'Pinned. My workspace is hidden; turn it on to show this shortcut.':'Workspace saved.');
      ownNotification.current=true;try{notifyWorkspacePreferenceChange();}finally{ownNotification.current=false;}return true;
    }catch(cause){if(current===generation.current)setError(workspaceErrorMessage(cause, ['set_application_visibility','set_menu_visibility'].includes(action.action) ? 'Could not save visibility in this browser. Check browser storage settings and try again.' : 'Could not confirm the save. Reload your workspace to check before trying again.'));return false;}
    finally{if(current===generation.current){saveInFlight.current=false;setSaving(false);}}
  };
  const keyboardMove = useRef(null);
  const move = (event, action) => {
    if (event.detail === 0) keyboardMove.current = event.currentTarget;
    return change(action);
  };
  useLayoutEffect(() => {
    const button = keyboardMove.current;
    if (saving || !button) return;
    keyboardMove.current = null;
    if (!button.isConnected || (document.activeElement !== document.body && document.activeElement !== button)) return;
    const row = button.closest('li, .my-workspace-group-heading');
    const next = button.disabled ? row?.querySelector('[data-workspace-move]:enabled') : button;
    next?.focus();
  }, [saving]);
  const groups=preferences?.groups || [];
  const titles=useMemo(()=>workspaceResourceTitles({applications}),[applications]);
  const pinTitle=item=>item.alias || titles.get(JSON.stringify([item.provider,item.uri])) || `Pinned ${item.kind}`;
  const createGroup=async event=>{event.preventDefault();if(!groupName.trim()||saving)return;if(await change({action:'create_group',groupId:newGroup,label:groupName.trim()})){setNewGroup(null);setGroupName('');}};
  return <ForgeThemeBoundary windowKey="workspace-settings"><main className={`app-ui-settings my-workspace-settings${embedded?' is-embedded':''}`} aria-label="Manage Workspace">
    <header className="app-ui-settings-header">{!embedded&&<a href={uiSettingsReturnHref()} className="app-ui-settings-back"><Icon name="lucide:arrow-left"/> Agently</a>}<h1>Manage Workspace</h1></header>
    <p>Customize your shortcuts, application tabs, menus, and starting prompts.</p>
    <div className="app-settings-tabs" role="group" aria-label="Workspace settings">{tabs.map(item=><Button key={item.id} minimal disabled={saving} aria-pressed={tab===item.id} active={tab===item.id} onClick={()=>setTab(item.id)}>{item.label}</Button>)}</div>
    {error&&<Callout intent="danger" role="alert">{error}</Callout>}<p role="status" aria-live="polite">{saving?'Saving…':status}</p>
    {signInRequired?<section className="app-ui-settings-card"><div><h2 className="app-ui-settings-title">Sign in to manage your workspace</h2><Button onClick={()=>beginLogin()}>Sign in</Button></div></section>:tab==='starting-tasks'?<StarterTaskSettings key={loadVersion}/>:!preferences?error?<Button onClick={()=>setLoadVersion(value=>value+1)}>Retry loading workspace</Button>:<p>Loading your workspace…</p>:tab==='applications'?<WorkspaceApplicationSettings applications={applications} preferences={preferences} saving={saving} visibilityDisabled={!visibilityReady} onChange={change}/>:<>
      <section className="app-ui-settings-card"><div><h2 className="app-ui-settings-title">Workspace title</h2><WorkspaceLabelEditor label="Workspace title" value={preferences.title || ''} original="My workspace" disabled={saving} onSave={label=>change({action:'set_title',label})}/></div></section>
      <div className="my-workspace-organize-actions"><Button intent="primary" icon={<Icon name="lucide:plus"/>} disabled={saving||pinning} onClick={()=>{setStatus('');setPinning(true);}}>Pin existing window or report</Button><Button disabled={saving||!!newGroup} onClick={()=>{setNewGroup(crypto.randomUUID());setGroupName('');}}>New group</Button></div>
      {pinning&&<PinExistingWorkspaceResource applications={applications} groups={groups} saving={saving} onPin={change} onCancel={()=>setPinning(false)}/>}
      {newGroup&&<section className="app-ui-settings-card"><form onSubmit={createGroup} className="workspace-new-group"><label>New group name<InputGroup aria-label="New group name" value={groupName} maxLength={120} disabled={saving} onChange={event=>setGroupName(event.target.value)}/></label><Button type="button" disabled={saving} onClick={()=>setNewGroup(null)}>Cancel</Button><Button type="submit" intent="primary" disabled={saving||!groupName.trim()}>Create group</Button></form></section>}
      <section className="app-ui-settings-card"><div><h2 className="app-ui-settings-title">Show My workspace</h2><p>Make your pinned windows and reports available from My workspace.</p></div><Switch aria-label="Show My workspace" checked={preferences.enabled===true} disabled={saving} onChange={event=>change({action:'set_enabled',enabled:event.target.checked})}/></section>
      <section className="app-ui-settings-card"><div><h2 className="app-ui-settings-title">Shortcut layout</h2><p>Choose how My workspace shows your pinned windows and reports.</p></div><SelectControl aria-label="My workspace layout" value={preferences.presentation || 'menu'} disabled={saving} onChange={event=>change({action:'set_presentation',presentation:event.target.value})}><option value="menu">Menu</option><option value="tiles">Tiles</option><option value="both">Both</option></SelectControl></section>
      {!groups.some(group=>group.items?.length)&&<p>Pin a window or report to make it easy to reopen.</p>}
      {groups.map((group,groupIndex)=><section key={group.id} className="app-ui-settings-card" aria-label={group.label}><div><div className="my-workspace-group-heading"><WorkspaceLabelEditor label={`Group name for ${group.label}`} value={group.label} disabled={saving} allowEmpty={false} heading onSave={label=>change({action:'rename_group',groupId:group.id,label})}/><div>
        <ForgeButton variant="ghost" hideLabel data-workspace-move="" disabled={saving||groupIndex===0} aria-label={`Move ${group.label} group up`} icon="lucide:arrow-up" onClick={event=>move(event,{action:'move_group',groupId:group.id,position:groupIndex-1})}/>
        <ForgeButton variant="ghost" hideLabel data-workspace-move="" disabled={saving||groupIndex===groups.length-1} aria-label={`Move ${group.label} group down`} icon="lucide:arrow-down" onClick={event=>move(event,{action:'move_group',groupId:group.id,position:groupIndex+1})}/>
        <Button minimal disabled={saving} aria-label={`Remove ${group.label} group`} onClick={()=>setRemovingGroup(group)}>Remove</Button>
      </div></div><ul className="my-workspace-pin-list">{(group.items || []).map((item,index)=><li key={item.id}><div className="my-workspace-pin-name"><Icon name={item.kind==='window'?'lucide:panel-right':'lucide:chart-no-axes-combined'}/><WorkspaceLabelEditor label={`Menu label for ${pinTitle(item)}`} value={item.alias || ''} original={pinTitle(item)} disabled={saving} onSave={label=>change({action:'rename_item',itemId:item.id,label})}/><span className="my-workspace-pin-kind">{item.kind==='window'?'Window':'Report'}</span>
        {groups.length>1&&<SelectControl aria-label={`Group for ${pinTitle(item)}`} value={group.id} disabled={saving} onChange={event=>change({action:'move_item',itemId:item.id,targetGroupId:event.target.value})}>{groups.map(target=><option key={target.id} value={target.id}>{target.label}</option>)}</SelectControl>}</div>
        <ForgeButton variant="ghost" hideLabel data-workspace-move="" disabled={saving||index===0} aria-label={`Move ${pinTitle(item)} up`} icon="lucide:arrow-up" onClick={event=>move(event,{action:'move_item',itemId:item.id,position:index-1})}/>
        <ForgeButton variant="ghost" hideLabel data-workspace-move="" disabled={saving||index===group.items.length-1} aria-label={`Move ${pinTitle(item)} down`} icon="lucide:arrow-down" onClick={event=>move(event,{action:'move_item',itemId:item.id,position:index+1})}/>
        <Button minimal disabled={saving} aria-label={`Unpin ${pinTitle(item)}`} onClick={()=>change({action:'unpin',itemId:item.id})}>Unpin</Button>
      </li>)}</ul></div></section>)}
      {!!preferences.defaults?.length&&<section className="app-ui-settings-card"><div><h2 className="app-ui-settings-title">Preferred report versions</h2><p>Used for unqualified requests to the assistant. Exact menu pins keep their targets.</p><ul>{preferences.defaults.map(entry=><li key={JSON.stringify([entry.sourceProvider,entry.sourceUri])}>{titles.get(JSON.stringify([entry.sourceProvider,entry.sourceUri])) || 'Original report'} → {groups.flatMap(group=>group.items || []).find(item=>item.uri===entry.targetUri)?.alias || 'Personal report'}</li>)}</ul><p>Ask the assistant to change a preferred version or use the original again.</p></div></section>}
    </>}
    <Dialog isOpen={!!removingGroup} onClose={()=>!saving&&setRemovingGroup(null)} title="Remove group" style={{width:'min(92vw,480px)'}}><div className="workspace-remove-group"><p>Remove {removingGroup?.label} and its menu shortcuts? The windows and reports remain available.</p><div><Button disabled={saving} onClick={()=>setRemovingGroup(null)}>Cancel</Button><Button intent="danger" loading={saving} onClick={async()=>{if(await change({action:'remove_group',groupId:removingGroup.id}))setRemovingGroup(null);}}>Remove group</Button></div></div></Dialog>
  </main></ForgeThemeBoundary>;
}
