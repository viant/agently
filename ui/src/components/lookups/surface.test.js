import {describe,expect,it} from 'vitest';
import {lookupSurfaceInvocation} from './surface.js';

describe('lookup picker source routing',()=>{
 const entry={dialogId:'picker',pickerResource:{uri:'window://external/picker'},providerIdentity:'external'};
 it('retains an explicitly bundled original dialog',()=>{
  const context={metadata:{resource:{uri:'window://external/order',providerIdentity:'external'},dialogs:[{id:'picker'}]}};
  expect(lookupSurfaceInvocation(entry,context).handler).toBe('openDialog');
 });
 it('opens the canonical owning picker instead of an ambient matching dialog',()=>{
  const context={metadata:{resource:{uri:'window://workspace/order',providerIdentity:'local'},dialogs:[{id:'picker'}]}};
  const invocation=lookupSurfaceInvocation(entry,context,[{name:'id'}]);
  expect(invocation.handler).toBe('openWindow');expect(invocation.args[0]).toBe('window://external/picker');
  expect(invocation.args[4]).toMatchObject({resourceRef:entry.pickerResource,providerIdentity:'external',pickerDialogId:'picker',awaitResult:true,modal:true,parameters:[{name:'id'}]});
 });
 it('supports a canonical picker resource without legacy IDs',()=>{
  const value=lookupSurfaceInvocation({pickerResource:entry.pickerResource,providerIdentity:'external'},{});
  expect(value.handler).toBe('openWindow');expect(value.args[0]).toBe('window://external/picker');
 });
 it('keeps legacy bundled dialog IDs and rejects missing canonical picker',()=>{
  expect(lookupSurfaceInvocation({dialogId:'original'},{}).args[0]).toBe('original');
  expect(()=>lookupSurfaceInvocation({},{})).toThrow(/unavailable/);
 });
});
