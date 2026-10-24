// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { AgUiClient } from 'agently-core-ui-sdk/agui';
import { AgUiViewProjection } from 'agently-core-ui-sdk/aguiViewProjection';
import { applyEvent, applyTranscript, newConversationState } from 'agently-core-ui-sdk/chatStore/reducer';
import { projectConversation } from 'agently-core-ui-sdk/chatStore/projector';
globalThis.IS_REACT_ACT_ENVIRONMENT=true;
let rows=[];
vi.mock('../services/chatStore',()=>({useChatProjection:()=>rows}));
vi.mock('../services/stageBus',()=>({useStage:()=>({phase:'executing'}),setStage:vi.fn()}));
vi.mock('../services/usageBus',()=>({getUsage:()=>({}),onUsageChange:()=>()=>{}}));
vi.mock('../services/agentlyClient',()=>({client:{cancelTurn:vi.fn(),browserMCPAppActivities:()=>[],subscribeBrowserMCPApps:()=>()=>{}}}));
vi.mock('./chat/LazyRichContent',async()=>({default:(await import('./chat/RichContent.jsx')).default}));
import TurnProgressStatus from './TurnProgressStatus.jsx';
import ChatFeed from './chat/ChatFeedFromChatStore.jsx';
import { ConversationViewContext } from '../context/ConversationViewContext.js';
import { DetailContext } from '../context/DetailContext.js';
const conversationId='public-task-conversation',turnId='public-task-turn';
const task='Preparing a Coca-Cola draft with an assumed budget.';
const highlight='Retained public highlights';
const meta=(id,mode='task',extra={})=>({agently:{presentation:{version:'1',conversationId,nativeTurnId:turnId,nativeMessageId:id,mode,createdAt:'2026-10-11T20:00:01Z',...extra}}});
let root,container;
afterEach(async()=>{if(root)await act(async()=>root.unmount());container?.remove();root=null;});
async function initialState(){
 let state=newConversationState(conversationId);
 const projection=new AgUiViewProjection({profile:'agently',allowHostEffects:true,logicalTurnId:turnId,conversationId,onViewEvent:event=>{state=applyEvent(state,event)}});
 const events=[{type:'RUN_STARTED',threadId:conversationId,runId:'public-run'},
 {type:'ACTIVITY_SNAPSHOT',messageId:'lifecycle',activityType:'agently.turn',content:{version:'1',nativeTurnId:turnId,status:'running',queueSequence:''},metadata:meta('lifecycle')},
 ...[['task-note',task,'task'],['highlights',highlight,'highlight']].flatMap(([id,text,mode])=>[
 {type:'TEXT_MESSAGE_START',messageId:id,role:'assistant',metadata:meta(id,mode,{messageKind:'standalone'})},
 {type:'TEXT_MESSAGE_CONTENT',messageId:id,delta:text},{type:'TEXT_MESSAGE_END',messageId:id}]),
 {type:'TEXT_MESSAGE_START',messageId:'internal',role:'assistant',metadata:meta('internal','chain',{modelCallId:'internal-model',pageId:'internal-page'})},
 {type:'TEXT_MESSAGE_CONTENT',messageId:'internal',delta:'PRIVATE_REASONING_CREDENTIAL'},
 {type:'TEXT_MESSAGE_END',messageId:'internal'},
 {type:'TEXT_MESSAGE_START',messageId:'private-task-model',role:'assistant',metadata:meta('private-task-model','task',{pageId:'private-task-page',modelCallId:'private-task-model-call'})},
 {type:'TEXT_MESSAGE_CONTENT',messageId:'private-task-model',delta:'PRIVATE_TASK_MODEL_CONTENT'},
 {type:'TEXT_MESSAGE_END',messageId:'private-task-model'},
 {type:'TOOL_CALL_START',toolCallId:'pending-call',toolCallName:'ordinary-tool',parentMessageId:'model-message',metadata:meta('model-message','task',{pageId:'model-page',modelCallId:'model-call'})},
 {type:'TOOL_CALL_ARGS',toolCallId:'pending-call',delta:'{"raw":"BINARY_TOOL_SECRET"}'},{type:'TOOL_CALL_END',toolCallId:'pending-call'}];
 const client=new AgUiClient({url:'/fixture',threadId:conversationId,initialMessages:[],fetch:async()=>new Response(events.map(event=>'data: '+JSON.stringify(event)+'\n\n').join(''),{headers:{'Content-Type':'text/event-stream'}})});
 await client.run({runId:'public-run'},projection.subscriber);
 // Existing typed pending report stays a separate renderer channel.
 state=applyEvent(state,{type:'assistant',conversationId,turnId,patch:{role:'assistant'},messageId:'report',createdAt:'2026-10-11T20:00:02Z',pageId:'report-page',mode:'task',content:'[Interactive content]',renderedContent:{schemaVersion:'1',parts:[],reports:[{scope:'message',id:'pending-report',status:'rendering',grammar:'report-document-v1',progress:{phase:'data',message:'Building report',elapsedSeconds:1},source:{blocks:[]}}]}});
 return state;
}
async function mount(state,developerMode,documentPresentation=false){
 rows=projectConversation(state);
 if(!root){container=document.createElement('div');document.body.append(container);root=createRoot(container);}
 await act(async()=>root.render(<ConversationViewContext.Provider value={{developerMode,toolFeedDock:'inline',presentation:documentPresentation?'document':undefined}}><DetailContext.Provider value={{showDetail:()=>{}}}>{!documentPresentation?<TurnProgressStatus conversationId={conversationId} developerMode={developerMode}/>:null}<ChatFeed conversationId={conversationId} rowsOverride={rows}/></DetailContext.Provider></ConversationViewContext.Provider>));
}
describe('public task narration in mounted turn status',()=>{
 it('renders admitted AG-UI narration once in document chat while retaining progress and reports',async()=>{
  let state=await initialState();await mount(state,false,true);
  expect(container.textContent.split(task).length-1).toBe(1);
  const progress=container.querySelector('[data-testid="turn-progress-status"]');
  expect(progress).not.toBeNull();expect(progress.textContent).toContain('elapsed');
  expect([...progress.querySelectorAll('button')].some(button=>button.textContent==='Stop request')).toBe(true);
  expect(progress.textContent).not.toContain(task);
  expect(container.textContent).toContain(highlight);expect(container.textContent).toContain('Building report');
  expect(container.textContent).not.toMatch(/PRIVATE_REASONING_CREDENTIAL|PRIVATE_TASK_MODEL_CONTENT|BINARY_TOOL_SECRET/);
  state=applyEvent(state,{type:'turn_completed',conversationId,turnId,status:'completed'});await mount(state,false,true);
  expect(container.querySelector('[data-testid="turn-progress-status"]')).toBeNull();
  expect(container.textContent.split(task).length-1).toBe(1);
  expect(container.textContent).toContain(highlight);expect(container.textContent).toContain('Building report');
 });
 for(const developerMode of [false,true])it(`shows task before tool completion and preserves highlights after refresh (${developerMode})`,async()=>{
  let state=await initialState();await mount(state,developerMode);
  expect(container.querySelector('[data-testid="turn-progress-narration"]')?.textContent).toBe(task);
  expect(container.textContent).toContain(highlight);
  expect(container.textContent).toContain('Building report');
  expect(container.querySelector('[data-testid="turn-progress-status"]')?.textContent).not.toContain('Building report');
  expect(container.querySelector('[data-testid="turn-progress-status"]')?.textContent).not.toContain('PRIVATE_REASONING_CREDENTIAL');
  expect(container.querySelector('[data-testid="turn-progress-status"]')?.textContent).not.toContain('PRIVATE_TASK_MODEL_CONTENT');
  expect(rows.some(row=>row.kind==='assistant'&&row.messageId==='private-task-model')).toBe(false);
  expect(container.querySelector('[data-testid="turn-progress-status"]')?.textContent).not.toContain('BINARY_TOOL_SECRET');
  state=applyEvent(state,{type:'turn_completed',conversationId,turnId,status:'completed'});await mount(state,developerMode);
  expect(container.querySelector('[data-testid="turn-progress-status"]')).toBeNull();
  expect(container.textContent).toContain(highlight);
  const restored=newConversationState(conversationId);
  const snapshot={conversationId,turns:[{turnId,status:'completed',messages:[{messageId:'task-note',role:'assistant',mode:'task',interim:0,content:task},{messageId:'highlights',role:'assistant',mode:'highlight',interim:0,content:highlight}],execution:{pages:[]}}]};
  applyTranscript(restored,snapshot);applyTranscript(restored,snapshot);await mount(restored,developerMode);
  expect(container.textContent).toContain(highlight);expect(container.querySelector('[data-testid="turn-progress-status"]')).toBeNull();
 });
});
