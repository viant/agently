// @vitest-environment jsdom
import React from 'react';
import {afterEach, expect, it, vi} from 'vitest';
import {act, cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {SettingProvider, activeWindows, selectedWindowId, removeWindow} from 'forge/core';
import {getWindowContext} from 'forge/core/context/registry.js';

const {getAuthMeSilently}=vi.hoisted(()=>({getAuthMeSilently:vi.fn()}));
vi.mock('../services/agentlyClient',()=>({getAuthMeSilently}));
vi.mock('./Sidebar',()=>({default:()=>null}));
vi.mock('./InlineReportWorkspace.jsx',()=>({default:()=>null}));
vi.mock('./ToolFeedDetail.jsx',()=>({default:()=>null}));
vi.mock('./mcpApps/AppRenderer.jsx',()=>({default:()=>null}));
vi.mock('forge/components',async()=>({
  WindowContent:(await import('forge/components/WindowContent.jsx')).default,
  Typography:(await import('forge/components/Typography.jsx')).default,
  typographyProps:(await import('forge/components/Typography.jsx')).typographyProps,
  Icon:(await import('forge/components/Icon.jsx')).default,
  SelectControl:(await import('forge/components/SelectControl.jsx')).default,
}));
import WorkspaceSidebar from './WorkspaceSidebar.jsx';
import ConversationWorkspaceSurface from './ConversationWorkspaceSurface.jsx';
import {applicationScope} from '../services/applicationScope.js';

function Workspace({sidebarVersion=0}){
  const [entry,setEntry]=React.useState(null);
  return <><WorkspaceSidebar key={sidebarVersion} onOpenWorkspace={()=>setEntry(activeWindows.peek().find(window=>window.windowId===selectedWindowId.peek()))}/>{entry?<ConversationWorkspaceSurface activeSurface="workspace" workspaceWindow={entry} workspaceWindows={[entry]} chatWindow={null}/>:null}</>;
}
afterEach(()=>{
  cleanup();
  for(const window of [...activeWindows.peek()])removeWindow(window.windowId);
  applicationScope.clear();vi.unstubAllGlobals();
});
it.each(['none','same actor','new actor','verified owner change','sidebar remount','pending unscoped response'])('applies scope from a menu-opened native window with %s recovery',async recovery=>{
  getAuthMeSilently.mockResolvedValue({subject:'scope-actor'});
  vi.stubGlobal('matchMedia',()=>({matches:false,addEventListener(){},removeEventListener(){}}));
  const app={id:'example',providerIdentity:'source',title:'Example',scope:{keys:['entityId']},menus:[{id:'select',title:'Choose entity',action:{type:'window',resourceUri:'window://example/selector'}}]};
  const queries=[];
  let holdNextLayout=false,releaseLayout;
  vi.stubGlobal('fetch',vi.fn(async url=>{
    const request=new URL(String(url),'http://localhost');
    if(request.pathname.endsWith('/workspace/layout')){
      const parameters=JSON.parse(request.searchParams.get('parameters')||'{}');queries.push(parameters);
      if(parameters.entityId){expect(request.searchParams.get('applicationId')).toBe('example');expect(request.searchParams.get('providerIdentity')).toBe('source');}
      const response=new Response(JSON.stringify({workspaceId:'scope-workspace',layout:{applications:[{...app,menus:[...app.menus,...(parameters.entityId?[{id:'rows',title:'Selected rows',action:{type:'window',windowKey:'rows'}}]:[])]}]}}),{headers:{'Content-Type':'application/json'}});
      if(holdNextLayout){holdNextLayout=false;return new Promise(resolve=>{releaseLayout=()=>resolve(response);});}
      return response;
    }
    if(request.pathname.includes('/forge/window/'))return new Response(JSON.stringify({data:{namespace:'fixture',dataSource:{},actionAliases:['actions'],actions:{code:'({select:({context,value})=>context.handlers.applicationScope.set(context,{entityId:Number(value)})})'},view:{content:{id:'selector-content',items:[{id:'entity',type:'select',label:'Scope entity',scope:'windowForm',dataField:'entityId',options:[{value:1,label:'First entity'},{value:2,label:'Second entity'}],on:[{event:'onChange',handler:'actions.select'}]},{id:'draft',type:'text',label:'Local draft',scope:'windowForm',dataField:'draft'}]}}}}),{headers:{'Content-Type':'application/json'}});
    throw new Error(`Unexpected request ${request.pathname}`);
  }));
  const services={applicationScope};
  const tree=version=><SettingProvider endpoints={{appAPI:{baseURL:'/v1/api'}}} connectorConfig={{window:{service:{uri:'agently/forge/window',endpoint:'appAPI'}}}} services={services}><Workspace sidebarVersion={version}/></SettingProvider>;
  const mounted=render(tree(0));
  fireEvent.click(await screen.findByRole('button',{name:'Choose entity'}));
  const apply=await screen.findByLabelText('Scope entity');
  const draft=screen.getByLabelText('Local draft');fireEvent.change(draft,{target:{value:'Retain draft'}});
  const windowId=selectedWindowId.peek();
  fireEvent.click(apply);fireEvent.click(await screen.findByRole('menuitem',{name:'First entity'}));
  await waitFor(()=>expect(queries.at(-1)).toEqual({entityId:1}));
  const context=getWindowContext(windowId);
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
