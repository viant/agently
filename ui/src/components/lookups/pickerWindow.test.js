import {describe,expect,it,vi} from 'vitest';
import React from 'react';
import WindowContent from 'forge/components/WindowContent.jsx';
import {openWindowPickerDialog} from 'forge/components/windowPickerDialog.js';
import {windowMetadataEndpoint} from 'forge/components/windowMetadataEndpoint.js';

describe('canonical lookup picker window transport',()=>{
 it('compiles the normal window component and forwards exact owning metadata refs',()=>{
  expect(React.isValidElement(React.createElement(WindowContent,{window:{windowId:'picker',windowKey:'picker'}}))).toBe(true);
  const resourceRef={uri:'window://external/picker',revision:'working'};
  const uri=new URL(windowMetadataEndpoint('/window',resourceRef.uri,{resourceRef,providerIdentity:'source'}),'http://fixture.local');
  expect(JSON.parse(uri.searchParams.get('resourceRef'))).toEqual(resourceRef);
  expect(uri.searchParams.get('providerIdentity')).toBe('source');
 });
 it('returns the existing authored dialog selection and closes cancellation',async()=>{
  const selected={adOrderId:42,adOrderName:'Original'};
  const handlers={openDialog:vi.fn().mockResolvedValue(selected),commit:vi.fn(),closeWindow:vi.fn()};
  const context={handlers:{window:handlers}};
  const window={pickerDialogId:'picker',pickerParameters:[{name:'AdOrderId'}],parameters:{AdOrderId:42}};
  await openWindowPickerDialog(window,{dialogs:[{id:'picker'}]},context);
  expect(handlers.openDialog).toHaveBeenCalledWith({execution:{args:['picker',{awaitResult:true,parameters:[{name:'AdOrderId'}]}]},parameters:{AdOrderId:42},context});
  expect(handlers.commit).toHaveBeenCalledWith({payload:selected,context});
  handlers.openDialog.mockResolvedValue({status:'canceled',canceled:true});
  await openWindowPickerDialog(window,{dialogs:[{id:'picker'}]},context);expect(handlers.closeWindow).toHaveBeenCalledTimes(1);
  await expect(openWindowPickerDialog(window,{dialogs:[{id:'unrelated'}]},context)).rejects.toThrow(/owning window bundle/);
 });
});
