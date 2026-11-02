// @vitest-environment jsdom
import {afterEach,expect,it,vi} from 'vitest';
import {createPublishedWindowBindingRecovery} from './publishedWindowBindingRecovery.js';
import {createDataConnector} from 'forge/hooks/dataconnector.js';
import {consumeDataSourceFetchRequest} from 'forge/components/dataSourceFetchState.js';
import {registerControlActions,invalidateControlActionState} from 'forge/core/ui/controlActions.js';
import {useDataSourceHandlers} from 'forge/hooks/dataSource.js';
import {refreshWorkspace,registerWorkspaceRefresh} from 'forge/core/ui/workspaceRefresh.js';
import {activeWindows,getMetadataSignal,getFormSignal,getInputSignal,getSelectionSignal,getCollectionSignal,getControlSignal,getFormStatusSignal,getCollectionInfoSignal,removeSignalsForKey,primeWindowSignals} from 'forge/core/store/signals.js';

const windowId='neutral-readmission';
let unregister;const controls=[];
afterEach(()=>{unregister?.();unregister=null;for(const off of controls.splice(0))off();activeWindows.value=[];removeSignalsForKey(windowId);vi.restoreAllMocks();vi.unstubAllGlobals();});
function fixture(){
 vi.spyOn(console,'error').mockImplementation(()=>{});
 const mounted={windowId,windowKey:'window://fixture/records',providerIdentity:'fixture',resourceRef:{uri:'window://fixture/records',revision:'1'},parameters:{recordId:7}};
 activeWindows.value=[mounted];
 const metadata={dataSource:{rows:{},background:{}},view:{content:{id:'records'}}};getMetadataSignal(windowId).value=metadata;
 const sourceId=`${windowId}DSbackground`;
 const signals={form:getFormSignal(sourceId),input:getInputSignal(sourceId),selection:getSelectionSignal(sourceId),collection:getCollectionSignal(sourceId),control:getControlSignal(sourceId),formStatus:getFormStatusSignal(sourceId),collectionInfo:getCollectionInfoSignal(sourceId)};
 signals.form.value={name:'retained draft'};signals.control.value={loading:true};
 signals.input.value={filter:{query:'retained'},page:2,parameters:{recordId:7},fetch:true,refresh:false};
 const capability=()=>registerControlActions({windowId,controlId:'read-status'},()=>({visible:true,disabled:signals.control.peek().loading,actions:{click(){}}}));
 const off=capability();controls.push(off);
 const handlers=useDataSourceHandlers({windowId,dataSourceId:sourceId,dataSourceRef:'background'},signals,metadata.dataSource,{});
 let resolveMetadata,metadataStarted;
 const gate=new Promise(resolve=>{resolveMetadata=resolve;}),started=new Promise(resolve=>{metadataStarted=resolve;});
 let admitted=false,dataRequests=0,metadataRequests=0;
 const fetch=vi.fn(async url=>{
  if(String(url).includes('/agently/forge/window/')){metadataRequests++;metadataStarted();await gate;admitted=true;return Response.json({data:metadata});}
  dataRequests++;
  return admitted?Response.json({data:[{id:7}]}):Response.json({code:'window_binding_required',scope:'window'},{status:403});
 });
 vi.stubGlobal('fetch',fetch);
 const prepareRequest=request=>({headers:{...request.headers,'Mcp-Session-Id':'session-a'}});
 const metadataConnector=createDataConnector({service:{URL:'/v1/api/agently/forge/window/records',method:'GET'}},{prepareRequest,windowId});
 const commit=vi.fn(()=>{const fresh={...metadata};getMetadataSignal(windowId).value=fresh;primeWindowSignals(windowId,fresh);});
 unregister=registerWorkspaceRefresh({windowId,scope:'window'},{
  admit:async()=>{await metadataConnector.get({fresh:true});return {admitted:true};},
  metadata:async()=>({commit}),
 });
 const recover=createPublishedWindowBindingRecovery({refresh:refreshWorkspace,currentTransport:()=>({sessionId:'session-a'}),currentWindow:id=>activeWindows.peek().find(row=>row.windowId===id)});
 const connector=createDataConnector({service:{URL:'/v1/api/datasources/rows/fetch?cohort=1',method:'POST'}},{prepareRequest,recoverWindowBinding:recover,windowId,dataSourceRef:'rows'});
 const begin=()=>connector.get({inputParameters:{recordId:7}}).then(value=>({value}),error=>({error}));
 return {begin,started,finish:()=>resolveMetadata(),handlers,signals,commit,replaceCapability:()=>{off();controls.push(capability());},counts:()=>({dataRequests,metadataRequests})};
}

it('readmits a retained window after a missing server binding and replays only its read',async()=>{
 const f=fixture(),request=f.begin();await f.started;f.finish();const result=await request;
 expect(result.error).toBeUndefined();expect(result.value).toEqual({data:[{id:7}]});
 expect(f.counts()).toEqual({dataRequests:2,metadataRequests:1});expect(f.commit).toHaveBeenCalledOnce();
 expect(f.signals.form.peek()).toEqual({name:'retained draft'});
});

it.each([
 ['loading',f=>{f.handlers.setLoading(false);invalidateControlActionState(windowId);}],
 ['error',f=>f.handlers.setError(new Error('Background read unavailable'))],
 ['collection',f=>f.handlers.setCollection([{id:8}])],
 ['request completion',f=>{f.signals.input.value=consumeDataSourceFetchRequest(f.signals.input.peek());}],
 ['clean baseline version',f=>f.handlers.resetFormDirty()],
])('readmission tolerates native background %s without a human edit',async(_name,settle)=>{
 const f=fixture(),request=f.begin();await f.started;
 settle(f);const currentControl=f.signals.control.peek(),currentRows=f.signals.collection.peek();
 f.finish();const result=await request;
 expect(result.error).toBeUndefined();expect(result.value).toEqual({data:[{id:7}]});
 expect(f.counts()).toEqual({dataRequests:2,metadataRequests:1});expect(f.commit).toHaveBeenCalledOnce();
 expect(f.signals.form.peek()).toEqual({name:'retained draft'});
 expect(f.signals.input.peek()).toMatchObject({filter:{query:'retained'},page:2,parameters:{recordId:7}});
 expect(f.signals.control.peek()).toEqual(currentControl);expect(f.signals.collection.peek()).toEqual(currentRows);
});

it('a concurrent real draft edit rejects readmission commit and never replays stale input',async()=>{
 const f=fixture(),request=f.begin();await f.started;
 f.handlers.setFormField({item:{dataField:'name'},value:'new human edit'});
 f.finish();const result=await request;
 expect(result.error?.message).toMatch(/stale UI operation revision/);
 expect(f.commit).not.toHaveBeenCalled();expect(f.counts()).toEqual({dataRequests:1,metadataRequests:1});
 expect(f.signals.form.peek()).toEqual({name:'new human edit'});
});

it.each([
 ['input',f=>f.handlers.setInputParameters({recordId:8})],
 ['selection',f=>{f.signals.selection.value={selected:{id:8}};}],
 ['window parameters',()=>{activeWindows.peek()[0].parameters.recordId=8;}],
 ['metadata',()=>{getMetadataSignal(windowId).value={...getMetadataSignal(windowId).peek(),title:'changed definition'};}],
 ['registered capability',f=>f.replaceCapability()],
])('readmission still rejects changed %s before commit or replay',async(_name,change)=>{
 const f=fixture(),request=f.begin();await f.started;change(f);f.finish();const result=await request;
 expect(result.error?.message).toMatch(/stale UI operation revision/);
 expect(f.commit).not.toHaveBeenCalled();expect(f.counts()).toEqual({dataRequests:1,metadataRequests:1});
});
