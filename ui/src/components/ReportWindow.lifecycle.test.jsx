// @vitest-environment jsdom
import React from 'react';
import {afterEach,expect,it,vi} from 'vitest';
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {SettingProvider,activeWindows,addWindow,selectedTabId,selectedWindowId,getViewSignal} from 'forge/core';
import {restoreWindowsFromSnapshot} from 'forge/core/store/signals.js';
import {buildUISnapshot} from 'forge/core/ui/snapshot.js';
import {runUICommand} from 'forge/core/ui/commands.js';
import AppSectionContent from './AppSectionContent.jsx';
import WorkspaceWindowContent from './WorkspaceWindowContent.jsx';
import {waitForWorkspaceReady} from 'forge/core/ui/workspaceReady.js';
vi.mock('forge/components/dashboard/ReportRuntime.jsx',()=>({default:({reportSpec})=><output>{reportSpec.title}</output>}));
afterEach(()=>{cleanup();activeWindows.value=[];selectedTabId.value=null;selectedWindowId.value=null;vi.restoreAllMocks();vi.unstubAllGlobals();});
it.each([false,true])('acknowledges hosted report metadata admission and denial (denied=%s)',async denied=>{
 const pin={uri:'report://example/summary',providerIdentity:'example',kind:'published',contentFingerprint:'fixture',authorityBinding:'fixture'};
 let deliver;
 const services={reportStore:{getReport:vi.fn(()=>new Promise((resolve,reject)=>{deliver=()=>denied?reject(Error('Denied')):resolve({resource:pin,resourceDefinition:{}});} ))}};
 let entry;
 await act(async()=>{entry=addWindow('Example report',null,pin.uri,'',true,{}, {presentation:'hosted',resourceRef:{uri:pin.uri},workspaceObject:{version:1,objectId:'workspace:example',kind:'report',content:{renderer:'forgeWindow'},lifecycle:{state:'opening'}}});});
 const acknowledgement=waitForWorkspaceReady(activeWindows,entry.windowId,1000).then(value=>({value}),error=>({error}));
 render(<SettingProvider services={services}><WorkspaceWindowContent window={entry}/></SettingProvider>);
 await waitFor(()=>expect(deliver).toBeTypeOf('function'));
 expect(activeWindows.peek().find(value=>value.windowId===entry.windowId).workspaceObject.lifecycle.state).toBe('opening');
 await act(async()=>deliver());
 const result=await acknowledgement;
 if(denied){expect(result.error?.message).toBe('Workspace could not be opened');expect(screen.getByText('Report unavailable')).toBeTruthy();}
 else expect(result.value?.lifecycle.state).toBe('ready');
});
it('retains report output and draft filters across unrelated section rerenders',async()=>{
 const pin={uri:'report://example/summary',providerIdentity:'example',kind:'published',contentFingerprint:'fixture',authorityBinding:'fixture'};
 const services={reportExport:{submitResource:vi.fn(),getStatus:vi.fn(),getArtifact:vi.fn()},reportStore:{getReport:vi.fn(async()=>({resource:pin,resourceDefinition:{builderDefinition:{reportBuilder:{predicates:[{id:'range',label:'Period',kind:'dateRange',startParamPath:'filters.from',endParamPath:'filters.to',default:{start:'2026-01-01',end:'2026-01-02'}}]}}}})),runReport:vi.fn(async()=>({resource:pin,reportSpec:{title:'Executed example',datasets:[],blocks:[]},reportFill:{datasets:[],blocks:[]},reportPrint:{}}))}};
 let entry;
 await act(async()=>{entry=addWindow('Example report',null,pin.uri,'',true,{}, {region:'app.section',presentation:'hosted',resourceRef:{uri:pin.uri}});});
 const renderSection=(version)=> <SettingProvider services={services}><AppSectionContent window={{...entry}} title={`Workspace ${version}`}/></SettingProvider>;
 const view=render(renderSection(1));
 const start=await screen.findByLabelText('Period from');
 fireEvent.change(start,{target:{value:'2026-01-02'}});
 fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
 await screen.findByRole('heading',{name:'Executed example'});
 await waitFor(()=>expect(screen.getByRole('button',{name:'Export PDF'}).disabled).toBe(false));
 fireEvent.change(start,{target:{value:'2026-01-03'}});
 view.rerender(renderSection(2));
 expect(screen.getByLabelText('Period from')).toBe(start);
 expect(start.value).toBe('2026-01-03');
 expect(screen.getByRole('heading',{name:'Executed example'})).toBeTruthy();
 await waitFor(()=>expect(screen.getByRole('button',{name:'Export PDF'}).disabled).toBe(false));
 expect(services.reportStore.getReport).toHaveBeenCalledTimes(1);
 expect(services.reportStore.runReport).toHaveBeenCalledTimes(1);
});

it('restores applied and draft filters from a workspace snapshot without rerunning the report',async()=>{
 const pin={uri:'report://example/summary',providerIdentity:'example',kind:'published',contentFingerprint:'fixture',authorityBinding:'fixture'};
 const definition={builderDefinition:{reportBuilder:{predicates:[{id:'region',label:'Region',kind:'text'}]}}};
 const services={reportStore:{getReport:vi.fn(async()=>({resource:pin,resourceDefinition:definition})),
   runReport:vi.fn(async()=>({resource:pin,reportSpec:{title:'Executed example',datasets:[],blocks:[]},reportFill:{datasets:[],blocks:[]},reportPrint:{}}))}};
 const entry=addWindow('Example report',null,pin.uri,'',true,{region:'launch',executeOnOpen:true},
   {conversationId:'example-conversation',presentation:'hosted',resourceRef:{uri:pin.uri}});
 const mount=window=><SettingProvider services={services}><WorkspaceWindowContent window={window}/></SettingProvider>;
 const view=render(mount(entry));
 await screen.findByRole('heading',{name:'Executed example'});
 fireEvent.change(screen.getByLabelText('Region'),{target:{value:'applied'}});
 fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
 await waitFor(()=>expect(services.reportStore.runReport).toHaveBeenCalledTimes(2));
 await screen.findByRole('heading',{name:'Executed example'});
 fireEvent.change(screen.getByLabelText('Region'),{target:{value:'draft'}});
 const read=()=>runUICommand({method:'ui.report.getCurrent',params:{windowId:entry.windowId}});
 expect(await read()).toMatchObject({request:{parameters:{region:'applied'}},draftParameters:{region:'draft'},status:'ready',hasCompletedRun:true,reportSpec:{title:'Executed example'}});
 expect(services.reportStore.runReport).toHaveBeenCalledTimes(2);
 const snapshot=buildUISnapshot();
 expect(snapshot.windows.find(window=>window.windowId===entry.windowId).viewState.report).toMatchObject({parameters:{region:'applied'},draftParameters:{region:'draft'},hasCompletedRun:true});
 view.unmount();

 restoreWindowsFromSnapshot(snapshot);
 const restored=activeWindows.peek().find(window=>window.windowId===entry.windowId);
 render(mount(restored));
 const field=await screen.findByLabelText('Region');
 await waitFor(()=>expect(field.value).toBe('draft'));
 expect(services.reportStore.getReport).toHaveBeenLastCalledWith(expect.objectContaining({windowInstance:{windowId:entry.windowId,conversationId:'example-conversation'}}));
 expect(services.reportStore.runReport).toHaveBeenCalledTimes(2);
 expect(await read()).toMatchObject({request:{parameters:{region:'applied'}},draftParameters:{region:'draft'},status:'idle',hasCompletedRun:false});
 expect(screen.getByRole('button',{name:'Export PDF'}).disabled).toBe(true);
 fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
 await screen.findByRole('heading',{name:'Executed example'});
 expect(services.reportStore.runReport).toHaveBeenCalledTimes(3);
 expect(services.reportStore.runReport.mock.lastCall[0].parameters).toEqual({region:'draft'});
});

it.each([true, false])('keeps applied dates separate from a restored calendar draft (saved dates=%s)', async savedDates => {
 const resource = {uri:'report://example/calendar',providerIdentity:'example',kind:'published',contentFingerprint:'fixture',authorityBinding:'fixture'};
 const datePresets = {
  calendar:{dataSourceRef:'calendar',input:{name:'ids',parameter:'entity',array:true},identityField:'id',timeZoneField:'zone',anchorField:'latestDate'},
  default:'completed',
  options:[{value:'completed',label:'Last week',range:{kind:'relativeDays',anchor:'sourceDate',startOffset:-6,endOffset:0}},{value:'custom',label:'Custom'}],
 };
 const resourceDefinition = {dataSources:{calendar:{cache:{enabled:false}}},builderDefinition:{reportBuilder:{predicates:[
  {id:'period',label:'Period',kind:'dateRange',prefill:{start:'from',end:'to'},datePresets},
 ]}}};
 const services = {reportStore:{getReport:vi.fn(async()=>({resource,resourceDefinition})),runReport:vi.fn()}};
 let resolveCalendar;
 const fetchCalendar = vi.fn(()=>new Promise(resolve=>{resolveCalendar=resolve;}));
 vi.stubGlobal('fetch',fetchCalendar);
 const applied = {entity:17,from:'2026-01-01',to:'2026-01-07'};
 const draft = savedDates ? {entity:17,from:'2026-01-08',to:'2026-01-14'} : {entity:17};
 restoreWindowsFromSnapshot({windows:[{windowId:'calendar-replay',windowKey:resource.uri,parameters:{entity:17,executeOnOpen:true},
  viewState:{report:{resourceUri:resource.uri,parameters:applied,draftParameters:draft}}}]});
 const entry=activeWindows.peek()[0];
 render(<SettingProvider services={services}><WorkspaceWindowContent window={entry}/></SettingProvider>);
 const start=await screen.findByLabelText('Period from');
 const end=screen.getByLabelText('Period to');
 const read=()=>runUICommand({method:'ui.report.getCurrent',params:{windowId:entry.windowId}});
 if(savedDates){
  await waitFor(()=>expect(start.value).toBe(draft.from));
  expect(end.value).toBe(draft.to);
  expect(fetchCalendar).not.toHaveBeenCalled();
 }else{
  await waitFor(()=>expect(fetchCalendar).toHaveBeenCalledTimes(1));
  expect((await read()).request.parameters).toEqual(applied);
  await act(async()=>resolveCalendar({ok:true,json:async()=>({rows:[{id:17,zone:'UTC',latestDate:'2026-02-28'}]})}));
  await waitFor(()=>expect(start.value).toBe('2026-02-22'));
  expect(end.value).toBe('2026-02-28');
 }
 expect(await read()).toMatchObject({request:{parameters:applied},draftParameters:{entity:17,from:start.value,to:end.value},status:'idle'});
 expect(services.reportStore.runReport).not.toHaveBeenCalled();
});

it('discards saved filter state when the mounted report changes its launch scope', async () => {
 const pin = {uri:'report://example/first',providerIdentity:'example',kind:'published',contentFingerprint:'fixture',authorityBinding:'fixture'};
 const resourceDefinition = {builderDefinition:{reportBuilder:{predicates:[{id:'region',label:'Region',kind:'text'}]}}};
 const services = {reportStore:{getReport:vi.fn(async({resource})=>({resource:{...pin,...resource},resourceDefinition})),runReport:vi.fn()}};
 restoreWindowsFromSnapshot({windows:[{windowId:'reused-report',windowKey:pin.uri,parameters:{region:'launch'},
  viewState:{report:{resourceUri:pin.uri,parameters:{region:'applied'},draftParameters:{region:'draft'}}}}]});
 const entry = activeWindows.peek()[0];
 const mount = window => <SettingProvider services={services}><WorkspaceWindowContent window={window}/></SettingProvider>;
 const view = render(mount(entry));
 const read = () => runUICommand({method:'ui.report.getCurrent',params:{windowId:entry.windowId}});
 const assertFilters = async (applied, draft) => {
  await waitFor(()=>expect(screen.getByLabelText('Region').value).toBe(draft));
  expect(await read()).toMatchObject({request:{parameters:{region:applied}},draftParameters:{region:draft},status:'idle'});
 };
 await assertFilters('applied', 'draft');
 view.rerender(mount({...entry,parameters:{region:'changed'}}));
 await assertFilters('changed', 'changed');
 view.rerender(mount(entry));
 await assertFilters('launch', 'launch');
 expect(services.reportStore.getReport).toHaveBeenCalledTimes(1);
 fireEvent.change(screen.getByLabelText('Region'),{target:{value:'new draft'}});

 const second = {uri:'report://example/second'};
 view.rerender(mount({...entry,windowKey:second.uri,resourceRef:second}));
 await assertFilters('launch', 'launch');
 expect(services.reportStore.getReport).toHaveBeenLastCalledWith(expect.objectContaining({resource:second}));
 expect(services.reportStore.runReport).not.toHaveBeenCalled();
});

vi.mock('./Sidebar',()=>({default:()=>null}));
vi.mock('../services/agentlyClient',async original=>({...await original(),getAuthMeSilently:vi.fn(async()=>({subject:'example-user'}))}));
import WorkspaceSidebar from './WorkspaceSidebar.jsx';
it.each([503,429,'network'])('keeps report state through unchanged layout and transient %s refresh failure',async(failure)=>{
 const pin={uri:'report://example/summary',providerIdentity:'example',kind:'published',contentFingerprint:'fixture',authorityBinding:'fixture'};
 const services={reportStore:{getReport:vi.fn(async()=>({resource:pin,resourceDefinition:{builderDefinition:{reportBuilder:{predicates:[{id:'range',label:'Period',kind:'dateRange',startParamPath:'filters.from',endParamPath:'filters.to',default:{start:'2026-01-01',end:'2026-01-02'}}]}}}})),runReport:vi.fn(async()=>({resource:pin,reportSpec:{title:'Executed example',datasets:[],blocks:[]},reportFill:{datasets:[],blocks:[]},reportPrint:{}}))}};
 const payload={workspaceId:'example',layoutRevision:'one',layout:{left:{navigation:{presentation:'sections'},history:{enabled:false}},applications:[{id:'example',title:'Example',menus:[{id:'summary',title:'Summary',action:{type:'report',resourceUri:pin.uri}}]}]}};
 const fetch=vi.fn(async()=>({ok:true,json:async()=>structuredClone(payload)}));vi.stubGlobal('fetch',fetch);
 function Shell(){const [section,setSection]=React.useState(false),[entry,setEntry]=React.useState(null);return <SettingProvider services={services}><WorkspaceSidebar section="workspaces" destination='["example","summary"]' onSectionPresentationChange={setSection} onSectionWindow={value=>setEntry(value)}/>{section&&<AppSectionContent window={entry}/>}</SettingProvider>;}
 render(<Shell/>);
 const start=await screen.findByLabelText('Period from');fireEvent.change(start,{target:{value:'2026-01-02'}});fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
 await screen.findByRole('heading',{name:'Executed example'});await waitFor(()=>expect(screen.getByRole('button',{name:'Export PDF'}).disabled).toBe(false));
 fireEvent.change(start,{target:{value:'2026-01-03'}});
 await act(async()=>window.dispatchEvent(new Event('focus')));
 expect(screen.getByLabelText('Period from')).toBe(start);
 if(failure==='network')fetch.mockRejectedValueOnce(new TypeError('Network unavailable'));
 else fetch.mockResolvedValueOnce({ok:false,status:failure});
 await act(async()=>window.dispatchEvent(new Event('focus')));
 expect(screen.getByLabelText('Period from')).toBe(start);
 expect(start.value).toBe('2026-01-03');
 expect(screen.getByRole('alert')).toBeTruthy();
 await act(async()=>window.dispatchEvent(new Event('focus')));
 expect(screen.getByLabelText('Period from')).toBe(start);
 expect(screen.getByRole('heading',{name:'Executed example'})).toBeTruthy();
 expect(screen.getByRole('button',{name:'Export PDF'}).disabled).toBe(false);
 expect(services.reportStore.getReport).toHaveBeenCalledTimes(1);
 expect(services.reportStore.runReport).toHaveBeenCalledTimes(1);
 expect(screen.queryByRole('alert')).toBeNull();
 const changed=structuredClone(payload);changed.layoutRevision='two';changed.layout.applications[0].menus.push({id:'added',title:'New report',action:{type:'report',resourceUri:'report://example/new'}});
 fetch.mockResolvedValueOnce({ok:true,json:async()=>changed});
 await act(async()=>window.dispatchEvent(new Event('focus')));
 expect(screen.getByRole('button',{name:'New report',exact:true})).toBeTruthy();
 expect(screen.getByLabelText('Period from')).toBe(start);
 expect(screen.getByRole('heading',{name:'Executed example'})).toBeTruthy();
 const revoked=structuredClone(changed);revoked.layoutRevision='three';revoked.layout.applications=[];
 fetch.mockResolvedValueOnce({ok:true,json:async()=>revoked});
 await act(async()=>window.dispatchEvent(new Event('focus')));
 expect(screen.queryByLabelText('Period from')).toBeNull();
});

it('keeps a typed custom range through real resource execution and metadata-stable rerender',async()=>{
 const pin={uri:'report://example/summary',providerIdentity:'example',kind:'published',contentFingerprint:'fixture',authorityBinding:'fixture'};
 const datePresets={calendar:{dataSourceRef:'calendar',input:{name:'ids',parameter:'entity',array:true},identityField:'id',timeZoneField:'zone'},default:'completed',options:[{value:'completed',label:'Completed days',range:{kind:'relativeDays',startOffset:-30,endOffset:-1}},{value:'custom',label:'Custom'}]};
 const definition={dataSources:{calendar:{cache:{enabled:false}}},builderDefinition:{reportBuilder:{predicates:[{id:'period',label:'Period',kind:'dateRange',prefill:{start:'from',end:'to'},datePresets}]}}};
 const services={reportStore:{getReport:vi.fn(async()=>({resource:pin,resourceDefinition:definition})),runReport:vi.fn(async()=>({resource:pin,reportSpec:{title:'Executed custom example',datasets:[],blocks:[]},reportFill:{datasets:[],blocks:[]},reportPrint:{}}))}};
 vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({rows:[{id:17,zone:'UTC'}]})}));
 let entry;
 await act(async()=>{entry=addWindow('Example report',null,pin.uri,'',true,{entity:17},{region:'app.section',presentation:'hosted',resourceRef:{uri:pin.uri}});});
 const view=render(<SettingProvider services={services}><AppSectionContent window={{...entry}}/></SettingProvider>);
 await screen.findByLabelText('Period from');
 await waitFor(()=>expect(screen.getByRole('button',{name:'Apply filters'}).disabled).toBe(false));
 fireEvent.change(screen.getByLabelText('Period from'),{target:{value:'2026-10-01'}});
 fireEvent.change(screen.getByLabelText('Period to'),{target:{value:'2026-11-01'}});
 await waitFor(()=>expect(getViewSignal(entry.windowId).peek().report.draftParameters).toMatchObject({from:'2026-10-01',to:'2026-11-01'}));
 fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
 await waitFor(()=>expect(services.reportStore.runReport).toHaveBeenCalled());
 expect(services.reportStore.runReport.mock.calls.at(-1)[0].parameters).toMatchObject({from:'2026-10-01',to:'2026-11-01'});
 view.rerender(<SettingProvider services={services}><AppSectionContent window={{...entry}}/></SettingProvider>);
 expect(screen.getByLabelText('Period from').value).toBe('2026-10-01');
 expect(screen.getByLabelText('Period to').value).toBe('2026-11-01');
});
