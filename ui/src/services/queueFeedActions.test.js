import {describe,it,expect,vi} from 'vitest';
vi.mock('./agentlyClient',()=>({client:{cancelQueuedTurn:vi.fn(async()=>{}),editQueuedTurn:vi.fn(async()=>{}),moveQueuedTurn:vi.fn(async()=>{}),forceSteerQueuedTurn:vi.fn(async()=>({status:'accepted'})),reconcileAgUiConversation:vi.fn(async()=>({}))}}));
vi.mock('./chatRuntime',async importOriginal=>({...await importOriginal(),dsTick:vi.fn(async()=>({}))}));
import {createFeedContext} from './feedForgeContext';
import {client} from './agentlyClient';
import {dsTick} from './chatRuntime';
describe('existing authored Queue feed actions',()=>{
 it('uses captured conversation scope and actual selected row for native controls',async()=>{
  const context=createFeedContext('queue',{queueTurns:{source:'output.queuedTurns'}},'queue-owner-conversation');
  const scoped=context.Context('queueTurns');
  scoped.signals.collection.value=[{id:'native-queued-turn',preview:'Same prompt',status:'queued'}];
  scoped.signals.selection.value={selected:{id:'native-queued-turn'},rowIndex:0};
  await context.lookupHandler('chat.cancelQueuedTurn')({context:scoped});
  await context.lookupHandler('chat.moveQueuedTurnUp')({context:scoped});
  await context.lookupHandler('chat.forceSteerQueuedTurnBySelection')({context:scoped});
  const fullPrompt='Edited full queued prompt '.repeat(25);
  await context.lookupHandler('chat.saveQueuedTurnForm')({context:scoped,parameters:{queueTurns:{id:'native-queued-turn',content:fullPrompt,preview:'Truncated preview'}}});
  expect(client.editQueuedTurn).toHaveBeenCalledWith('queue-owner-conversation','native-queued-turn',{content:fullPrompt.trim()});
  expect(client.moveQueuedTurn.mock.invocationCallOrder[0]).toBeLessThan(client.reconcileAgUiConversation.mock.invocationCallOrder[0]);
  expect(client.cancelQueuedTurn).toHaveBeenCalledWith('queue-owner-conversation','native-queued-turn');
  expect(client.reconcileAgUiConversation).toHaveBeenCalledWith('queue-owner-conversation');
  expect(client.moveQueuedTurn).toHaveBeenCalledWith('queue-owner-conversation','native-queued-turn',{direction:'up'});
  expect(client.forceSteerQueuedTurn).toHaveBeenCalledWith('queue-owner-conversation','native-queued-turn');
 });
 it('waits for post-commit reconciliation before updating the authored Queue view',async()=>{
  vi.clearAllMocks();
  let release;
  client.reconcileAgUiConversation.mockImplementationOnce(()=>new Promise(resolve=>{release=resolve;}));
  const context=createFeedContext('queue',{queueTurns:{source:'output.queuedTurns'}},'owner');
  const scoped=context.Context('queueTurns');
  scoped.signals.selection.value={selected:{id:'queued-native-turn'},rowIndex:0};
  const pending=context.lookupHandler('chat.moveQueuedTurnDown')({context:scoped});
  await vi.waitFor(()=>expect(client.reconcileAgUiConversation).toHaveBeenCalledWith('owner'));
  expect(dsTick).not.toHaveBeenCalled();
  release({});
  await pending;
  expect(dsTick).toHaveBeenCalled();
 });

 it('saves bound editor B even when a refresh selects A, and never guesses an absent editor id',async()=>{
  vi.clearAllMocks();
  const context=createFeedContext('queue',{queueTurns:{source:'output.queuedTurns'}},'owner');
  const scoped=context.Context('queueTurns');
  scoped.signals.selection.value={selected:{id:'turn-A'},rowIndex:0};
  const edited='Full prompt for B '.repeat(30);
  scoped.handlers.dataSource.setFormData({values:{id:'turn-B',content:edited}});
  await context.lookupHandler('chat.saveQueuedTurnForm')({context:scoped});
  expect(client.editQueuedTurn).toHaveBeenCalledWith('owner','turn-B',{content:edited.trim()});
  client.editQueuedTurn.mockClear();
  scoped.handlers.dataSource.setFormData({values:{id:'turn-A',content:'Refreshed A'}});
  await context.lookupHandler('chat.saveQueuedTurnForm')({context:scoped,parameters:{queueTurns:{id:'turn-B',content:edited}}});
  expect(client.editQueuedTurn).toHaveBeenCalledWith('owner','turn-B',{content:edited.trim()});
  client.editQueuedTurn.mockClear();
  expect(await context.lookupHandler('chat.saveQueuedTurnForm')({context:scoped,parameters:{queueTurns:{content:edited}}})).toBe(false);
  expect(client.editQueuedTurn).not.toHaveBeenCalled();
  scoped.handlers.dataSource.setFormData({values:{content:edited}});
  expect(await context.lookupHandler('chat.saveQueuedTurnForm')({context:scoped})).toBe(false);
  expect(client.editQueuedTurn).not.toHaveBeenCalled();
 });

});
