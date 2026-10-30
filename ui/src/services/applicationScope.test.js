import {describe,it,expect,vi} from 'vitest';
vi.mock('forge/core', () => ({activeWindows:{peek:()=>[]}, removeWindow:vi.fn()}));
import {createApplicationScope} from './applicationScope';
describe('application scope',()=>{
 const app={id:'analytics',providerIdentity:'source',scope:{keys:['entityId','entityName']}};
 function setup(){
  let opened=[{windowId:'selector'},{windowId:'report',parameters:{entityId:1}},{windowId:'child',parentKey:'selector',parameters:{entityId:1}},{windowId:'other',parameters:{entityId:1}}];
  const close=vi.fn(id=>{opened=opened.filter(w=>w.windowId!==id);});
  const store=createApplicationScope({windows:()=>opened,closeWindow:close});const other={...app,id:'operations'};
  store.configure('user-one',[app,other]);store.register(opened[0],app,{});store.register(opened[1],app,{parameterBindings:{record:{from:'entityId'}}});store.register(opened[3],other,{});
  return {store,close,context:{identity:{windowId:'selector'}},other};
 }
 it('retains scope across focus and invalidates only dependent views without replay',()=>{
  const {store,close,context,other}=setup();expect(store.set(context,{entityId:2,entityName:'Second'})).toBe(true);
  expect(close.mock.calls.flat()).toEqual(['report','child']);
  expect(store.inputs(app,{entityId:99,local:'keep'})).toEqual({entityId:2,entityName:'Second',local:'keep'});
  expect(store.inputs(other,{entityId:99})).toEqual({});expect(store.get(context)).toEqual({entityId:2,entityName:'Second'});
  close.mockClear();expect(store.set(context,{entityId:2})).toBe(true);expect(close).not.toHaveBeenCalled();
 });
 it('rejects undeclared updates atomically and resets between authenticated sessions',()=>{
  const {store,context}=setup();store.set(context,{entityId:2});expect(()=>store.set(context,{entityId:3,unexpected:1})).toThrow('Undeclared');expect(store.get(context)).toEqual({entityId:2});
  expect(()=>store.set({identity:{windowId:'unknown'}},{entityId:3})).toThrow('unavailable');store.configure('user-two',[app]);expect(store.get(context)).toEqual({});expect(store.inputs(app)).toEqual({});
 });
 it('isolates providers and clears explicitly without borrowing focused scope',()=>{
  const {store,context}=setup();store.set(context,{entityId:[1],entityName:'First'});const snapshot=store.get(context);snapshot.entityId.push(2);expect(store.get(context).entityId).toEqual([1]);
  expect(store.inputs({...app,providerIdentity:'different'})).toEqual({});store.set(context,{entityId:null,entityName:null});expect(store.inputs(app,{entityId:8})).toEqual({});
 });
});
