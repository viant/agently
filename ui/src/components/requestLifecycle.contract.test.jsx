// @vitest-environment jsdom
import React, {act} from 'react';
import {createRoot} from 'react-dom/client';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {client} from '../services/agentlyClient.js';
import {onTranscript} from '../services/chatStore.js';
import TurnProgressStatus from './TurnProgressStatus.jsx';
import RunProgressView, {workDurationLabel} from './RunProgressView.jsx';
import StatusBar from './StatusBar.jsx';
import {setStage} from '../services/stageBus.js';
import {ElicitationDialog} from './ElicitationOverlay.jsx';
import ChatFeed from './chat/ChatFeedFromChatStore.jsx';
import {ConversationViewContext} from '../context/ConversationViewContext.js';
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
let root, element;
afterEach(async()=>{if(root)await act(async()=>root.unmount());element?.remove();root=null;vi.useRealTimers();vi.restoreAllMocks();});
async function mount(node){if(!root){element=document.createElement('div');document.body.append(element);root=createRoot(element);}await act(async()=>root.render(node));}
const transcript=(id,status='running')=>({conversationId:id,turns:[{turnId:`${id}-turn`,status,createdAt:new Date(Date.now()-14000).toISOString(),user:{messageId:'question',content:'Review this example.'},execution:{pages:[{pageId:'page',status,phase:'thinking',content:''}]}}]});
describe('document request lifecycle',()=>{
 it('uses a small completion label with an honest duration',()=>{
  expect(workDurationLabel(30)).toBe('Worked for 30s');
  expect(workDurationLabel(1628)).toBe('Worked for 27m 8s');
  expect(workDurationLabel(3600)).toBe('Worked for 1h');
  expect(workDurationLabel(null)).toBe('Work completed');
 });
 it('gives inline progress the request clock while retaining global attention and approvals',async()=>{
  setStage({phase:'executing',text:'Executing example/check'});
  await mount(<StatusBar requestInFeed/>);
  expect(element.querySelector('footer').style.display).toBe('none');
  await mount(<StatusBar requestInFeed approvals={{pendingCount:1}}/>);
  expect(element.textContent).toContain('Approvals 1');
  expect(element.textContent).not.toContain('Running a step');
  setStage({phase:'error',text:'Network timeout'});
  await mount(<StatusBar requestInFeed/>);
  expect(element.textContent).toContain('connection was interrupted');
  expect(element.querySelector('footer').style.display).not.toBe('none');
  setStage({phase:'ready',text:'Ready'});
 });
 it('keeps technical execution names in developer details instead of public status',async()=>{
  setStage({phase:'executing',text:'Executing ui/view/open…'});
  await mount(<StatusBar/>);
  expect(element.textContent).toContain('Running a step');
  expect(element.textContent).not.toContain('ui/view/open');
  await mount(<StatusBar developerMode/>);
  expect(element.textContent).toContain('Executing ui/view/open');
  setStage({phase:'executing',text:'Completed tool ui/view/open'});
  await mount(<StatusBar/>);
  expect(element.textContent).toContain('Step completed');
 });
 it('distinguishes a quiet request from new progress without inventing stages',async()=>{
  vi.useFakeTimers({toFake:['Date','setInterval','clearInterval']});
  vi.setSystemTime(new Date('2026-10-26T17:00:00Z'));
  const startedAt=new Date().toISOString(), stop=vi.fn();
  const view=(key='one',summary='')=><RunProgressView startedAt={startedAt} progressKey={key} summary={summary} onStop={stop}/>;
  await mount(view());
  expect(element.textContent).toContain('Working on your request');
  expect(element.querySelector('.app-run-progress-spinner')).not.toBeNull();
  expect(element.textContent).not.toContain('leave this chat');
  await act(async()=>vi.advanceTimersByTime(31000));
  expect(element.textContent).toContain('Working on your request');
  expect(element.textContent).toContain('No new updates for 0:31');
  expect(element.textContent).not.toMatch(/failed|reconnect|%|Thinking/);
  await mount(view()); // An unrelated render is not evidence of backend progress.
  await act(async()=>vi.advanceTimersByTime(2000));
  expect(element.textContent).toContain('No new updates for 0:33');
  await mount(view('two','1 step completed'));
  expect(element.textContent).toContain('Working on your request');
  expect(element.textContent).toContain('1 step completed');
  expect(element.textContent).not.toContain('No new updates');
  expect(element.querySelector('.app-run-progress-heading [aria-live=off]').getAttribute('aria-label')).toBe('0:33 elapsed');
  await act(async()=>element.querySelector('button').click());expect(stop).toHaveBeenCalledOnce();
  await mount(<RunProgressView startedAt={startedAt} waiting/>);
  await act(async()=>vi.advanceTimersByTime(31000));
  expect(element.textContent).toContain('Needs your input');
  expect(element.querySelector('.app-run-progress-spinner')).toBeNull();
  expect(element.textContent).not.toContain('Waiting for an update');
  await mount(<RunProgressView startedAt={startedAt} stopping onStop={stop}/>);
  expect(element.querySelector('button').disabled).toBe(true);
  expect(element.textContent).toContain('Waiting for confirmation');
 });
 it('leaves document cancellation in the composer instead of progress',async()=>{
  onTranscript('run-example',transcript('run-example'));
  await mount(<TurnProgressStatus appearance="document" conversationId="run-example"/>);
  expect(element.querySelector('[aria-label$=" elapsed"]')).not.toBeNull();
  expect(element.querySelector('button')).toBeNull();
  expect(element.textContent).not.toContain('Stop');
 });
 it('keeps the activity area in place and replaces progress with completion',async()=>{
  onTranscript('inline-example',transcript('inline-example'));
  const node=<ConversationViewContext.Provider value={{presentation:'document'}}><ChatFeed conversationId="inline-example"/></ConversationViewContext.Provider>;
  await mount(node);expect(element.querySelectorAll('[data-testid=turn-progress-status]').length).toBe(1);
  expect(element.querySelector('.app-chat-feed [data-testid=turn-progress-status]')).not.toBeNull();
  const activity=element.querySelector('.app-run-activity');
  await act(async()=>onTranscript('inline-example',transcript('inline-example','completed')));
  expect(element.querySelector('[data-testid=turn-progress-status]')).toBeNull();
  expect(element.querySelector('.app-run-activity')).toBe(activity);
  expect(element.querySelector('[data-testid=turn-completed-status]')).not.toBeNull();
 });
 it('retains selection after a rejected decision and disables historical review',async()=>{
  const resolve=vi.spyOn(client,'resolveElicitation').mockRejectedValue(new Error('Decision did not complete. Try again.'));
  const pending={conversationId:'approval-example',elicitationId:'review-example',requestedSchema:{type:'object',properties:{_type:{const:'tool_approval'},_acceptLabel:{const:'Save recommendation'},scope:{type:'string',title:'Scope',default:'Original scope'},rows:{type:'array','x-ui-widget':'planner.table',title:'Proposed items',items:{type:'object',properties:{id:{type:'string'},selected:{type:'boolean'},rationale:{type:'string'}}},default:[{id:'one',selected:true,rationale:'Keep this evidence.'},{id:'two',selected:false,rationale:'Keep this evidence too.'}]}}}};
  await mount(<ElicitationDialog appearance="document" pending={pending}/>);
  const dialog=document.querySelector('.app-request-review-dialog');
  const choices=dialog.querySelectorAll('tbody input[type=checkbox]');expect(choices.length).toBe(2);
  await act(async()=>choices[1].click());
  const scope=dialog.querySelector('input[type=text]');
  await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(scope,'Edited scope');scope.dispatchEvent(new Event('input',{bubbles:true}));});
  const save=[...dialog.querySelectorAll('button')].find(b=>b.textContent==='Save recommendation');await act(async()=>save.click());
  expect(resolve.mock.calls[0][2].payload.scope).toBe('Edited scope');
  expect(resolve.mock.calls[0][2].payload.rows.map(r=>r.selected)).toEqual([true,true]);
  expect(dialog.querySelector('[role=alert]').textContent).toContain('Check its status');
  expect([...dialog.querySelectorAll('tbody input')].every(input=>input.checked)).toBe(true);
  await mount(<ElicitationDialog appearance="document" pending={{...pending,status:'accepted'}}/>);
  expect([...document.querySelectorAll('.app-request-review-dialog input[type=checkbox]')].every(input=>input.disabled)).toBe(true);
 });
});
