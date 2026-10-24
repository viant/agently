// @vitest-environment jsdom
import React,{act} from 'react';
import {createRoot} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import ToolFeedDetail from './ToolFeedDetail.jsx';
import {SettingProvider} from 'forge/core';
import {runUICommand} from 'forge/core/ui/commands.js';
import {getWindowContext} from 'forge/core/context/registry.js';
import {getFormSignal} from 'forge/core/store/signals.js';
import {applyFeedEvent,updateFeedData,clearFeedState} from '../services/toolFeedBus.js';
import spec from './__fixtures__/ownedMediaPlan.json';
const output=spec.output;
let root,element;
const flush=()=>act(async()=>{for(let i=0;i<25;i++)await Promise.resolve()});
afterEach(async()=>{if(root)await act(async()=>root.unmount());element?.remove();root=null;clearFeedState();vi.unstubAllGlobals()});
it('the real mounted owned feed exposes only its own declared conversation state',async()=>{
 globalThis.IS_REACT_ACT_ENVIRONMENT=true;vi.stubGlobal('ResizeObserver',class{observe(){}unobserve(){}disconnect(){}});
 const conversationId='owned-feed-conversation';const windowId=`feed-media-plan-${conversationId}`;
 // Tracked fixture declares the actual feed renderer inputs; no renderer or manager is mocked.
 updateFeedData('media-plan',{title:spec.title,data:{output},dataSources:spec.dataSource,ui:spec.ui},conversationId);
 applyFeedEvent({type:'tool_feed_active',feedId:'media-plan',conversationId,turnId:'turn-fixture',feedTarget:'inline',feedItemCount:3});
 element=document.createElement('div');document.body.append(element);root=createRoot(element);
 await act(async()=>root.render(<SettingProvider><ToolFeedDetail conversationId={conversationId} turnId="turn-fixture"/></SettingProvider>));await flush();
 expect(element.textContent).toContain('Coca-Cola');expect(element.textContent).toContain('Total budget');
 const budget=()=>element.querySelector('input[data-forge-control-id="totalBudget"], [data-forge-control-id="totalBudget"] input');
 expect(budget()?.value).toBe('100000');
 expect(getWindowContext(windowId)?.identity).toMatchObject({feedId:'media-plan',conversationId});
 await act(async()=>{getFormSignal(`${windowId}DSeditDraft`).value={...getFormSignal(`${windowId}DSeditDraft`).peek(),totalBudget:120000}});await flush();
 expect(budget()?.value).toBe('120000');
 const result=await runUICommand({method:'ui.feed.get',params:{feedId:'media-plan',conversationId,dataSourceRefs:['editDraft','channels']}});
 expect(result.dataSources.editDraft.form.totalBudget).toBe(120000);expect(result.dataSources.channels.collection).toHaveLength(3);
 await expect(runUICommand({method:'ui.feed.get',params:{feedId:'unknown',conversationId,dataSourceRefs:['editDraft']}})).rejects.toThrow('active conversation feed');
 await expect(runUICommand({method:'ui.feed.get',params:{feedId:'media-plan',conversationId:'foreign',dataSourceRefs:['editDraft']}})).rejects.toThrow('active conversation feed');
 await expect(runUICommand({method:'ui.feed.get',params:{feedId:'media-plan',conversationId,dataSourceRefs:['private-window-source']}})).rejects.toThrow('feed datasource');
 await expect(runUICommand({method:'ui.feed.update',params:{feedId:'unknown',conversationId,operations:[{dataSourceRef:'editDraft',op:'replace',path:'/form/totalBudget',value:1}]}})).rejects.toThrow('active conversation feed');
 await act(async()=>root.unmount());root=null;expect(getWindowContext(windowId)).toBeUndefined();
 await expect(runUICommand({method:'ui.feed.get',params:{feedId:'media-plan',conversationId,dataSourceRefs:['editDraft']}})).rejects.toThrow('active conversation feed');
});

it('retains a document root when its only child has no layout',async()=>{
 globalThis.IS_REACT_ACT_ENVIRONMENT=true;vi.stubGlobal('ResizeObserver',class{observe(){}unobserve(){}disconnect(){}});
 const conversationId='document-root-conversation';
 updateFeedData('example-document',{data:{output:{state:'sample'}},dataSources:{sample:{source:'output'}},ui:{renderMode:'forge',appearance:'document',containers:[{id:'identity',dataSourceRef:'sample',items:[{id:'document-title',type:'label',value:'Example document',properties:{as:'h1',typographyRole:'title'}}]}]}},conversationId);
 applyFeedEvent({type:'tool_feed_active',feedId:'example-document',conversationId,turnId:'document-turn',feedTarget:'inline',feedItemCount:1});
 element=document.createElement('div');document.body.append(element);root=createRoot(element);
 await act(async()=>root.render(<SettingProvider><ToolFeedDetail conversationId={conversationId} turnId="document-turn"/></SettingProvider>));await flush();
 expect(element.querySelector('.app-tool-feed-detail-forge')?.dataset.forgeAppearance).toBe('document');
 expect(element.querySelectorAll('.forge-container[data-forge-appearance="document"]')).toHaveLength(1);
 expect(element.querySelector('[data-forge-container-id="identity"]')).not.toBeNull();
});
