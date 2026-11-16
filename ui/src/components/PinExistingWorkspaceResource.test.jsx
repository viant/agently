// @vitest-environment jsdom
import React from 'react';
import {render,screen,fireEvent,waitFor,cleanup} from '@testing-library/react';
import {test,expect,vi,afterEach} from 'vitest';
import PinExistingWorkspaceResource from './PinExistingWorkspaceResource';
import {listReports} from '../services/reportStoreService';
vi.mock('forge/components',()=>({Icon:()=>null,SelectControl:props=><select {...props}/>}));
vi.mock('../services/reportStoreService',()=>({listReports:vi.fn()}));
afterEach(()=>{cleanup();vi.clearAllMocks();});
const applications=[{id:'example',title:'Example',providerIdentity:'example-provider',menus:[{id:'orders',title:'Orders',action:{type:'window',resourceUri:'window://example/orders',parameters:{section:'open'}}}]}];
const groups=[{id:'weekly',label:'Weekly checks'}];
const report={title:'Overview',resourceUri:'report://example/overview',namespace:'example',ownedByCurrentUser:false,resource:{providerIdentity:'report-provider'}};

test('pins an existing window to the chosen group with its declared parameters',async()=>{
 const onPin=vi.fn().mockResolvedValue(true),onCancel=vi.fn();
 render(<PinExistingWorkspaceResource applications={applications} groups={groups} saving={false} onPin={onPin} onCancel={onCancel}/>);
 fireEvent.click(screen.getByRole('button',{name:/Orders/}));
 fireEvent.change(screen.getByRole('combobox',{name:'Pin group'}),{target:{value:'__new__'}});
 fireEvent.change(screen.getByRole('textbox',{name:'New group name'}),{target:{value:'Daily checks'}});
 fireEvent.click(screen.getByRole('button',{name:'Pin to My workspace'}));
 await waitFor(()=>expect(onCancel).toHaveBeenCalledTimes(1));
 expect(onPin).toHaveBeenCalledWith(expect.objectContaining({action:'pin',groupId:expect.any(String),label:'Daily checks',pin:expect.objectContaining({id:expect.any(String),kind:'window',provider:'example-provider',uri:'window://example/orders',parameters:{section:'open'}})}));
 expect(listReports).not.toHaveBeenCalled();
});

test('pins the original report identity without cloning or loading its definition',async()=>{
 listReports.mockResolvedValueOnce({reports:[report]});
 const onPin=vi.fn().mockResolvedValue(true);
 render(<PinExistingWorkspaceResource applications={applications} groups={groups} onPin={onPin} onCancel={()=>{}}/>);
 fireEvent.change(screen.getByRole('combobox',{name:'Resource type'}),{target:{value:'report'}});
 fireEvent.click(await screen.findByRole('button',{name:/Overview.*Original/}));
 fireEvent.change(screen.getByRole('textbox',{name:'Menu label'}),{target:{value:'Original overview'}});
 fireEvent.click(screen.getByRole('button',{name:'Pin to My workspace'}));
 await waitFor(()=>expect(onPin).toHaveBeenCalledTimes(1));
 expect(onPin.mock.calls[0][0]).toMatchObject({action:'pin',groupId:'weekly',pin:{kind:'report',provider:'report-provider',uri:report.resourceUri,alias:'Original overview',parameters:{}}});
 expect(listReports).toHaveBeenCalledTimes(1);
 expect(listReports).toHaveBeenCalledWith({limit:25});
});

test('a failed pin retains selection and its stable retry identity',async()=>{
 const onPin=vi.fn().mockResolvedValue(false),onCancel=vi.fn();
 render(<PinExistingWorkspaceResource applications={applications} groups={groups} onPin={onPin} onCancel={onCancel}/>);
 fireEvent.click(screen.getByRole('button',{name:/Orders/}));
 fireEvent.click(screen.getByRole('button',{name:'Pin to My workspace'}));
 await waitFor(()=>expect(onPin).toHaveBeenCalledTimes(1));
 fireEvent.click(screen.getByRole('button',{name:'Pin to My workspace'}));
 await waitFor(()=>expect(onPin).toHaveBeenCalledTimes(2));
 expect(onPin.mock.calls[0][0].pin.id).toBe(onPin.mock.calls[1][0].pin.id);
 expect(onCancel).not.toHaveBeenCalled();
 expect(screen.getByText('Selected: Orders')).toBeTruthy();
});

test('shows a personal report name while keeping its UUID reference internal',async()=>{
 const uri='report://users/6de53188-9d88-48cf-b2a9-fd5911b14090/copies/ed573a95-6cb1-40b5-b2c7-af54c5058cc3';
 listReports.mockResolvedValueOnce({reports:[{...report,title:'My weekly performance',namespace:'users',resourceUri:uri}]});
 const onPin=vi.fn().mockResolvedValue(true);
 const {container}=render(<PinExistingWorkspaceResource applications={applications} groups={groups} onPin={onPin} onCancel={()=>{}}/>);
 fireEvent.change(screen.getByRole('combobox',{name:'Resource type'}),{target:{value:'report'}});
 fireEvent.click(await screen.findByRole('button',{name:/My weekly performance.*Personal/}));
 expect(screen.getByRole('textbox',{name:'Menu label'}).value).toBe('My weekly performance');
 expect(container.textContent).not.toContain(uri);
 expect(container.textContent).not.toContain('6de53188-9d88-48cf-b2a9-fd5911b14090');
 fireEvent.click(screen.getByRole('button',{name:'Pin to My workspace'}));
 await waitFor(()=>expect(onPin).toHaveBeenCalledTimes(1));
 expect(onPin.mock.calls[0][0].pin.uri).toBe(uri);
 expect(onPin.mock.calls[0][0].pin.alias).toBe('My weekly performance');
});

test('loads further report pages only on request and retries a failed catalog read',async()=>{
 listReports.mockRejectedValueOnce(new Error('Reports unavailable.')).mockResolvedValueOnce({reports:[report],nextCursor:'opaque-next'}).mockResolvedValueOnce({reports:[{...report,title:'Weekly',resourceUri:'report://example/weekly'}]});
 render(<PinExistingWorkspaceResource applications={applications} groups={groups} onPin={()=>{}} onCancel={()=>{}}/>);
 fireEvent.change(screen.getByRole('combobox',{name:'Resource type'}),{target:{value:'report'}});
 fireEvent.click(await screen.findByRole('button',{name:'Retry loading reports'}));
 await screen.findByRole('button',{name:/Overview/});
 expect(listReports).toHaveBeenCalledTimes(2);
 fireEvent.click(screen.getByRole('button',{name:'Load more reports'}));
 await waitFor(()=>expect(listReports).toHaveBeenCalledTimes(3));
 expect(listReports.mock.calls[2][0]).toEqual({limit:25,cursor:'opaque-next'});
});

test('switching away from a pending report page clears loading and ignores its late response',async()=>{
 let finish;
 listReports.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
 render(<PinExistingWorkspaceResource applications={applications} groups={groups} onPin={()=>{}} onCancel={()=>{}}/>);
 fireEvent.change(screen.getByRole('combobox',{name:'Resource type'}),{target:{value:'report'}});
 expect(screen.getByText('Loading reports…')).toBeTruthy();
 fireEvent.change(screen.getByRole('combobox',{name:'Resource type'}),{target:{value:'window'}});
 expect(screen.queryByText('Loading reports…')).toBeNull();
 finish({reports:[report]});
 await waitFor(()=>expect(screen.getByRole('button',{name:/Orders/})).toBeTruthy());
 expect(screen.queryByRole('button',{name:/Overview/})).toBeNull();
});

test('catalog failures show recovery copy without exposing transport diagnostics',async()=>{
 listReports.mockRejectedValueOnce(Object.assign(new Error('Internal schema validation: report://users/private-id invalid field'),{status:500}));
 render(<PinExistingWorkspaceResource applications={applications} groups={groups} onPin={()=>{}} onCancel={()=>{}}/>);
 fireEvent.change(screen.getByRole('combobox',{name:'Resource type'}),{target:{value:'report'}});
 expect((await screen.findByRole('alert')).textContent).toContain('Reports could not be loaded. Try again.');
 expect(screen.queryByText(/private-id/)).toBeNull();
 expect(screen.getByRole('button',{name:'Retry loading reports'})).toBeTruthy();
});

test('retrying a later report page keeps earlier pages and the selected shortcut',async()=>{
 const second={...report,title:'Weekly review',resourceUri:'report://example/weekly'};
 const third={...report,title:'Monthly review',resourceUri:'report://example/monthly'};
 listReports.mockResolvedValueOnce({reports:[report],nextCursor:'page-two'})
   .mockResolvedValueOnce({reports:[second],nextCursor:'page-three'})
   .mockRejectedValueOnce(new Error('connection interrupted'))
   .mockResolvedValueOnce({reports:[third]});
 const onPin=vi.fn().mockResolvedValue(true);
 render(<PinExistingWorkspaceResource applications={applications} groups={groups} onPin={onPin} onCancel={()=>{}}/>);
 fireEvent.change(screen.getByRole('combobox',{name:'Resource type'}),{target:{value:'report'}});
 fireEvent.click(await screen.findByRole('button',{name:'Load more reports'}));
 fireEvent.click(await screen.findByRole('button',{name:/Weekly review.*Original/}));
 fireEvent.click(screen.getByRole('button',{name:'Load more reports'}));
 fireEvent.click(await screen.findByRole('button',{name:'Retry loading reports'}));
 await screen.findByRole('button',{name:/Monthly review.*Original/});
 expect(listReports.mock.calls.map(([input])=>input.cursor)).toEqual([undefined,'page-two','page-three','page-three']);
 expect(screen.getByRole('button',{name:/Weekly review.*Original/}).getAttribute('aria-pressed')).toBe('true');
 expect(screen.getByRole('button',{name:/Overview.*Original/})).toBeTruthy();
 fireEvent.click(screen.getByRole('button',{name:'Pin to My workspace'}));
 expect(onPin.mock.calls[0][0].pin.uri).toBe(second.resourceUri);
});

test('search covers later reports and keeps the query with its cursor', async () => {
 const later = {...report, title:'Later report', resourceUri:'report://example/later'};
 listReports.mockResolvedValueOnce({reports:[report],nextCursor:'browse-next'})
   .mockResolvedValueOnce({reports:[later],nextCursor:'search-next'})
   .mockResolvedValueOnce({reports:[]});
 render(<PinExistingWorkspaceResource applications={applications} groups={groups} onPin={()=>{}} onCancel={()=>{}}/>);
 fireEvent.change(screen.getByRole('combobox',{name:'Resource type'}),{target:{value:'report'}});
 await screen.findByRole('button',{name:/Overview/});
 const input = screen.getByRole('textbox',{name:'Find a report'});
 fireEvent.change(input,{target:{value:'Lat'}});
 fireEvent.change(input,{target:{value:'Later'}});
 await screen.findByRole('button',{name:/Later report/});
 expect(screen.queryByRole('button',{name:/Overview/})).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'Load more reports'}));
 await waitFor(()=>expect(listReports.mock.calls.map(([request])=>request)).toEqual([
   {limit:25}, {limit:25,search:'Later'}, {limit:25,search:'Later',cursor:'search-next'},
 ]));
});

test('keeps healthy reports selectable beside an unavailable personal report',async()=>{
 listReports.mockResolvedValueOnce({reports:[{title:'Weekly copy',resourceUri:'report://users/example/copy',namespace:'users',unavailable:true}, report]});
 render(<PinExistingWorkspaceResource applications={applications} groups={groups} onPin={vi.fn()} onCancel={()=>{}}/>);
 fireEvent.change(screen.getByRole('combobox',{name:'Resource type'}),{target:{value:'report'}});
 expect((await screen.findByRole('button',{name:/Weekly copy.*Unavailable/})).disabled).toBe(true);
 expect(screen.getByRole('button',{name:new RegExp(report.title)}).disabled).toBe(false);
});
