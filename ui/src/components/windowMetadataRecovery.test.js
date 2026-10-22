// @vitest-environment jsdom
import React,{act} from 'react';
import {createRoot} from 'react-dom/client';
import {afterEach,expect,test,vi} from 'vitest';
import WindowContent from 'forge/components/WindowContent.jsx';
import {SettingProvider} from 'forge/core';
import {registerContainerRenderer} from 'forge/runtime/containerRendererRegistry.js';
import {removeSignalsForKey} from 'forge/core/store/signals.js';

registerContainerRenderer('test.metadata-recovery',()=>React.createElement('div',{'data-recovered-window':true},'Ready window'));
let root,host;
const windowId='metadata-recovery-window';
const flush=()=>act(async()=>{for(let i=0;i<30;i++)await Promise.resolve();});
afterEach(async()=>{
 if(root)await act(async()=>root.unmount());
 host?.remove();root=null;removeSignalsForKey(windowId);vi.restoreAllMocks();
});
async function mount(options={}){
 globalThis.IS_REACT_ACT_ENVIRONMENT=true;
 vi.spyOn(console,'error').mockImplementation(()=>{});
 let healthy=false,calls=0,permissions=0;
 const metadata={dataSource:{},view:{content:{id:'ready',kind:'test.metadata-recovery'}},...(options.protected?{authorization:{scope:'resource'}}:{})};
 const services={dataConnectorTransport:async()=>{
  calls++;if(!healthy)throw Object.assign(new Error('metadata unavailable'),{status:options.status || 500});
  return {data:metadata};
 },applyPermission:async value=>{permissions++;if(options.denyPermission)throw Object.assign(new Error('denied'),{status:403});return {...value.completeMetadata,authorizationSnapshot:{resources:{}}};}};
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 await act(async()=>root.render(React.createElement(SettingProvider,{connectorConfig:{window:{service:{uri:'/window'}}},services},React.createElement(WindowContent,{window:{windowId,windowKey:'recovery',parameters:{}}}))));
 await flush();
 return {recover:()=>{healthy=true;},calls:()=>calls,permissions:()=>permissions};
}
async function retry(times=1){await act(async()=>{for(let i=0;i<times;i++)window.dispatchEvent(new Event('forge:retry-window-metadata'));});await flush();}
test('layout retry event recovers the actual retained window and leaves a healthy window mounted',async()=>{
 const state=await mount();expect(host.querySelector('[role=alert]')).not.toBeNull();expect(state.calls()).toBe(1);
 state.recover();await retry();const content=host.querySelector('[data-recovered-window]');expect(content).not.toBeNull();expect(state.calls()).toBe(2);
 await retry();expect(state.calls()).toBe(2);expect(host.querySelector('[data-recovered-window]')).toBe(content);
});
test('duplicate recovery events issue one GET and repeat the declared preflight',async()=>{
 const state=await mount({protected:true});state.recover();await retry(2);
 expect(state.calls()).toBe(2);expect(state.permissions()).toBe(1);expect(host.querySelector('[data-recovered-window]')).not.toBeNull();
});
for(const status of [401,403,404,422])test(`recovery never retries denied/invalid status ${status}`,async()=>{
 const state=await mount({status});state.recover();await retry();expect(state.calls()).toBe(1);expect(host.querySelector('[role=alert]')).not.toBeNull();
});
test('permission denial after successful metadata recovery remains closed',async()=>{
 const state=await mount({protected:true,denyPermission:true});state.recover();await retry();
 expect(state.calls()).toBe(2);expect(state.permissions()).toBe(1);expect(host.querySelector('[role=alert]')).not.toBeNull();expect(host.querySelector('[data-recovered-window]')).toBeNull();
 await retry();expect(state.calls()).toBe(2);
});
