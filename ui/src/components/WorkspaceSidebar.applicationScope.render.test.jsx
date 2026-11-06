// @vitest-environment jsdom
import React from 'react';
import {afterEach, expect, it, vi} from 'vitest';
import {act, cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {SettingProvider, activeWindows, selectedWindowId, addWindow, removeWindow} from 'forge/core';
import {getWindowContext} from 'forge/core/context/registry.js';

const {getAuthMeSilently}=vi.hoisted(()=>({getAuthMeSilently:vi.fn()}));
vi.mock('../services/agentlyClient',()=>({getAuthMeSilently}));
vi.mock('./Sidebar',()=>({default:()=>null}));
vi.mock('./InlineReportWorkspace.jsx',()=>({default:()=>null}));
vi.mock('./ToolFeedDetail.jsx',()=>({default:()=>null}));
vi.mock('./mcpApps/AppRenderer.jsx',()=>({default:()=>null}));
vi.mock('./ConversationAuditDestination.jsx',()=>({default:()=>null,isConversationAuditWindow:()=>false}));
vi.mock('forge/components',async()=>({
  WindowContent:(await import('forge/components/WindowContent.jsx')).default,
  WindowManager:(await import('forge/components/WindowManager.jsx')).default,
  Typography:(await import('forge/components/Typography.jsx')).default,
  typographyProps:(await import('forge/components/Typography.jsx')).typographyProps,
  Icon:(await import('forge/components/Icon.jsx')).default,
  SelectControl:(await import('forge/components/SelectControl.jsx')).default,
}));
import WorkspaceSidebar from './WorkspaceSidebar.jsx';
import ConversationWorkspaceSurface from './ConversationWorkspaceSurface.jsx';
import AppSectionContent from './AppSectionContent.jsx';
import {applicationScope} from '../services/applicationScope.js';

function Workspace({sidebarVersion=0,sections=false}){
  const [entry,setEntry]=React.useState(null);
  const [route,setRoute]=React.useState({section:sections?'workspaces':'chat',destination:''});
  const [sectionVisible,setSectionVisible]=React.useState(false);
  return <>
    <WorkspaceSidebar key={sidebarVersion} section={route.section} destination={route.destination}
      onSectionChange={(section,destination)=>setRoute({section,destination})}
      onSectionWindow={setEntry} onSectionPresentationChange={setSectionVisible}
      onOpenWorkspace={()=>setEntry(activeWindows.peek().find(window=>window.windowId===selectedWindowId.peek()))}/>
    {sections?<button onClick={()=>setRoute({section:'workspaces',destination:'["missing"]'})}>Navigate unavailable destination</button>:null}
    {entry ? sections ? sectionVisible ? <AppSectionContent window={entry}/> : null
      : <ConversationWorkspaceSurface activeSurface="workspace" workspaceWindow={entry} workspaceWindows={[entry]} chatWindow={null}/> : null}
  </>;
}
afterEach(()=>{
  cleanup();
  window.localStorage.clear();
  for(const window of [...activeWindows.peek()])removeWindow(window.windowId);
  applicationScope.clear();vi.unstubAllGlobals();
  vi.restoreAllMocks();
});
it.each([
  'none','same actor','new actor','verified owner change','sidebar remount','pending unscoped response',
  'denied scope','denied scope 403','denied scope in sections','denied scope in sections 403',
  'denied scope in sections with identity reset','unauthorized scope 401',
  'omitted scoped application','omitted scoped application in sections',
  'omitted scoped application zero','omitted scoped application false',
  'omitted scoped application with peer','omitted scoped application in sections with peer',
  'omitted scoped application catalog removal','omitted scoped application in sections catalog removal',
  'omitted scoped application new actor','omitted scoped application in sections new actor',
])('applies scope from a menu-opened native window with %s recovery',async recovery=>{
  const sections=recovery.includes('sections');
  const omitted=recovery.startsWith('omitted scoped application');
  getAuthMeSilently.mockResolvedValue({subject:'scope-actor'});
  if(omitted)vi.spyOn(window,'setInterval').mockReturnValue(12345);
  vi.stubGlobal('matchMedia',()=>({matches:false,addEventListener(){},removeEventListener(){}}));
  const app={id:'example',providerIdentity:'source',title:'Example',scope:{keys:['entityId']},menus:[{id:'select',title:'Choose entity',action:{type:'window',resourceUri:'window://example/selector'}}]};
  const peer={id:'peer',providerIdentity:'source',title:'Peer',menus:[{id:'peer-rows',title:'Peer rows',action:{type:'window',windowKey:'peer'}}]};
  const queries=[];
  let holdNextLayout=false,releaseLayout,omissionReturned=false,newActorNoApps=false;
  vi.stubGlobal('fetch',vi.fn(async url=>{
    const request=new URL(String(url),'http://localhost');
    if(request.pathname.endsWith('/workspace/layout')){
      const parameters=JSON.parse(request.searchParams.get('parameters')||'{}');queries.push(parameters);
      if(parameters.entityId===3 || (omitted && (parameters.entityId===0 || parameters.entityId===false)) || newActorNoApps || (omissionReturned && recovery.includes('catalog removal') && !Object.hasOwn(parameters,'entityId'))){
        if(!omitted)return new Response('Projection unavailable',{status:recovery.includes('403')?403:recovery.includes('401')?401:503});
        omissionReturned=true;
        return new Response(JSON.stringify({workspaceId:'scope-workspace',layout:{...(sections?{left:{navigation:{presentation:'sections'}}}:{}),applications:recovery.includes('with peer')?[peer]:[]}}),{headers:{'Content-Type':'application/json'}});
      }
      if(parameters.entityId){expect(request.searchParams.get('applicationId')).toBe('example');expect(request.searchParams.get('providerIdentity')).toBe('source');}
      const response=new Response(JSON.stringify({workspaceId:'scope-workspace',layout:{...(sections?{left:{navigation:{presentation:'sections'}}}:{}),applications:[{...app,menus:[...app.menus,...(parameters.entityId?[{id:'rows',title:'Selected rows',action:{type:'window',windowKey:'rows'}}]:[])]},peer]}}),{headers:{'Content-Type':'application/json'}});
      if(holdNextLayout){holdNextLayout=false;return new Promise(resolve=>{releaseLayout=()=>resolve(response);});}
      return response;
    }
    if(request.pathname.includes('/forge/window/'))return new Response(JSON.stringify({data:{namespace:'fixture',dataSource:{},actionAliases:['actions'],actions:{code:'({select:({context,value})=>context.handlers.applicationScope.set(context,{entityId:value===false?false:Number(value)})})'},view:{content:{id:'selector-content',items:[{id:'entity',type:'select',label:'Scope entity',scope:'windowForm',dataField:'entityId',options:[{value:1,label:'First entity'},{value:2,label:'Second entity'},{value:3,label:'Unavailable entity'},{value:0,label:'Zero entity'},{value:false,label:'False entity'}],on:[{event:'onChange',handler:'actions.select'}]},{id:'draft',type:'text',label:'Local draft',scope:'windowForm',dataField:'draft'}]}}}}),{headers:{'Content-Type':'application/json'}});
    throw new Error(`Unexpected request ${request.pathname}`);
  }));
  const services={applicationScope};
  const tree=version=><SettingProvider endpoints={{appAPI:{baseURL:'/v1/api'}}} connectorConfig={{window:{service:{uri:'agently/forge/window',endpoint:'appAPI'}}}} services={services}><Workspace sidebarVersion={version} sections={sections}/></SettingProvider>;
  const mounted=render(tree(0));
  fireEvent.click(await screen.findByRole('button',{name:'Choose entity'}));
  const apply=await screen.findByLabelText('Scope entity');
  const draft=screen.getByLabelText('Local draft');fireEvent.change(draft,{target:{value:'Retain draft'}});
  const windowId=selectedWindowId.peek();
  fireEvent.click(apply);fireEvent.click(await screen.findByRole('menuitem',{name:'First entity'}));
  await waitFor(()=>expect(queries.at(-1)).toEqual({entityId:1}));
  const context=getWindowContext(windowId);
  if(omitted){
    await screen.findByRole('button',{name:'Selected rows'});
    const omittedValue=recovery.includes('zero')?0:recovery.includes('false')?false:3;
    const omittedLabel=omittedValue===0?'Zero entity':omittedValue===false?'False entity':'Unavailable entity';
    const windowsBefore=activeWindows.peek().map(entry=>entry.windowId);
    fireEvent.click(apply);fireEvent.click(await screen.findByRole('menuitem',{name:omittedLabel}));
    await waitFor(()=>expect(screen.queryByRole('button',{name:'Selected rows'})).toBeNull());
    expect(screen.queryByRole('button',{name:'Choose entity'})).toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(applicationScope.inputs(app)).toEqual({entityId:omittedValue});
    expect(context.handlers.applicationScope.get(context)).toEqual({entityId:omittedValue});
    expect(activeWindows.peek().map(entry=>entry.windowId)).toEqual(windowsBefore);
    expect(screen.getByLabelText('Local draft')).toBe(draft);expect(draft.value).toBe('Retain draft');
    if(recovery.includes('with peer'))expect(screen.getByRole('tab',{name:'Peer'})).toBeTruthy();
    if(recovery.includes('catalog removal')){
      await act(async()=>context.handlers.applicationScope.set(context,{entityId:null}));
      await waitFor(()=>expect(screen.queryByRole('tab',{name:'Example'})).toBeNull());
      expect(applicationScope.inputs(app)).toEqual({});
      expect(()=>context.handlers.applicationScope.set(context,{entityId:1})).toThrow('Application scope is unavailable');
      return;
    }
    if(recovery.includes('new actor')){
      newActorNoApps=true;getAuthMeSilently.mockResolvedValue({subject:'another-actor'});
      await act(async()=>{window.dispatchEvent(new Event('agently:session-reset'));window.dispatchEvent(new Event('agently:authorized'));});
      await waitFor(()=>expect(screen.queryByRole('tab',{name:'Example'})).toBeNull());
      expect(applicationScope.inputs(app)).toEqual({});
      expect(()=>context.handlers.applicationScope.set(context,{entityId:1})).toThrow('Application scope is unavailable');
      return;
    }
    fireEvent.click(apply);fireEvent.click(await screen.findByRole('menuitem',{name:'First entity'}));
    await screen.findByRole('button',{name:'Selected rows'});
    expect(context.handlers.applicationScope.get(context)).toEqual({entityId:1});
    expect(screen.getByLabelText('Local draft')).toBe(draft);
    expect(selectedWindowId.peek()).toBe(windowId);
    expect(screen.getAllByLabelText('Scope entity')).toHaveLength(1);
    return;
  }
  if(recovery==='unauthorized scope 401'){
    fireEvent.click(apply);fireEvent.click(await screen.findByRole('menuitem',{name:'Unavailable entity'}));
    await screen.findByRole('alert');
    expect(screen.queryByRole('button',{name:'Selected rows'})).toBeNull();
    expect(screen.queryByRole('tab',{name:'Example'})).toBeNull();
    expect(screen.queryByRole('tab',{name:'Peer'})).toBeNull();
    return;
  }
  if(recovery.startsWith('denied scope')){
    await screen.findByRole('button',{name:'Selected rows'});
    let child;
    await act(async()=>{child=addWindow('Dependent view',windowId,'dependent',null,true,{entityId:1},{inlineMetadata:{dataSource:{},view:{content:{id:'dependent-content',items:[{id:'notice',type:'label',value:'Dependent content'}]}}}});});
    fireEvent.click(apply);fireEvent.click(await screen.findByRole('menuitem',{name:'Unavailable entity'}));
    await screen.findByRole('alert');
    expect(screen.queryByRole('button',{name:'Selected rows'})).toBeNull();
    expect(screen.queryByRole('button',{name:'Choose entity'})).toBeNull();
    expect(context.handlers.applicationScope.get(context)).toEqual({entityId:3});
    expect(activeWindows.peek().some(entry=>entry.windowId===child.windowId)).toBe(false);
    expect(screen.getByLabelText('Local draft')).toBe(draft);expect(draft.value).toBe('Retain draft');
    if(recovery.includes('identity reset')){
      await act(async()=>window.dispatchEvent(new Event('agently:session-reset')));
      expect(screen.queryByLabelText('Local draft')).toBeNull();
      expect(applicationScope.inputs(app)).toEqual({});
      expect(()=>context.handlers.applicationScope.set(context,{entityId:1})).toThrow('Application scope is unavailable');
      return;
    }
    fireEvent.click(screen.getByRole('tab',{name:'Peer'}));
    expect(screen.getByRole('button',{name:'Peer rows'})).toBeTruthy();
    fireEvent.click(screen.getByRole('tab',{name:'Example'}));
    fireEvent.click(apply);fireEvent.click(await screen.findByRole('menuitem',{name:'First entity'}));
    await screen.findByRole('button',{name:'Selected rows'});
    expect(screen.queryByRole('alert')).toBeNull();
    expect(context.handlers.applicationScope.get(context)).toEqual({entityId:1});
    expect(selectedWindowId.peek()).toBe(windowId);
    if(sections){
      fireEvent.click(apply);fireEvent.click(await screen.findByRole('menuitem',{name:'Unavailable entity'}));
      await screen.findByRole('alert');
      const count=activeWindows.peek().length;
      fireEvent.click(screen.getByRole('button',{name:'Navigate unavailable destination'}));
      expect(screen.queryByLabelText('Local draft')).toBeNull();
      expect(activeWindows.peek()).toHaveLength(count);
      return;
    }
    fireEvent.click(screen.getByRole('tab',{name:'Peer'}));
    expect(screen.getByRole('button',{name:'Peer rows'})).toBeTruthy();
    return;
  }
  if(recovery==='pending unscoped response'){
    holdNextLayout=true;
    mounted.rerender(tree(1));
    await waitFor(()=>expect(releaseLayout).toBeTypeOf('function'));
    fireEvent.click(apply);fireEvent.click(await screen.findByRole('menuitem',{name:'Second entity'}));
    expect(applicationScope.inputs(app)).toEqual({entityId:2});
    await act(async()=>releaseLayout());
    await waitFor(()=>expect(queries.at(-1)).toEqual({entityId:2}));
    expect(selectedWindowId.peek()).toBe(windowId);
    expect(screen.getByLabelText('Local draft')).toBe(draft);expect(draft.value).toBe('Retain draft');
    expect(screen.getAllByLabelText('Scope entity')).toHaveLength(1);
    return;
  }
  if(recovery==='sidebar remount'){
    mounted.rerender(tree(1));
    await screen.findByRole('button',{name:'Choose entity'});
    await waitFor(()=>expect(queries.at(-1)).toEqual({entityId:1}));
  }
  if(recovery==='verified owner change'){
    const reads=queries.length;
    getAuthMeSilently.mockImplementationOnce(async()=>{
      window.dispatchEvent(new Event('agently:session-reset'));
      return {subject:'another-actor'};
    });
    await act(async()=>window.dispatchEvent(new Event('agently:authorized')));
    expect(queries).toHaveLength(reads);
    expect(applicationScope.inputs(app)).toEqual({});
    expect(()=>context.handlers.applicationScope.set(context,{entityId:2})).toThrow('Application scope is unavailable');
    return;
  }
  if(recovery==='new actor'){
    getAuthMeSilently.mockResolvedValue({subject:'another-actor'});
    await act(async()=>window.dispatchEvent(new Event('agently:session-reset')));
  }
  if(['same actor','new actor'].includes(recovery)){
    await act(async()=>window.dispatchEvent(new Event('agently:authorized')));
    await screen.findByRole('button',{name:'Choose entity'});
  }
  if(recovery==='new actor'){
    expect(queries.at(-1)).toEqual({});
    expect(applicationScope.inputs(app)).toEqual({});
    expect(()=>context.handlers.applicationScope.set(context,{entityId:2})).toThrow('Application scope is unavailable');
    return;
  }
  expect(applicationScope.inputs(app)).toEqual({entityId:1});
  fireEvent.click(apply);
  fireEvent.click(await screen.findByRole('menuitem',{name:'Second entity'}));
  await screen.findByRole('button',{name:'Selected rows'});
  await waitFor(()=>expect(queries.at(-1)).toEqual({entityId:2}));
  expect(selectedWindowId.peek()).toBe(windowId);
  expect(screen.getByLabelText('Local draft')).toBe(draft);expect(draft.value).toBe('Retain draft');
  expect(screen.getAllByLabelText('Scope entity')).toHaveLength(1);
});
