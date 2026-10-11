// @vitest-environment jsdom
import React, {act} from 'react';
import {createRoot} from 'react-dom/client';
import {afterEach, expect, it, vi} from 'vitest';
// Load the actual manager/content implementations without unrelated barrel exports.
vi.mock('forge/components',async()=>({WindowContent:(await import('forge/components/WindowContent.jsx')).default,WindowManager:(await import('forge/components/WindowManager.jsx')).default}));
import {SettingProvider} from 'forge/core';
import {registerContainerRenderer} from 'forge/runtime/containerRendererRegistry.js';
import {activeWindows,addWindow,removeWindow,getCollectionSignal,removeSignalsForKey} from 'forge/core/store/signals.js';
import {getWindowContext} from 'forge/core/context/registry.js';
import {prepareAgentlyDataConnectorRequest} from '../../services/datasourceRequestContext.js';
import MCPUIForgeWindowPage, {buildWindowPayload} from './MCPUIForgeWindowPage.jsx';
import StandaloneForgeWindowSurface from './StandaloneForgeWindowSurface.jsx';
import WindowManager from 'forge/components/WindowManager.jsx';

const parameters={EntityID:['9007199254740993'],nested:{selection:{region:'west'}},Limit:7};
let root,element;
const contexts=new Map();
const mounts=new Map();
registerContainerRenderer('test.native-navigation',({context})=>{
 contexts.set(context.identity.windowId,context);
 React.useEffect(()=>{ mounts.set(context.identity.windowId,(mounts.get(context.identity.windowId)||0)+1); },[]);
 const key=context.windowState.windowKey;
 return <section data-native-window={key}>
   <span>{key} native content</span>
   {key==='Root' ? <><button onClick={()=>context.handlers.window.openWindow({context,execution:{args:['Child','Child',null,true]},parameters})}>Open original child</button><button onClick={()=>context.handlers.window.openWindow({context,execution:{args:['ChildFloat','Child floating',null,false]},parameters})}>Open original floating child</button></> : null}
 </section>;
});
const flush=()=>act(async()=>{for(let i=0;i<35;i++)await Promise.resolve();});
afterEach(async()=>{
 if(root)await act(async()=>root.unmount());element?.remove();root=null;
 for(const entry of activeWindows.peek())removeWindow(entry.windowId);
 contexts.clear();mounts.clear();vi.unstubAllGlobals();
});
function admitted(key){
 const selectionBinding={actor:{issuer:'issuer',subject:'actor',tenantId:'tenant',accountId:'account'},instanceId:`${key}-instance`,parametersDigest:`${key}-digest`,token:`${key}-signed`,opaque:{preserved:true}};
 const resource={uri:`window://example/${key}`,kind:'working',contentFingerprint:`${key}-pin`,authorityBinding:`${key}-authority`,validUntil:'2030-01-01T00:00:00.123456789Z'};
 const resourceTarget={platform:'web',formFactor:'desktop',selectionToken:`${key}-target`,dependencyPins:{rows:{uri:`datasource://example/${key}/rows`,contentFingerprint:`${key}-source`}},executionProof:{token:`${key}-proof`}};
 return {resource,resourceTarget,selectionBinding,authorization:{scope:'resource'},authorizationSnapshot:{schemaVersion:2,expiresAt:'2000-01-01T00:00:00Z',resource:{capabilities:{read:false,write:false}}},dataSource:{rows:{service:{uri:`datasources/${key}/fetch`,endpoint:'appAPI',method:'POST'},selectionMode:'none'}},view:{content:{id:`${key}-content`,kind:'test.native-navigation',dataSourceRef:'rows'}}};
}
it.each(['signed','unsigned'])('opens an authored child, returns without remounting, and owns only its admitted windows (%s)',async(delivery)=>{
 const frame=key=>{const value=admitted(key);if(delivery==='unsigned'){delete value.selectionBinding;delete value.authorization;delete value.authorizationSnapshot;}return value;};
 let releaseChild;
 globalThis.IS_REACT_ACT_ENVIRONMENT=true;
 const unrelated=addWindow('Unrelated','foreign','Foreign','',true,{}, {inlineMetadata:{view:{content:{id:'foreign',kind:'test.native-navigation'}}}});
 const foreignFloat=addWindow('Foreign float','foreign','ForeignFloat','',false,{}, {inlineMetadata:{view:{content:{id:'foreign-float',kind:'test.native-navigation'}}}});
 window.history.replaceState({},'',`/mcp-ui/forge-window?${new URLSearchParams({windowKey:'Root',windowParams:JSON.stringify(parameters)})}`);
 const opens={Root:0,Child:0,ChildFloat:0},posts=[];
 vi.stubGlobal('fetch',vi.fn(async(url,options={})=>{
  const request=new URL(String(url),'http://localhost');
  const key=request.pathname.split('/').pop();
  if(request.pathname.includes('/agently/forge/window/')){
   expect(opens).toHaveProperty(key);
   opens[key]++;
   expect(opens[key]).toBe(1);
   expect(request.searchParams.get('applyPermission')).toBe('true');
   expect(JSON.parse(request.searchParams.get('windowParams'))).toEqual(parameters);
   expect(request.searchParams.get('platform')).toBe('web');
   expect(request.searchParams.get('formFactor')).toBe('desktop');
   if(delivery==='unsigned'&&key==='Child')await new Promise(resolve=>{releaseChild=resolve});
   return new Response(JSON.stringify({data:frame(key)}),{status:200,headers:{'Content-Type':'application/json'}});
  }
  if(request.pathname.endsWith('/fetch')){
   const owner=request.pathname.split('/').at(-2);
   const body=JSON.parse(options.body);posts.push({owner,body});
   expect(body).toMatchObject({resource:frame(owner).resource,target:frame(owner).resourceTarget,...(delivery==='signed'?{selectionBinding:frame(owner).selectionBinding}:{}),windowParameters:parameters});
   return new Response(JSON.stringify({data:[{id:`${owner}-row`}]}),{status:200,headers:{'Content-Type':'application/json'}});
  }
  throw new Error(`Unexpected request ${request.pathname}`);
 }));
 const applyPermission=vi.fn(()=>{throw new Error('Duplicate UI permission preflight')});
 element=document.createElement('div');document.body.append(element);root=createRoot(element);
 await act(async()=>root.render(<SettingProvider endpoints={{appAPI:{baseURL:'/v1/api'}}} connectorConfig={{window:{service:{uri:'agently/forge/window',endpoint:'appAPI'}}}} services={{prepareDataConnectorRequest:prepareAgentlyDataConnectorRequest,applyPermission}} targetContext={{platform:'web',formFactor:'desktop'}}><MCPUIForgeWindowPage/></SettingProvider>));
 await flush();
 expect(element.querySelector('[data-native-window="Root"]')).not.toBeNull();
 expect(element.textContent).not.toContain('Foreign');
 const parent=[...contexts.entries()].find(([,context])=>context.windowState.windowKey==='Root');
 const parentContext=parent[1];
 await act(async()=>Array.from(element.querySelectorAll('button')).find(button=>button.textContent==='Open original child').click());await flush();
 if(delivery==='unsigned'){expect(element.querySelector('[data-native-window="Child"]')).toBeNull();expect(posts.map(post=>post.owner)).toEqual(['Root']);await act(async()=>releaseChild());await flush();}
 expect(element.querySelector('[data-native-window="Child"]')).not.toBeNull();
 const rootTab=Array.from(element.querySelectorAll('[role="tab"]')).find(tab=>tab.textContent.includes('Root'));
 expect(rootTab).toBeDefined();
 await act(async()=>rootTab.click());await flush();
 expect(element.querySelector('[data-native-window="Root"]')).not.toBeNull();
 expect(mounts.get(parent[0])).toBe(1);
 expect(getWindowContext(parent[0]).windowState.selectionBinding).toEqual(frame("Root").selectionBinding);
 expect(getWindowContext(parent[0]).windowState.openedWindowParameters).toEqual(parameters);
 expect(opens).toEqual({Root:1,Child:1,ChildFloat:0});
 expect(posts.map(post=>post.owner)).toEqual(['Root','Child']);
 expect(applyPermission).not.toHaveBeenCalled();
 expect(getCollectionSignal(`${parent[0]}DSrows`).peek()).toEqual([{id:'Root-row'}]);
 await act(async()=>Array.from(element.querySelectorAll('button')).find(button=>button.textContent==='Open original floating child').click());await flush();
 expect(element.querySelector('.floating-window [data-native-window="ChildFloat"]')).not.toBeNull();
 expect(element.textContent).not.toContain('Foreign');
 await act(async()=>window.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape'})));await flush();
 expect(element.querySelector('[data-native-window="ChildFloat"]')).toBeNull();
 expect(activeWindows.peek().some(entry=>entry.windowId===foreignFloat.windowId)).toBe(true);
 expect(opens).toEqual({Root:1,Child:1,ChildFloat:1});
 expect(posts.map(post=>post.owner)).toEqual(['Root','Child','ChildFloat']);
 await act(async()=>root.unmount());root=null;
 expect(activeWindows.peek().map(entry=>entry.windowId).sort()).toEqual([unrelated.windowId,foreignFloat.windowId].sort());
});

it('disposes children and rejects late admission when the same root key/parameters receives a new target and signed revision',async()=>{
 globalThis.IS_REACT_ACT_ENVIRONMENT=true;
 let resolveChild;
 const loadWindow=vi.fn(()=>new Promise(resolve=>{resolveChild=resolve}));
 const posts=[];
 vi.stubGlobal('fetch',vi.fn(async(url,options={})=>{
  const request=new URL(String(url),'http://localhost');
  expect(request.pathname).toBe('/v1/api/datasources/Root/fetch');posts.push(JSON.parse(options.body));
  return new Response(JSON.stringify({data:[{id:'Root-row'}]}),{status:200,headers:{'Content-Type':'application/json'}});
 }));
 element=document.createElement('div');document.body.append(element);root=createRoot(element);
 const services={prepareDataConnectorRequest:prepareAgentlyDataConnectorRequest,applyPermission:()=>{throw new Error('Duplicate permission preflight')}};
 const original=buildWindowPayload('Root',{data:admitted('Root')},parameters);
 const render=(selected,target)=>root.render(<SettingProvider endpoints={{appAPI:{baseURL:'/v1/api'}}} connectorConfig={{window:{service:{uri:'agently/forge/window',endpoint:'appAPI'}}}} services={services} targetContext={target}><StandaloneForgeWindowSurface selectedWindow={selected} loadWindow={loadWindow} targetContext={target}/></SettingProvider>);
 await act(async()=>render(original,{platform:'web',formFactor:'desktop'}));await flush();
 const oldRoot=activeWindows.peek().find(entry=>entry.windowKey==='Root');
 await act(async()=>Array.from(element.querySelectorAll('button')).find(button=>button.textContent==='Open original child').click());await flush();
 expect(loadWindow).toHaveBeenCalledTimes(1);expect(posts).toHaveLength(1);
 expect(element.querySelector('[data-native-window="Child"]')).toBeNull();expect(element.textContent).toContain('Loading workspace window');
 const pendingChild=activeWindows.peek().find(entry=>entry.windowKey==='Child');
 await act(async()=>removeWindow(pendingChild.windowId));await flush();
 expect(element.querySelector('[data-native-window="Child"]')).toBeNull();expect(posts).toHaveLength(1);
 const updated=buildWindowPayload('Root',{data:{...admitted('Root'),resource:{...admitted('Root').resource,contentFingerprint:'revision-two'},selectionBinding:{...admitted('Root').selectionBinding,token:'new-root-binding',instanceId:'new-root-instance'}}},parameters);
 await act(async()=>render(updated,{platform:'web',formFactor:'tablet'}));await flush();
 const newRoot=activeWindows.peek().find(entry=>entry.windowKey==='Root');
 expect(newRoot.windowId).not.toBe(oldRoot.windowId);expect(newRoot.id).toBe(original.id);expect(newRoot.stateKey).toBe(original.stateKey);
 expect(activeWindows.peek().some(entry=>entry.windowKey==='Child')).toBe(false);
 expect(posts).toHaveLength(2);expect(posts[1].selectionBinding).toEqual(updated.selectionBinding);expect(posts[1].resource).toEqual(updated.resource);
 await act(async()=>resolveChild(buildWindowPayload('Child',{data:admitted('Child')},parameters)));await flush();
 expect(activeWindows.peek().some(entry=>entry.windowKey==='Child')).toBe(false);expect(posts).toHaveLength(2);
});

it('retains the default manager behavior for ordinary tabs and floating windows',async()=>{
 globalThis.IS_REACT_ACT_ENVIRONMENT=true;
 const frame=key=>buildWindowPayload(key,{data:{...admitted(key),dataSource:{},view:{content:{id:key,kind:'test.native-navigation'}}}},{});
 const tab=addWindow('Default tab','default','DefaultTab','',true,{},frame('DefaultTab'));
 const floating=addWindow('Default floating','default','DefaultFloat','',false,{},frame('DefaultFloat'));
 element=document.createElement('div');document.body.append(element);root=createRoot(element);
 await act(async()=>root.render(<SettingProvider connectorConfig={{window:{service:{uri:'agently/forge/window',endpoint:'appAPI'}}}}><WindowManager/></SettingProvider>));await flush();
 expect(element.querySelector('[data-native-window="DefaultTab"]')).not.toBeNull();
 expect(element.querySelector('.floating-window [data-native-window="DefaultFloat"]')).not.toBeNull();
 await act(async()=>window.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape'})));await flush();
 expect(activeWindows.peek().some(entry=>entry.windowId===floating.windowId)).toBe(false);
 expect(activeWindows.peek().some(entry=>entry.windowId===tab.windowId)).toBe(true);
});

it('shows a child HTTP admission error without mounting its content or fetching its data',async()=>{
 globalThis.IS_REACT_ACT_ENVIRONMENT=true;
 window.history.replaceState({},'',`/mcp-ui/forge-window?${new URLSearchParams({windowKey:'Root',windowParams:JSON.stringify(parameters)})}`);
 const posted=[];
 vi.stubGlobal('fetch',vi.fn(async(url,options={})=>{
  const request=new URL(String(url),'http://localhost');
  if(request.pathname==='/v1/api/agently/forge/window/Child')return new Response('child unavailable',{status:403});
  if(request.pathname==='/v1/api/agently/forge/window/Root'){
   const value=admitted('Root');delete value.selectionBinding;delete value.authorization;delete value.authorizationSnapshot;
   return new Response(JSON.stringify({data:value}),{status:200,headers:{'Content-Type':'application/json'}});
  }
  expect(request.pathname).toBe('/v1/api/datasources/Root/fetch');posted.push(JSON.parse(options.body));
  return new Response(JSON.stringify({data:[{id:'Root-row'}]}),{status:200,headers:{'Content-Type':'application/json'}});
 }));
 element=document.createElement('div');document.body.append(element);root=createRoot(element);
 await act(async()=>root.render(<SettingProvider endpoints={{appAPI:{baseURL:'/v1/api'}}} connectorConfig={{window:{service:{uri:'agently/forge/window',endpoint:'appAPI'}}}} services={{prepareDataConnectorRequest:prepareAgentlyDataConnectorRequest,applyPermission:()=>{throw new Error('Duplicate preflight')}}} targetContext={{platform:'web',formFactor:'desktop'}}><MCPUIForgeWindowPage/></SettingProvider>));await flush();
 await act(async()=>Array.from(element.querySelectorAll('button')).find(button=>button.textContent==='Open original child').click());await flush();
 expect(element.querySelector('[role="alert"]')?.textContent).toBe('child unavailable');
 expect(element.querySelector('[data-native-window="Child"]')).toBeNull();expect(posted).toHaveLength(1);
});
