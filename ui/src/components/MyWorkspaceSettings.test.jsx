// @vitest-environment jsdom
import React from 'react';
import {act,fireEvent,render,screen,waitFor,cleanup} from '@testing-library/react';
import {expect,test,vi,afterEach} from 'vitest';
import MyWorkspaceSettings from './MyWorkspaceSettings';
import {request} from '../services/httpClient';
import {getAuthMeSilently} from '../services/agentlyClient';
vi.mock('forge/components',()=>({Button:({variant,hideLabel,icon,...props})=><button {...props}/>,ForgeThemeBoundary:({children})=><>{children}</>,Icon:()=>null,SelectControl:props=><select {...props}/>}));
vi.mock('../services/agentlyClient',()=>({getAuthMeSilently:vi.fn(async()=>({subject:'alice'}))}));
vi.mock('../services/httpClient',()=>({request:vi.fn()}));
vi.mock('../services/reportStoreService',()=>({listReports:vi.fn()}));
afterEach(()=>{cleanup();vi.unstubAllGlobals();vi.clearAllMocks();});
test('saves a visibility operation and displays only persisted readback',async()=>{
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({layout:{applications:[]}})}));
 request.mockResolvedValueOnce({preferenceOwnerId:'alice-id',workspace:{enabled:true}}).mockResolvedValueOnce(null).mockResolvedValueOnce({preferenceOwnerId:'alice-id',workspace:{enabled:false}});
 render(<MyWorkspaceSettings/>);
 fireEvent.click(await screen.findByRole('checkbox',{name:'Show My workspace'}));
 await screen.findByText('Workspace saved.');
 expect(screen.getByRole('checkbox',{name:'Show My workspace'}).checked).toBe(false);
 expect(request).toHaveBeenCalledTimes(3);
 expect(JSON.parse(request.mock.calls[1][1].body)).toEqual({expectedOwnerId:'alice-id',workspaceAction:{action:'set_enabled',enabled:false}});
});
test('failed unpin retains existing item and actionable error',async()=>{
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({layout:{applications:[]}})}));
 request.mockResolvedValueOnce({preferenceOwnerId:'alice-id',workspace:{enabled:true,groups:[{id:'weekly',label:'Weekly',items:[{id:'orders',kind:'window',alias:'Orders'}]}]}}).mockRejectedValueOnce(Object.assign(new Error('Internal conflict payload: revision=secret-reference'), {status:409}));
 render(<MyWorkspaceSettings/>);fireEvent.click(await screen.findByRole('button',{name:'Unpin Orders'}));
 await screen.findByRole('alert');expect(screen.getByText('Your workspace changed. Reload it before trying again.')).toBeTruthy();expect(screen.queryByText(/secret-reference/)).toBeNull();
 expect(screen.getByRole('button',{name:'Unpin Orders'})).toBeTruthy();
 await waitFor(()=>expect(screen.getByRole('button',{name:'Unpin Orders'}).disabled).toBe(false));
});

test('session reset clears old user rows and ignores an old preference response',async()=>{
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({layout:{applications:[]}})}));
 let finishOld;
 request.mockImplementationOnce(()=>new Promise(resolve=>{finishOld=resolve;})).mockResolvedValueOnce({preferenceOwnerId:'alice-id',workspace:{groups:[{id:'new',label:'New user',items:[]}]}});
 render(<MyWorkspaceSettings/>);
 window.dispatchEvent(new CustomEvent('agently:session-reset'));
 await screen.findByRole('heading',{name:'New user'});
 finishOld({preferenceOwnerId:'alice-id',workspace:{groups:[{id:'old',label:'Old user',items:[]}]}});
 await waitFor(()=>expect(screen.queryByRole('heading',{name:'Old user'})).toBeNull());
 expect(screen.getByRole('heading',{name:'New user'})).toBeTruthy();
});

test('uses the authorized catalog title for a pin without a personal alias', async () => {
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({layout:{applications:[{id:'example',providerIdentity:'example',menus:[{id:'overview',title:'Overview',action:{type:'report',resourceUri:'report://example/overview'}}]}]}})}));
 request.mockResolvedValueOnce({preferenceOwnerId:'alice-id',workspace:{groups:[{id:'weekly',label:'Weekly',items:[{id:'overview',kind:'report',provider:'example',uri:'report://example/overview'}]}]}});
 render(<MyWorkspaceSettings/>);
 expect(await screen.findByRole('button',{name:'Unpin Overview'})).toBeTruthy();
 expect(screen.getByText('Overview')).toBeTruthy();
});
test('a failed initial read offers retry instead of an indefinite loading message', async () => {
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({layout:{applications:[]}})}));
 request.mockRejectedValueOnce(new Error('Preferences unavailable.')).mockResolvedValueOnce({preferenceOwnerId:'alice-id',workspace:{enabled:true}});
 render(<MyWorkspaceSettings/>);
 fireEvent.click(await screen.findByRole('button',{name:'Retry loading workspace'}));
 expect(await screen.findByRole('checkbox',{name:'Show My workspace'})).toBeTruthy();
 expect(screen.queryByText('Loading your workspace…')).toBeNull();
});

test('an assistant workspace edit refreshes preferences without refetching the catalog', async () => {
 const catalog = vi.fn().mockResolvedValue({ok:true,json:async()=>({layout:{applications:[]}})});
 vi.stubGlobal('fetch',catalog);
 request.mockResolvedValueOnce({preferenceOwnerId:'alice-id',workspace:{groups:[]}}).mockResolvedValueOnce({preferenceOwnerId:'alice-id',workspace:{groups:[{id:'weekly',label:'Weekly checks',items:[]}]}});
 render(<MyWorkspaceSettings/>);
 await screen.findByRole('checkbox',{name:'Show My workspace'});
 window.dispatchEvent(new CustomEvent('agently:workspace-preferences-changed'));
 expect(await screen.findByRole('heading',{name:'Weekly checks'})).toBeTruthy();
 expect(catalog).toHaveBeenCalledTimes(1);
 expect(request).toHaveBeenCalledTimes(2);
});

test('saves the chosen shortcut presentation through native preferences and reads it back',async()=>{
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({layout:{applications:[]}})}));
 request.mockResolvedValueOnce({preferenceOwnerId:'alice-id',workspace:{enabled:true}}).mockResolvedValueOnce(null).mockResolvedValueOnce({preferenceOwnerId:'alice-id',workspace:{enabled:true,presentation:'both'}});
 render(<MyWorkspaceSettings/>);
 fireEvent.change(await screen.findByRole('combobox',{name:'My workspace layout'}),{target:{value:'both'}});
 await screen.findByText('Workspace saved.');
 expect(screen.getByRole('combobox',{name:'My workspace layout'}).value).toBe('both');
 expect(JSON.parse(request.mock.calls[1][1].body)).toEqual({expectedOwnerId:'alice-id',workspaceAction:{action:'set_presentation',presentation:'both'}});
});

test('application visibility saves only in the current browser and restores on remount',async()=>{
 localStorage.clear();
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({workspaceId:'workspace',layout:{applications:[{id:'example',providerIdentity:'provider',title:'Example',menus:[]}]}})}));
 request.mockResolvedValue({preferenceOwnerId:'alice-id',workspace:{}});
 const view=render(<MyWorkspaceSettings initialTab="applications"/>);
 const toggle=await screen.findByRole('checkbox',{name:'Show Example application tab'});
 await act(async()=>{});
 fireEvent.click(toggle);
 await screen.findByText('Visibility saved in this browser.');
 expect(toggle.checked).toBe(false);
 expect(request.mock.calls.every(([,options])=>options?.method!=='PATCH')).toBe(true);
 view.unmount();
 render(<MyWorkspaceSettings initialTab="applications"/>);
 await waitFor(()=>expect(screen.getByRole('checkbox',{name:'Show Example application tab'}).checked).toBe(false));
 localStorage.clear();
});

test('visibility controls wait for current identity and ignore a prior-session identity result',async()=>{
 let resolveIdentity;
 getAuthMeSilently.mockImplementationOnce(()=>new Promise(resolve=>{resolveIdentity=resolve;}));
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({workspaceId:'workspace',layout:{applications:[{id:'example',title:'Example',menus:[]}]}})}));
 request.mockResolvedValue({preferenceOwnerId:'alice-id',workspace:{}});
 render(<MyWorkspaceSettings initialTab="applications"/>);
 expect((await screen.findByRole('checkbox',{name:'Show Example application tab'})).disabled).toBe(true);
 await act(async()=>window.dispatchEvent(new CustomEvent('agently:logout')));
 await act(async()=>resolveIdentity({subject:'old-user'}));
 expect(screen.queryByRole('checkbox',{name:'Show Example application tab'})).toBeNull();
});

test('credential recovery during a pending save does not leave settings locked',async()=>{
 let finishSave;
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({layout:{applications:[]}})}));
 request.mockImplementation((_path,options)=>options?.method==='PATCH'?new Promise(resolve=>{finishSave=resolve;}):Promise.resolve({preferenceOwnerId:'alice-id',workspace:{enabled:true}}));
 render(<MyWorkspaceSettings/>);
 fireEvent.click(await screen.findByRole('checkbox',{name:'Show My workspace'}));
 await waitFor(()=>expect(finishSave).toBeTypeOf('function'));
 await act(async()=>window.dispatchEvent(new CustomEvent('agently:authorized')));
 await waitFor(()=>expect(screen.getByRole('checkbox',{name:'Show My workspace'}).disabled).toBe(false));
 await act(async()=>finishSave(null));
 expect(screen.getByRole('checkbox',{name:'Show My workspace'}).disabled).toBe(false);
});

test('failed save readback preserves the workspace and reports an unknown outcome without raw diagnostics',async()=>{
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({layout:{applications:[]}})}));
 request.mockResolvedValueOnce({preferenceOwnerId:'alice-id',workspace:{enabled:true}}).mockResolvedValueOnce(null).mockRejectedValueOnce(Object.assign(new Error('GET internal schema path=/private/report.yaml'),{status:503}));
 render(<MyWorkspaceSettings/>);
 fireEvent.click(await screen.findByRole('checkbox',{name:'Show My workspace'}));
 expect((await screen.findByRole('alert')).textContent).toBe('Could not confirm the save. Reload your workspace to check before trying again.');
 expect(screen.queryByText(/private\/report/)).toBeNull();
 expect(screen.getByRole('checkbox',{name:'Show My workspace'}).checked).toBe(true);
 expect(screen.queryByText('Workspace saved.')).toBeNull();
});

test('a switch opened by Alice cannot change her preference after Bob signs in',async()=>{
 const app={id:'example',title:'Example',providerIdentity:'publisher',menus:[]};
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({workspaceId:'owner-switch',layout:{applications:[app]}})}));
 request.mockResolvedValue({preferenceOwnerId:'alice-id',workspace:{enabled:true}});
 let subject='alice';
 getAuthMeSilently.mockImplementation(async()=>({subject}));
 render(<MyWorkspaceSettings initialTab="applications"/>);
 const control=await screen.findByRole('checkbox',{name:'Show Example application tab'});
 await waitFor(()=>expect(control.disabled).toBe(false));
 subject='bob';
 fireEvent.click(control);
 await waitFor(()=>expect(getAuthMeSilently).toHaveBeenCalledWith({refresh:true}));
 await waitFor(()=>expect(control.disabled).toBe(false));
 for(const user of ['alice','bob']){
  expect(localStorage.getItem('agently.workspaceVisibility.v1:'+JSON.stringify(['owner-switch',user]))).toBeNull();
 }
});

test('expired session offers sign-in and keeps the selected tab on recovery',async()=>{
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({workspaceId:'example',layout:{applications:[]}})}));
 request.mockResolvedValue({preferenceOwnerId:'alice-id',workspace:{}});
 render(<MyWorkspaceSettings initialTab="applications"/>);
 await screen.findByRole('button',{name:'Applications'});
 act(()=>window.dispatchEvent(new Event('agently:unauthorized')));
 expect(screen.getByRole('button',{name:'Sign in'})).toBeTruthy();
 expect(screen.queryByText('Loading your workspace…')).toBeNull();
 act(()=>window.dispatchEvent(new Event('agently:authorized')));
 await waitFor(()=>expect(screen.queryByRole('button',{name:'Sign in'})).toBeNull());
 expect(screen.getByRole('button',{name:'Applications'}).getAttribute('aria-pressed')).toBe('true');
});

test('keyboard reorder keeps focus with the shortcut at its new position', async () => {
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({layout:{applications:[]}})}));
 const first={id:'first',kind:'report',alias:'First',uri:'report://example/first',provider:'example'};
 const second={...first,id:'second',alias:'Second',uri:'report://example/second'};
 const prefs=items=>({preferenceOwnerId:'alice-id',workspace:{groups:[{id:'weekly',label:'Weekly',items}]}});
 request.mockResolvedValueOnce(prefs([first,second])).mockResolvedValueOnce(null).mockResolvedValueOnce(prefs([second,first]));
 render(<MyWorkspaceSettings/>);
 const move=await screen.findByRole('button',{name:'Move Second up'});
 move.focus();
 fireEvent.click(move,{detail:0});
 await screen.findByText('Workspace saved.');
 expect(screen.getByRole('button',{name:'Move Second up'}).disabled).toBe(true);
 expect(document.activeElement).toBe(screen.getByRole('button',{name:'Move Second down'}));
});
