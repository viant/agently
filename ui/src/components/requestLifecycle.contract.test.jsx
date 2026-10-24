import React, {act} from 'react';
import {createRoot} from 'react-dom/client';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {client} from '../services/agentlyClient.js';
import {onTranscript} from '../services/chatStore.js';
import TurnProgressStatus from './TurnProgressStatus.jsx';
import {ElicitationDialog} from './ElicitationOverlay.jsx';
import ChatFeed from './chat/ChatFeedFromChatStore.jsx';
import {ConversationViewContext} from '../context/ConversationViewContext.js';
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
let root, element;
afterEach(async()=>{if(root)await act(async()=>root.unmount());element?.remove();root=null;vi.restoreAllMocks();});
async function mount(node){if(!root){element=document.createElement('div');document.body.append(element);root=createRoot(element);}await act(async()=>root.render(node));}
const transcript=(id,status='running')=>({conversationId:id,turns:[{turnId:`${id}-turn`,status,createdAt:new Date(Date.now()-14000).toISOString(),user:{messageId:'question',content:'Review this example.'},execution:{pages:[{pageId:'page',status,phase:'thinking',content:''}]}}]});
describe('document request lifecycle',()=>{
 it('uses the actual scoped cancellation, retains failure and resets for the next turn',async()=>{
  const cancel=vi.spyOn(client,'cancelTurn').mockRejectedValue(new Error('private stop diagnostic'));
  onTranscript('run-example',transcript('run-example'));
  await mount(<TurnProgressStatus appearance="document" conversationId="run-example"/>);
  expect(element.textContent).toContain('elapsed');expect(element.textContent).not.toContain('tokens');
  await act(async()=>element.querySelector('button').click());
  expect(cancel).toHaveBeenCalledWith('run-example-turn');
  expect(element.querySelector('[role=alert]').textContent).toContain('Try again');
  expect(element.textContent).not.toContain('private stop diagnostic');
  cancel.mockResolvedValue(true);
  await act(async()=>element.querySelector('button').click());expect(element.textContent).toContain('Stopping request');
  onTranscript('next-example',transcript('next-example'));await mount(<TurnProgressStatus appearance="document" conversationId="next-example"/>);
  expect(element.querySelector('button').disabled).toBe(false);expect(element.textContent).not.toContain('Stopping request');
 });
 it('places one public progress record at the request and hides it at completion',async()=>{
  onTranscript('inline-example',transcript('inline-example'));
  const node=<ConversationViewContext.Provider value={{presentation:'document'}}><ChatFeed conversationId="inline-example"/></ConversationViewContext.Provider>;
  await mount(node);expect(element.querySelectorAll('[data-testid=turn-progress-status]').length).toBe(1);
  expect(element.querySelector('.app-chat-feed [data-testid=turn-progress-status]')).not.toBeNull();
  await act(async()=>onTranscript('inline-example',transcript('inline-example','completed')));
  expect(element.querySelector('[data-testid=turn-progress-status]')).toBeNull();
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
