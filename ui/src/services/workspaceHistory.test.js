import {describe, it, expect} from 'vitest';
import {deriveWorkspaceHistoryFromTranscriptTurns} from 'agently-core-ui-sdk/workspaceRestore';
const object=(id)=>({windowId:id,windowKey:'resource',conversationId:'c',presentation:'hosted',region:'chat.top',parentKey:'chat/new',parameters:{id}});
const turn=(id,steps)=>({id,conversationId:'c',status:'completed',execution:{pages:[{toolSteps:steps}]}});
const step=(name,response,request={})=>({toolName:name,status:'completed',responsePayload:response,requestPayload:request});
describe('workspace transcript history',()=>{
 it('replays multiple opened objects and a later close without losing owning turn',()=>{
  const turns=[turn('origin',[step('ui/view/open',{ok:true,items:[object('one'),object('two')]})]),
   turn('later',[step('ui/window/show',{}, {windowId:'one'}),step('ui/window/close',{}, {windowId:'one'})])];
  const history=deriveWorkspaceHistoryFromTranscriptTurns(turns);
  expect(history).toHaveLength(2);
  expect(history[0].workspaceObject.origin.turnId).toBe('origin');
  expect(history[0].workspaceObject.lifecycle.state).toBe('closed');
  expect(history[1].workspaceObject.lifecycle.state).toBe('ready');
 });
 it('keeps original origin when the same object is explicitly opened again',()=>{
  const result=deriveWorkspaceHistoryFromTranscriptTurns([
   turn('origin',[step('ui/view/open',{ok:true,...object('one')})]),
   turn('later',[step('ui/view/open',{ok:true,...object('one')})])]);
  expect(result).toHaveLength(1);
  expect(result[0].workspaceObject.origin.turnId).toBe('origin');
 });
 it('does not project rejected opens or infer objects from assistant prose',()=>{
  expect(deriveWorkspaceHistoryFromTranscriptTurns([turn('failed',[step('ui/view/open',{ok:false,items:[object('one')]})]),
   {...turn('text',[]),messages:[{role:'assistant',content:'The resource is open.'}]}])).toEqual([]);
 });
});
