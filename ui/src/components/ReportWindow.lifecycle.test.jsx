// @vitest-environment jsdom
import React from 'react';
import {afterEach,expect,it,vi} from 'vitest';
const transport=vi.hoisted(()=>({current:null}));
vi.mock('forge/core',async original=>({...await original(),getActiveUIBridgeTransportContext:()=>transport.current}));
import {act,cleanup,fireEvent,render,screen,waitFor} from '@testing-library/react';
import {SettingProvider,activeWindows,addWindow,selectedTabId,selectedWindowId,getViewSignal} from 'forge/core';
import {restoreWindowsFromSnapshot} from 'forge/core/store/signals.js';
import {buildUISnapshot,snapshotForRestore} from 'forge/core/ui/snapshot.js';
import {runUICommand} from 'forge/core/ui/commands.js';
import AppSectionContent from './AppSectionContent.jsx';
import WorkspaceWindowContent from './WorkspaceWindowContent.jsx';
import {waitForWorkspaceReady} from 'forge/core/ui/workspaceReady.js';
vi.mock('forge/components/dashboard/ReportRuntime.jsx',()=>({default:({reportSpec})=><output>{reportSpec.title}</output>}));
afterEach(()=>{transport.current=null;cleanup();activeWindows.value=[];selectedTabId.value=null;selectedWindowId.value=null;vi.restoreAllMocks();vi.unstubAllGlobals();});
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

it('readmits a mounted report after credential reconnect without rerunning or replacing its applied and draft range',async()=>{
 const pin={uri:'report://example/summary',providerIdentity:'example',kind:'published',contentFingerprint:'fixture',authorityBinding:'before'};
 const renewed={...pin,authorityBinding:'after'};
 const definition={builderDefinition:{reportBuilder:{predicates:[{id:'period',label:'Period',kind:'dateRange',startParamPath:'filters.from',endParamPath:'filters.to',default:{start:'2026-01-01',end:'2026-01-10'}}]}}};
 const services={reportStore:{getReport:vi.fn().mockResolvedValue({resource:pin,resourceDefinition:definition}),runReport:vi.fn().mockResolvedValue({resource:pin,reportSpec:{title:'Kept output',datasets:[],blocks:[]},reportFill:{datasets:[],blocks:[]}})},reportExport:{submitResource:vi.fn().mockResolvedValue({jobId:'example-job'}),getStatus:vi.fn().mockResolvedValue({status:'failed',error:'Example stop'})}};
 transport.current={clientId:'reader',sessionId:'before'};
 const entry=addWindow('Example report',null,pin.uri,'',true,{}, {region:'app.section',presentation:'hosted',resourceRef:{uri:pin.uri}});
 render(<SettingProvider services={services}><AppSectionContent window={entry}/></SettingProvider>);
 const start=await screen.findByLabelText('Period from');
 fireEvent.change(start,{target:{value:'2026-01-02'}});
 fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
 await screen.findByRole('heading',{name:'Kept output'});
 fireEvent.change(start,{target:{value:'2026-01-03'}});
 const output=screen.getByText('Kept output',{selector:'output'});
 services.reportStore.getReport.mockResolvedValue({resource:renewed,resourceDefinition:definition});
 transport.current={clientId:'reader',sessionId:'after'};
 await act(async()=>window.dispatchEvent(new CustomEvent('forge:retry-window-metadata',{detail:{reason:'transport-ready'}})));
 await waitFor(()=>expect(services.reportStore.getReport).toHaveBeenCalledTimes(2));
 expect(services.reportStore.runReport).toHaveBeenCalledTimes(1);
 expect(screen.getByText('Kept output',{selector:'output'})).toBe(output);
 expect(screen.getByLabelText('Period from')).toBe(start);
 expect(start.value).toBe('2026-01-03');
 expect(getViewSignal(entry.windowId).peek().report.parameters.from).toBe('2026-01-02');
 expect(getViewSignal(entry.windowId).peek().report.draftParameters.from).toBe('2026-01-03');
 fireEvent.click(screen.getByRole('button',{name:'Export PDF'}));
 await waitFor(()=>expect(services.reportExport.submitResource).toHaveBeenCalledTimes(1));
 expect(services.reportExport.submitResource.mock.calls[0][0].resolvedResource).toEqual(renewed);
 expect(services.reportExport.submitResource.mock.calls[0][0].parameters.from).toBe('2026-01-02');
 expect(services.reportStore.runReport).toHaveBeenCalledTimes(1);
});


it('recovers a changed publication with a distinct instance and retains applied/draft filters without running data',async()=>{
 const uri='report://example/summary';
 const old=addWindow('Example',null,uri,null,true,{region:'launch',executeOnOpen:true},{resourceRef:{uri,revision:'review'},presentation:'hosted',region:'app.section',conversationId:'example-chat'});
 getViewSignal(old.windowId).value={report:{resourceUri:uri,parameters:{region:'applied'},draftParameters:{region:'draft'},hasCompletedRun:true,reportFill:{rows:[{private:'old output'}]}}};
 const getReport=vi.fn(async request=>{
  if(request.windowInstance.windowId===old.windowId)throw Object.assign(new Error('Existing instance belongs to another selected candidate'),{status:409});
  return {resource:{uri,kind:'published',contentFingerprint:'current',authorityBinding:'current'},resourceDefinition:{builderDefinition:{reportBuilder:{predicates:[{id:'region',label:'Region',kind:'text'}]}}}};
 });
 const runReport=vi.fn();render(<SettingProvider services={{reportStore:{getReport,runReport}}}><AppSectionContent window={old}/></SettingProvider>);
 await screen.findByText('Report unavailable');
 fireEvent.click(screen.getByRole('button',{name:'Reopen report'}));
 await waitFor(()=>expect(getReport).toHaveBeenCalledTimes(2));
 const next=activeWindows.peek().find(window=>window.windowId!==old.windowId);
 expect(next.windowId).not.toBe(old.windowId);expect(next.parentKey).toBe(old.windowId);
 expect(next.resourceRef).toEqual({uri,revision:'review'});expect(next.conversationId).toBe('example-chat');
 expect(next.parameters).toMatchObject({region:'launch',executeOnOpen:false});
 expect(activeWindows.peek().find(window=>window.windowId===old.windowId)).toBe(old);
 await screen.findByLabelText('Region');expect(screen.getByLabelText('Region').value).toBe('draft');
 expect(getViewSignal(next.windowId).peek().report.parameters.region).toBe('applied');
 expect(JSON.stringify(getViewSignal(next.windowId).peek())).not.toContain('old output');
 expect(runReport).not.toHaveBeenCalled();
});

it.each([403,503])('ordinary %s errors retry the same instance without creating windows',async status=>{
 const entry=addWindow('Example',null,'report://example/summary',null,true,{}, {resourceRef:{uri:'report://example/summary'},presentation:'hosted'});
 const getReport=vi.fn(async()=>{throw Object.assign(new Error('Unavailable'),{status});});
 render(<SettingProvider services={{reportStore:{getReport}}}><WorkspaceWindowContent window={entry}/></SettingProvider>);
 await screen.findByText('Report unavailable');
 expect(screen.queryByRole('button',{name:'Reopen report'})).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'Retry'}));
 await waitFor(()=>expect(getReport).toHaveBeenCalledTimes(2));
 expect(activeWindows.peek()).toHaveLength(1);
 expect(getReport.mock.calls[1][0].windowInstance.windowId).toBe(entry.windowId);
});

it.each(['revision','open-time'])('reloads the exact same draft only after an explicit workspace open %s',async mode=>{
 const uri='report://example/personal';
 const getReport=vi.fn().mockResolvedValueOnce({resource:{uri,kind:'working',contentFingerprint:'before',authorityBinding:'owner'},resourceDefinition:{reportDocument:{title:'Before edit'}}}).mockResolvedValueOnce({resource:{uri,kind:'working',contentFingerprint:'after',authorityBinding:'owner'},resourceDefinition:{reportDocument:{title:'After edit'}}});
 const runReport=vi.fn();
 const entry={windowId:'same-slot',windowKey:uri,resourceRef:{uri,revision:'working'},workspaceObject:{revision:1,lifecycle:{updatedAt:'2026-01-01T00:00:00Z'},lastActivatedBy:{toolName:'ui/view/open',toolCallId:'open-before'}}};
 const services={reportStore:{getReport,runReport}};
 const content=window=><SettingProvider services={services}><WorkspaceWindowContent window={window}/></SettingProvider>;
 const view=render(content(entry));
 await screen.findByRole('heading',{name:'Before edit'});
 view.rerender(content({...entry,windowTitle:'Unrelated layout title',workspaceObject:{...entry.workspaceObject,lastActivatedBy:{toolName:'ui/window/show',toolCallId:'focus-only'}}}));
 expect(getReport).toHaveBeenCalledTimes(1);
 view.rerender(content({...entry,workspaceObject:{revision:mode==='revision'?2:1,lifecycle:{updatedAt:mode==='open-time'?'2026-01-01T00:00:01Z':'2026-01-01T00:00:00Z'}}}));
 await screen.findByRole('heading',{name:'After edit'});
 expect(getReport).toHaveBeenCalledTimes(2);
 expect(getReport.mock.calls[1][0].windowInstance.windowId).toBe('same-slot');
 expect(runReport).not.toHaveBeenCalled();
});


it('retires previous report readiness while an explicit same-URI reopen awaits admission',async()=>{
 const uri='report://example/readiness';
 let admit;
 const getReport=vi.fn().mockResolvedValueOnce({resource:{uri,kind:'working',contentFingerprint:'old',authorityBinding:'owner'},resourceDefinition:{reportDocument:{title:'Old title'}}}).mockImplementationOnce(()=>new Promise(resolve=>{admit=resolve;}));
 const services={reportStore:{getReport,runReport:vi.fn()}};
 const first=addWindow('Old title',null,uri,null,true,{}, {windowId:'readiness-slot',resourceRef:{uri,revision:'working'},workspaceObject:{revision:1,lifecycle:{state:'ready',updatedAt:'2026-01-01T00:00:00Z'}}});
 const content=entry=><SettingProvider services={services}><WorkspaceWindowContent window={entry}/></SettingProvider>;
 const view=render(content(first));
 await screen.findByRole('heading',{name:'Old title'});
 getViewSignal(first.windowId).value={report:{resourceUri:uri,status:'ready',hasCompletedRun:true,title:'Old title'}};
 const reopened={...activeWindows.peek().find(entry=>entry.windowId===first.windowId),workspaceObject:{...first.workspaceObject,lifecycle:{state:'ready',updatedAt:'2026-01-01T00:00:01Z'}}};
 activeWindows.value=[reopened];
 view.rerender(content(reopened));
 await waitFor(()=>expect(getReport).toHaveBeenCalledTimes(2));
 expect(getViewSignal(first.windowId).peek().report).toMatchObject({status:'opening',hasCompletedRun:false});
 expect(getViewSignal(first.windowId).peek().report.title).toBeUndefined();
 const pending=buildUISnapshot().windows.find(entry=>entry.windowId===first.windowId);
 expect(pending.workspaceObject.lifecycle.state).toBe('opening');
 expect(screen.queryByRole('heading',{name:'Old title'})).toBeNull();
 await act(async()=>admit({resource:{uri,kind:'working',contentFingerprint:'new',authorityBinding:'owner'},resourceDefinition:{reportDocument:{title:'New title'}}}));
 await screen.findByRole('heading',{name:'New title'});
 expect(activeWindows.peek().find(entry=>entry.windowId===first.windowId).workspaceObject.lifecycle.state).toBe('ready');
 expect(services.reportStore.runReport).not.toHaveBeenCalled();
});

it.each([false,true,'keep'])('waits for native reopen admission and reconciles changed predicates (%s)',async changedPredicates=>{
 const uri='report://example/native-reopen';
 let admit;
 const definition=title=>({reportDocument:{title},builderDefinition:{reportBuilder:{predicates:title==='New report'&&changedPredicates?[{id:'newRegion',label:'New region',kind:'text',default:'new authored default'}]:[{id:'region',label:'Region',kind:'text'}]}}});
 const getReport=vi.fn().mockResolvedValueOnce({resource:{uri,kind:'working',contentFingerprint:'old',authorityBinding:'owner'},resourceDefinition:definition('Old report')}).mockImplementationOnce(()=>new Promise(resolve=>{admit=resolve;}));
 const services={reportStore:{getReport,runReport:vi.fn()}};
 const descriptor={version:1,revision:1,objectId:'workspace:native-slot',kind:'report',content:{renderer:'forgeWindow',windowId:'native-slot',windowKey:uri},lifecycle:{state:'opening',updatedAt:'2026-01-01T00:00:00Z'}};
 const entry=addWindow('Original',null,uri,null,true,{region:'launch',executeOnOpen:false}, {windowId:'native-slot',region:'app.section',presentation:'hosted',resourceRef:{uri,revision:'working'},workspaceObject:descriptor});
 render(<SettingProvider services={services}><AppSectionContent window={entry}/></SettingProvider>);
 await screen.findByRole('heading',{name:'Old report'});
 fireEvent.change(screen.getByLabelText('Region'),{target:{value:'unsaved draft'}});
 await waitFor(()=>expect(getViewSignal(entry.windowId).peek().report.draftParameters.region).toBe('unsaved draft'));
 getViewSignal(entry.windowId).value={report:{...getViewSignal(entry.windowId).peek().report,parameters:{region:'applied choice'},status:'ready',hasCompletedRun:true,title:'Old report'}};
 const before=buildUISnapshot().windows.find(value=>value.windowId===entry.windowId);
 let acknowledged=false,opening;
 await act(async()=>{
  opening=runUICommand({method:'ui.window.open',params:{windowId:entry.windowId,windowKey:uri,windowTitle:'New report',parameters:{executeOnOpen:false,region:'launch'},expectedRevision:before.operationRevision,options:{parentKey:null,region:'app.section',presentation:'hosted',resourceRef:{uri,revision:'working'},workspaceObject:{...descriptor,lifecycle:{state:'opening',updatedAt:'2026-01-01T00:00:01Z'}}}}}).then(value=>{acknowledged=true;return value;});
 });
 await waitFor(()=>expect(getReport).toHaveBeenCalledTimes(2));
 expect(acknowledged).toBe(false);
 expect(buildUISnapshot().windows.find(value=>value.windowId===entry.windowId).workspaceObject.lifecycle.state).toBe('opening');
 expect(getViewSignal(entry.windowId).peek().report.hasCompletedRun).toBe(false);
 expect(screen.queryByRole('heading',{name:'Old report'})).toBeNull();
 await act(async()=>admit({resource:{uri,kind:'working',contentFingerprint:'new',authorityBinding:'owner'},resourceDefinition:definition('New report')}));
 if(changedPredicates){
  const choice=await screen.findByRole('checkbox',{name:'region'});
  expect(screen.queryByRole('heading',{name:'New report'})).toBeNull();
  expect(services.reportStore.runReport).not.toHaveBeenCalled();
  if(changedPredicates==='keep')fireEvent.click(choice);
  fireEvent.click(screen.getByRole('button',{name:'Continue'}));
 }
 await screen.findByRole('heading',{name:'New report'});
 expect((await opening).workspaceObject.lifecycle.state).toBe('ready');
 const current=await runUICommand({method:'ui.report.getCurrent',params:{windowId:entry.windowId}});
 if(changedPredicates){
  expect(screen.queryByLabelText('Region')).toBeNull();
  expect(screen.getByLabelText('New region').value).toBe('new authored default');
  expect(current.request.parameters.newRegion).toBe('new authored default');
  expect(JSON.stringify(current)).not.toContain('unsaved draft');
  if(changedPredicates==='keep')expect(current.request.parameters.region).toBe('launch');
  else {
   expect(current.request.parameters).not.toHaveProperty('region');
   expect(buildUISnapshot().windows.find(row=>row.windowId===entry.windowId).parameters).not.toHaveProperty('region');
  }
 }else{
  expect(screen.getByLabelText('Region').value).toBe('unsaved draft');
  expect(current.request.parameters.region).toBe('applied choice');
 }
 const admitted=activeWindows.peek().find(value=>value.windowId===entry.windowId);
 await act(async()=>{await runUICommand({method:'ui.window.activate',params:{windowId:entry.windowId,workspaceObject:{...admitted.workspaceObject,lastActivatedBy:{toolName:'ui/window/show',toolCallId:'focus-only'}}}});});
 expect(getReport).toHaveBeenCalledTimes(2);
 expect(screen.getByRole('heading',{name:'New report'})).toBeTruthy();
 expect(activeWindows.peek()).toHaveLength(1);
 expect(services.reportStore.runReport).not.toHaveBeenCalled();
});

it.each([false,true,'same-id'])('reconciles working report filters after snapshot restore (changed=%s)',async changed=>{
 const uri='report://users/example/editable';
 let definition={builderDefinition:{reportBuilder:{predicates:[{id:'region',label:'Region',kind:'text',default:'initial'}]}}};
 const services={reportStore:{getReport:vi.fn(async()=>({resource:{uri,kind:'working',contentFingerprint:changed?'second':'first'},resourceDefinition:definition})),runReport:vi.fn()}};
 const entry=addWindow('Editable report',null,uri,'',true,{}, {conversationId:'editable-chat',presentation:'hosted',resourceRef:{uri}});
 const mount=window=><SettingProvider services={services}><WorkspaceWindowContent window={window}/></SettingProvider>;
 const view=render(mount(entry));
 fireEvent.change(await screen.findByLabelText('Region'),{target:{value:'unsaved draft'}});
 await waitFor(()=>expect(getViewSignal(entry.windowId).peek().report.draftParameters.region).toBe('unsaved draft'));
 const snapshot=JSON.parse(JSON.stringify(buildUISnapshot()));view.unmount();
 if(changed)definition={builderDefinition:{reportBuilder:{predicates:[{id:changed==='same-id'?'region':'newRegion',label:changed==='same-id'?'Region':'New region',kind:'text',default:'new default'}]}}};
 restoreWindowsFromSnapshot(snapshot);
 render(mount(activeWindows.peek().find(item=>item.windowId===entry.windowId)));
 const field=await screen.findByLabelText(changed===true?'New region':'Region');
 await waitFor(()=>expect(field.value).toBe(changed?'new default':'unsaved draft'));
 const current=await runUICommand({method:'ui.report.getCurrent',params:{windowId:entry.windowId}});
 if(changed===true){expect(current.draftParameters.region).toBeUndefined();expect(current.request.parameters.region).toBeUndefined();}
 if(changed==='same-id'){expect(current.draftParameters.region).toBe('new default');expect(current.request.parameters.region).toBe('new default');}
 if(!changed)expect(current.request.parameters.region).toBe('initial');
 expect(services.reportStore.runReport).not.toHaveBeenCalled();
 expect(services.reportStore.getReport).toHaveBeenCalledTimes(2);
});

it('preserves a working draft without WebCrypto when JSON object keys are reordered',async()=>{
 vi.stubGlobal('crypto',undefined);
 const changed=false;
 const uri='report://users/example/editable';
 let definition={builderDefinition:{reportBuilder:{predicates:[{id:'region',label:'Region',kind:'text',default:'initial'}]}}};
 const services={reportStore:{getReport:vi.fn(async()=>({resource:{uri,kind:'working',contentFingerprint:changed?'second':'first'},resourceDefinition:definition})),runReport:vi.fn()}};
 const entry=addWindow('Editable report',null,uri,'',true,{}, {conversationId:'editable-chat',presentation:'hosted',resourceRef:{uri}});
 const mount=window=><SettingProvider services={services}><WorkspaceWindowContent window={window}/></SettingProvider>;
 const view=render(mount(entry));
 fireEvent.change(await screen.findByLabelText('Region'),{target:{value:'unsaved draft'}});
 await waitFor(()=>expect(getViewSignal(entry.windowId).peek().report.draftParameters.region).toBe('unsaved draft'));
 const snapshot=JSON.parse(JSON.stringify(buildUISnapshot()));view.unmount();
 definition={builderDefinition:{reportBuilder:{predicates:[{default:'initial',kind:'text',label:'Region',id:'region'}]}}};
 restoreWindowsFromSnapshot(snapshot);
 render(mount(activeWindows.peek().find(item=>item.windowId===entry.windowId)));
 const field=await screen.findByLabelText(changed===true?'New region':'Region');
 await waitFor(()=>expect(field.value).toBe(changed?'new default':'unsaved draft'));
 const current=await runUICommand({method:'ui.report.getCurrent',params:{windowId:entry.windowId}});
 if(changed===true){expect(current.draftParameters.region).toBeUndefined();expect(current.request.parameters.region).toBeUndefined();}
 if(changed==='same-id'){expect(current.draftParameters.region).toBe('new default');expect(current.request.parameters.region).toBe('new default');}
 if(!changed)expect(current.request.parameters.region).toBe('initial');
 expect(services.reportStore.runReport).not.toHaveBeenCalled();
 expect(services.reportStore.getReport).toHaveBeenCalledTimes(2);
});

it.each(['working','published'])('preserves date drafts through serialized snapshots and pending admission (%s)',async kind=>{
 const uri='report://example/date-restore';
 const definition={builderDefinition:{reportBuilder:{predicates:[{id:'period',label:'Period',kind:'dateRange',startParamPath:'filters.from',endParamPath:'filters.to',default:{start:'2026-01-01',end:'2026-01-07'}}]}}};
 const report={resource:{uri,kind,contentFingerprint:'stable'},resourceDefinition:definition};
 let finish;
 const getReport=vi.fn().mockResolvedValueOnce(report).mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;})).mockResolvedValue(report);
 const services={reportStore:{getReport,runReport:vi.fn()}};
 const entry=addWindow('Dates',null,uri,'',true,{}, {conversationId:'date-replay',presentation:'hosted',resourceRef:{uri}});
 const mount=window=><SettingProvider services={services}><WorkspaceWindowContent window={window}/></SettingProvider>;
 const view=render(mount(entry));
 fireEvent.change(await screen.findByLabelText('Period from'),{target:{value:'2026-01-03'}});
 await waitFor(()=>expect(getViewSignal(entry.windowId).peek().report.draftParameters.from).toBe('2026-01-03'));
 let snapshot=JSON.parse(JSON.stringify(buildUISnapshot()));view.unmount();
 restoreWindowsFromSnapshot(snapshot);
 const pending=render(mount(activeWindows.peek().find(item=>item.windowId===entry.windowId)));
 await waitFor(()=>expect(finish).toBeTypeOf('function'));
 snapshot=JSON.parse(JSON.stringify(buildUISnapshot()));pending.unmount();
 restoreWindowsFromSnapshot(snapshot);
 render(mount(activeWindows.peek().find(item=>item.windowId===entry.windowId)));
 const field=await screen.findByLabelText('Period from');
 await waitFor(()=>expect(field.value).toBe('2026-01-03'));
 const current=await runUICommand({method:'ui.report.getCurrent',params:{windowId:entry.windowId}});
 expect(current.request.parameters.from).toBe('2026-01-01');
 expect(current).not.toHaveProperty('filterDeclaration');
 expect(services.reportStore.runReport).not.toHaveBeenCalled();
 await act(async()=>finish(report));
 expect(field.value).toBe('2026-01-03');
});

it('retains a working input draft through the actual snapshot size limits',async()=>{
 const uri='report://users/example/many-options';
 const applied=Array.from({length:120},(_,i)=>i+1);
 const draft=Array.from({length:137},(_,i)=>i+501);
 const report={resource:{uri,kind:'working',contentFingerprint:'same'},resourceDefinition:{builderDefinition:{reportBuilder:{predicates:[{id:'selection',label:'Selection',kind:'value',multiple:true,default:applied,description:'Example declaration '.repeat(1500)}]}}}};
 const services={reportStore:{getReport:vi.fn(async()=>report),runReport:vi.fn()}};
 const entry=addWindow('Options',null,uri,'',true,{}, {conversationId:'options-chat',presentation:'hosted',resourceRef:{uri}});
 const mount=window=><SettingProvider services={services}><WorkspaceWindowContent window={window}/></SettingProvider>;
 const view=render(mount(entry));
 fireEvent.change(await screen.findByLabelText('Selection'),{target:{value:draft.join(',')}});
 await waitFor(()=>expect(getViewSignal(entry.windowId).peek().report.draftParameters.selection).toEqual(draft));
 const snapshot=JSON.parse(JSON.stringify(snapshotForRestore(buildUISnapshot({includeCollection:true,includeInlineMetadata:true,includeFullWindowForm:true}))));
 const saved=snapshot.windows.find(item=>item.windowId===entry.windowId).viewState.report;
 expect(saved.filterFingerprint).toMatch(/^[a-f0-9]{64}$/);expect(saved).not.toHaveProperty('filterDeclaration');
 view.unmount();restoreWindowsFromSnapshot(snapshot);
 render(mount(activeWindows.peek().find(item=>item.windowId===entry.windowId)));
 const field=await screen.findByLabelText('Selection');
 await waitFor(()=>expect(field.value.split(',').map(Number)).toEqual(draft));
 expect(getViewSignal(entry.windowId).peek().report.parameters.selection).toEqual(applied);
 expect(services.reportStore.runReport).not.toHaveBeenCalled();
});

it.each([true,false])('reviews additional inputs on first open of a personal shortcut (keep=%s)',async keep=>{
 const {openWindow}=await import('./menuBarHelpers');
 const {resolveLayoutReportOptions}=await import('./WorkspaceSidebar');
 const {PERSONAL_WORKSPACE_ID}=await import('../services/personalWorkspace');
 const uri='report://example/first-shortcut-open';
 const definition={reportDocument:{title:'First shortcut open'},builderDefinition:{reportBuilder:{predicates:[{id:'period',label:'Period',kind:'text'}]}}};
 const services={reportStore:{getReport:vi.fn(async()=>({resource:{uri,kind:'working',providerIdentity:'example',authorityBinding:'owner'},resourceDefinition:definition})),runReport:vi.fn()}};
 const action={type:'report',providerIdentity:'example',resourceUri:uri,parameters:{formerRegion:'west',period:'monthly',executeOnOpen:false}};
 const options=resolveLayoutReportOptions(action,'',{},PERSONAL_WORKSPACE_ID);
 const entry=openWindow(uri,'Monthly',[],options);
 render(<SettingProvider services={services}><WorkspaceWindowContent window={entry}/></SettingProvider>);
 const choice=await screen.findByRole('checkbox',{name:'formerRegion'});
 expect(screen.queryByRole('checkbox',{name:'period'})).toBeNull();
 expect(services.reportStore.runReport).not.toHaveBeenCalled();
 if(keep)fireEvent.click(choice);
 fireEvent.click(screen.getByRole('button',{name:'Continue'}));
 await screen.findByRole('heading',{name:'First shortcut open'});
 await waitFor(()=>expect(getViewSignal(entry.windowId).peek().report.parameters.period).toBe('monthly'));
 const current=getViewSignal(entry.windowId).peek().report;
 expect(current.parameters.formerRegion).toBe(keep?'west':undefined);
 expect(current.shortcutInputsPending).toBeUndefined();
 expect(services.reportStore.runReport).not.toHaveBeenCalled();
});
