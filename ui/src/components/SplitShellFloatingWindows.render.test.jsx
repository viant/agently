// @vitest-environment jsdom
import React,{act} from 'react';
import {createRoot} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
vi.mock('forge/components',async()=>({WindowManager:(await import('forge/components/WindowManager.jsx')).default}));
import {SettingProvider,activeWindows,addWindow,removeWindow} from 'forge/core';
import {useWindowHandlers} from 'forge/hooks/window.js';
import {registerContainerRenderer} from 'forge/runtime/containerRendererRegistry.js';
import SplitShellFloatingWindows from './SplitShellFloatingWindows.jsx';

let root,node;const contexts=new Map();
globalThis.IS_REACT_ACT_ENVIRONMENT=true;
registerContainerRenderer('test.split-float',({context})=>{contexts.set(context.identity.windowId,context);return <button onClick={()=>context.handlers.window.commit({payload:context.signals.collection.peek()[0],context})}>Select original record</button>});
registerContainerRenderer('test.split-dialog',({context})=><button onClick={()=>context.handlers.dialog.commit({payload:context.signals.collection.peek()[0],context})}>Select dialog record</button>);
const flush=()=>act(async()=>{for(let i=0;i<35;i++)await Promise.resolve()});
afterEach(async()=>{if(root)await act(async()=>root.unmount());node?.remove();root=null;for(const entry of activeWindows.peek())removeWindow(entry.windowId);contexts.clear();vi.unstubAllGlobals()});
async function mount(){node=document.createElement('div');document.body.appendChild(node);root=createRoot(node);await act(async()=>root.render(<SettingProvider endpoints={{appAPI:{baseURL:'/v1/api'}}} connectorConfig={{window:{service:{uri:'/window',endpoint:'appAPI'}}}}><div data-chat-pane>Existing chat</div><SplitShellFloatingWindows/></SettingProvider>));await flush()}

it('renders ordinary openWindow modal children beside split chat without duplicate tabs or standalone scopes',async()=>{
 addWindow('Chat',null,'chat',null,true);
 addWindow('Standalone',null,'standalone',null,false,{}, {windowScopeId:'other-scope',inlineMetadata:{view:{content:{id:'other',text:'Standalone content'}},dataSource:{}}});
 const requests=[];vi.stubGlobal('fetch',vi.fn(async url=>{if(!String(url))return new Response(JSON.stringify({data:[{id:42}]}),{headers:{'Content-Type':'application/json'}});requests.push(String(url));return new Response(JSON.stringify({data:{windowKey:'ordinary',view:{content:{id:'ordinary',kind:'test.split-float',dataSourceRef:'rows'}},dataSource:{rows:{data:[{id:42}]}}}}),{headers:{'Content-Type':'application/json'}})}));
 await mount();const handlers=useWindowHandlers('chat');let selected;
 await act(async()=>{selected=handlers.openWindow({execution:{args:['ordinary','Ordinary','',false,{awaitResult:true,modal:true}]},context:{}})});await flush();
 expect(node.querySelector('[data-chat-pane]')?.textContent).toBe('Existing chat');
 expect(node.querySelectorAll('.floating-window')).toHaveLength(1);expect(requests).toHaveLength(1);expect(requests[0]).toContain('/window/ordinary');
 expect(node.textContent).not.toContain('Standalone content');expect(node.querySelectorAll('.bp4-tab')).toHaveLength(0);
 const button=Array.from(node.querySelectorAll('button')).find(button=>button.textContent==='Select original record');expect(button).toBeTruthy();await act(async()=>button.click());await flush();expect(await selected).toEqual({id:42});await flush();expect(node.querySelector('.floating-window')).toBeNull();
});

it('mounts the intact canonical picker Dialog and returns its selection through the same window lifecycle',async()=>{
 addWindow('Chat',null,'chat',null,true);
 vi.stubGlobal('fetch',vi.fn(async url=>{if(!String(url))return new Response(JSON.stringify({data:[{id:84}]}),{headers:{'Content-Type':'application/json'}});const query=new URL(String(url),'http://fixture').searchParams;expect(query.get('providerIdentity')).toBe('owning');expect(JSON.parse(query.get('resourceRef')).uri).toBe('window://external/picker');return new Response(JSON.stringify({data:{windowKey:'picker',view:{content:{id:'host'}},dataSource:{rows:{data:[{id:84}]}},dialogs:[{id:'originalDialog',title:'Original picker',dataSourceRef:'rows',content:{id:'dialog',kind:'test.split-dialog',dataSourceRef:'rows'}}]}}),{headers:{'Content-Type':'application/json'}})}));
 await mount();const handlers=useWindowHandlers('chat');let selected;
 await act(async()=>{selected=handlers.openWindow({execution:{args:['window://external/picker','','',false,{awaitResult:true,modal:true,resourceRef:{uri:'window://external/picker'},providerIdentity:'owning',pickerDialogId:'originalDialog'}]},context:{}})});await flush();
 expect(document.body.textContent).toContain('Original picker');const button=Array.from(document.querySelectorAll('button')).find(button=>button.textContent==='Select dialog record');expect(button).toBeTruthy();await act(async()=>button.click());await flush();expect(await selected).toEqual({id:84});await flush();expect(node.querySelector('.floating-window')).toBeNull();
});
