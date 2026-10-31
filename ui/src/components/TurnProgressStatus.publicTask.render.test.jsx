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
import IterationBlock from './chat/IterationBlock.jsx';
import RunProgressView from './RunProgressView.jsx';
const conversationId='public-task-conversation',turnId='public-task-turn';
const task='Preparing a Coca-Cola draft with an assumed budget.';
const highlight='Retained public highlights';
const meta=(id,mode='task',extra={})=>({agently:{presentation:{version:'1',conversationId,nativeTurnId:turnId,nativeMessageId:id,mode,createdAt:'2026-10-11T20:00:01Z',...extra}}});
let root,container;
const openWorkspace=vi.fn();
afterEach(async()=>{if(root)await act(async()=>root.unmount());container?.remove();root=null;vi.useRealTimers();});
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
async function mount(state,developerMode,documentPresentation=false,reportsInWorkspace=false){
 rows=projectConversation(state);
 if(!root){container=document.createElement('div');document.body.append(container);root=createRoot(container);}
 await act(async()=>root.render(<ConversationViewContext.Provider value={{developerMode,toolFeedDock:'inline',presentation:documentPresentation?'document':undefined,reportsInWorkspace,onOpenWorkspace:reportsInWorkspace?openWorkspace:undefined}}><DetailContext.Provider value={{showDetail:()=>{}}}>{!documentPresentation?<TurnProgressStatus conversationId={conversationId} developerMode={developerMode}/>:null}<ChatFeed conversationId={conversationId} rowsOverride={rows}/></DetailContext.Provider></ConversationViewContext.Provider>));
}
describe('public task narration in mounted turn status',()=>{
 for(const lifecycle of ['completed','cancelled'])it(`keeps recorded history clickable after a sparse ${lifecycle} update`,async()=>{
  vi.useFakeTimers({toFake:['Date','setInterval','clearInterval']});
  vi.setSystemTime(new Date('2026-10-11T20:00:10Z'));
  container=document.createElement('div');document.body.append(container);root=createRoot(container);
  const row={kind:'iteration',turnId:'recorded-history',turnStartedAt:'2026-10-11T20:00:00Z',lifecycle:'running',rounds:[{pageId:'recorded-page',phase:'main',status:'running',content:'PRIVATE_STEP_PROSE',toolCalls:[{toolCallId:'recorded-step',toolName:'example/recorded-step',status:'completed'}]}]};
  const show=next=>act(async()=>root.render(<ConversationViewContext.Provider value={{presentation:'document',developerMode:false}}><IterationBlock canonicalRow={next} conversationId="history-fixture" showToolFeedDetail={false} progress={next.lifecycle==='running'?<RunProgressView startedAt={next.turnStartedAt}/>:null}/></ConversationViewContext.Provider>));
  await show(row);
  const toggle=container.querySelector('.app-iteration-head');
  vi.setSystemTime(new Date('2026-10-11T20:00:15Z'));
  await show({...row,lifecycle,rounds:[]});
  const finished=container.querySelector('[data-testid="turn-completed-status"] .app-iteration-head');
  expect(finished).toBe(toggle);
  expect(finished.textContent).toContain('Worked for 15s');
  expect(finished.getAttribute('aria-expanded')).toBe('false');
  await act(async()=>finished.click());
  expect(finished.getAttribute('aria-expanded')).toBe('true');
  expect(container.querySelector('.app-iteration-groups').textContent).toContain('example/recorded-step');
  expect(container.querySelector('.app-iteration-link')).toBeNull();
  expect(container.textContent).not.toContain('PRIVATE_STEP_PROSE');
  await act(async()=>finished.click());expect(finished.getAttribute('aria-expanded')).toBe('false');
  await show({...row,turnId:'different-request',rounds:[]});
  expect(container.textContent).not.toContain('example/recorded-step');
  await show({...row,turnId:'different-request',lifecycle:'cancelled',rounds:[]});
  await act(async()=>container.querySelector('[data-testid="turn-completed-status"] button').click());
  expect(container.textContent).toContain('No steps were recorded for this request.');
  expect(container.textContent).not.toContain('example/recorded-step');
 });
 it('expands public step metadata inline without model prose or payload actions',async()=>{
  const state=await initialState();await mount(state,false,true,true);
  const activity=container.querySelector('.app-run-activity');
  const toggle=activity.querySelector('.app-iteration-head');
  await act(async()=>toggle.click());
  expect(toggle.getAttribute('aria-expanded')).toBe('true');
  expect(activity.textContent).toContain('ordinary-tool');
  expect(activity.querySelector('.app-iteration-groups')).not.toBeNull();
  expect(activity.querySelector('.app-iteration-link')).toBeNull();
  expect(activity.textContent).not.toMatch(/PRIVATE_REASONING_CREDENTIAL|PRIVATE_TASK_MODEL_CONTENT|BINARY_TOOL_SECRET/);
  expect(activity.textContent).not.toContain(task);
  expect(activity.textContent).not.toContain(highlight);
 });
 it('keeps activity in place and appends findings and report only after completion',async()=>{
  vi.useFakeTimers({toFake:['Date','setInterval','clearInterval']});
  vi.setSystemTime(new Date('2026-10-11T20:00:02Z'));
  let state=await initialState();
  state.turns.find(turn=>turn.turnId===turnId).operationalNarration=highlight;
  await mount(state,true,true,true);
  const progress=()=>container.querySelector('[data-testid="turn-progress-status"]');
  expect(progress().textContent).toContain('Preparing report');
  expect(progress().querySelector('[data-testid="turn-progress-narration"]')).toBeNull();
  const card=()=>container.querySelector('[data-testid="workspace-attachment-card"]');
  expect(card()).toBeNull();
  const activity=container.querySelector('.app-run-activity');
  const disclosure=activity.querySelector('.app-iteration-head');
  expect(disclosure.getAttribute('aria-expanded')).toBe('false');
  expect(container.querySelector('.app-forge-fence-loading,.app-rich-inline-report')).toBeNull();
  expect(container.textContent).not.toContain(highlight);
  expect(container.textContent).not.toContain(task);
  await act(async()=>vi.advanceTimersByTime(31000));
  expect(progress().textContent).toContain('Preparing report');
  expect(progress().textContent).toContain('No new updates for 0:31');
  const update=(status,sequence)=>{
   state=applyEvent(state,{type:'assistant',conversationId,turnId,messageId:'report',pageId:'report-page',mode:'task',patch:{role:'assistant'},content:'[Interactive content]',
    renderedContent:{schemaVersion:'1',parts:[],reports:[{scope:'message',id:'pending-report',status,grammar:'report-document-v1',sequence,
     source:{title:'Evidence review',blocks:[{id:'summary',kind:'markdownBlock',markdown:'Full report detail belongs in the viewer.'}]},
     dataSources:{evidence:{id:'evidence',sequence,format:'json',payload:[{value:sequence}]}}}]}});
  };
  update('rendering',1);await mount(state,true,true,true);
  expect(progress().textContent).not.toContain('No new updates');
  expect(card()).toBeNull();
  expect(container.querySelector('.app-run-activity')).toBe(activity);
  expect(disclosure.getAttribute('aria-expanded')).toBe('false');
  expect(container.textContent).not.toContain('Full report detail belongs in the viewer.');
  update('committed',2);await mount(state,true,true,true);expect(card()).toBeNull();
  state=applyEvent(state,{type:'turn_completed',conversationId,turnId,status:'completed'});
  await mount(state,true,true,true);
  expect(progress()).toBeNull();expect(card().disabled).toBe(false);
  const identity=card().getAttribute('data-workspace-object-id');
  expect(container.querySelector('.app-run-activity')).toBe(activity);
  expect(activity.querySelector('[data-testid="turn-completed-status"]')).not.toBeNull();
  expect(activity.querySelector('.app-iteration-head')).toBe(disclosure);
  expect(disclosure.textContent).toContain('Worked for 32s');
  expect(disclosure.getAttribute('aria-expanded')).toBe('false');
  expect(activity.compareDocumentPosition(container.querySelector('.app-bubble-assistant')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(container.querySelectorAll('[data-testid="workspace-attachment-card"]').length).toBe(1);
  expect(container.textContent.split(highlight).length-1).toBe(1);
  expect(container.textContent).not.toContain('Full report detail belongs in the viewer.');
  openWorkspace.mockClear();await act(async()=>card().click());
  expect(openWorkspace).toHaveBeenCalledOnce();
  expect(openWorkspace.mock.calls[0][0].workspaceObject.objectId).toBe(identity);
  expect(openWorkspace.mock.calls[0][0].workspaceObject.content).not.toHaveProperty('source');
  vi.useRealTimers();
 });
 it('contains progress and execution details together while public output stays outside',async()=>{
  let state=await initialState();
  state.turns.find(turn=>turn.turnId===turnId).operationalNarration='### Key findings\n- **Main finding:** review the scope.\n- Read [details](https://example.com/report).';
  await mount(state,true,true,true);
  const activity=container.querySelector('.app-run-activity');
  expect(activity.getAttribute('aria-label')).toBe('Request activity');
  expect(activity.querySelectorAll('[data-testid="turn-progress-status"]').length).toBe(1);
  expect(activity.querySelector('.app-iteration-card')).not.toBeNull();
  expect(activity.querySelector('[data-testid="execution-status-narration"]')).toBeNull();
  expect(activity.querySelector('.app-iteration-title-time')).toBeNull();
  expect(activity.textContent).not.toContain(task);
  expect(activity.textContent).not.toContain('Building report');
  const toggle=activity.querySelector('.app-iteration-head');
  expect(toggle.getAttribute('aria-expanded')).toBe('false');
  const details=container.querySelector(`[id="${toggle.getAttribute('aria-controls')}"]`);
  expect(details.hidden).toBe(toggle.getAttribute('aria-expanded')==='false');
  const wasExpanded=toggle.getAttribute('aria-expanded');
  await act(async()=>toggle.click());
  expect(toggle.getAttribute('aria-expanded')).not.toBe(wasExpanded);
  expect(details.hidden).toBe(toggle.getAttribute('aria-expanded')==='false');
  state=applyEvent(state,{type:'turn_completed',conversationId,turnId,status:'completed'});await mount(state,true,true,true);
  expect(container.querySelectorAll('.app-run-activity').length).toBe(1);
  expect(container.querySelector('[data-testid="turn-progress-status"]')).toBeNull();
  expect(activity.querySelector('[data-testid="turn-completed-status"]')).not.toBeNull();
  expect(toggle.getAttribute('aria-expanded')).toBe('true');
  expect(activity.compareDocumentPosition(container.querySelector('.app-bubble-assistant')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(container.textContent).toContain(task);
  expect(container.querySelector('.app-run-activity').textContent).not.toContain(task);
 });
 it('keeps the standalone legacy execution preview readable',async()=>{
  const state=await initialState();
  state.turns.find(turn=>turn.turnId===turnId).operationalNarration='### Key findings\n- **Main finding:** review the scope.\n- Read [details](https://example.com/report).';
  await mount(state,true);
  const preview=container.querySelector('[data-testid="execution-status-narration"]');
  expect(preview).not.toBeNull();
  expect(preview.textContent).toBe('Key findings Main finding: review the scope. Read details.');
  expect(preview.title).toBe(preview.textContent);
  expect(preview.querySelector('a,button')).toBeNull();
  expect(container.querySelector('.app-run-activity')).toBeNull();
 });
 it('defers admitted document replies until completion while retaining progress',async()=>{
  let state=await initialState();await mount(state,false,true);
  expect(container.textContent).not.toContain(task);
  const progress=container.querySelector('[data-testid="turn-progress-status"]');
  expect(progress).not.toBeNull();expect(progress.querySelector('[aria-label$=" elapsed"]')).not.toBeNull();
  expect([...progress.querySelectorAll('button')].some(button=>button.textContent==='Stop request')).toBe(false);
  expect(progress.textContent).not.toContain(task);
  expect(container.textContent).not.toContain(highlight);expect(container.textContent).not.toContain('Building report');
  expect(container.textContent).not.toMatch(/PRIVATE_REASONING_CREDENTIAL|PRIVATE_TASK_MODEL_CONTENT|BINARY_TOOL_SECRET/);
  state=applyEvent(state,{type:'turn_completed',conversationId,turnId,status:'completed'});await mount(state,false,true);
  expect(container.querySelector('[data-testid="turn-progress-status"]')).toBeNull();
  expect(container.textContent.split(task).length-1).toBe(1);
  expect(container.textContent).toContain(highlight);expect(container.textContent).toContain('Building report');
 });
 for(const developerMode of [false,true])it(`withholds draft Markdown from document progress and reply (${developerMode})`,async()=>{
  let state=newConversationState(conversationId);
  const markdown='### Finished findings\n- **Reviewed evidence:** keep the scope.';
  applyTranscript(state,{conversationId,turns:[{turnId,status:'running',createdAt:'2026-10-11T20:00:00Z',
   user:{messageId:'question',content:'Review the scope.'},operationalNarration:markdown,
   assistant:{final:{messageId:'answer',content:markdown}},
   execution:{pages:[{pageId:'answer-page',status:'running',phase:'main',content:markdown}]}}]});
  await mount(state,developerMode,true);
  expect(container.querySelector('[data-testid="turn-progress-narration"]')).toBeNull();
  expect(container.querySelector('.app-bubble-assistant')).toBeNull();
  expect(container.textContent).not.toContain('Finished findings');
  state=applyEvent(state,{type:'turn_completed',conversationId,turnId,status:'completed'});
  await mount(state,developerMode,true);
  expect(container.querySelectorAll('.app-bubble-assistant h3')).toHaveLength(1);
  expect(container.querySelector('.app-bubble-assistant').textContent).toContain('Finished findings');
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
