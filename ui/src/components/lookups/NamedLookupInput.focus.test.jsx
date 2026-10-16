// @vitest-environment jsdom
import React, {act, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {afterEach, describe, expect, it, vi} from 'vitest';
import NamedLookupInput from './NamedLookupInput.jsx';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;
afterEach(() => {vi.restoreAllMocks();vi.unstubAllGlobals();document.body.innerHTML='';});

describe('named lookup editor focus ownership', () => {
  for (const multiline of [false,true]) it('preserves selection and cleared ID in multiline='+multiline, async () => {
    const pending=new Map();let frameId=0;
    vi.stubGlobal('requestAnimationFrame',callback=>{pending.set(++frameId,callback);return frameId;});
    vi.stubGlobal('cancelAnimationFrame',id=>pending.delete(id));
    vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify({entries:[{
      name:'record',dataSource:'records',dialogId:'record-selector',
      token:{store:'${id}',display:'${name}',resolveInput:'ID'},
    }]}),{headers:{'Content-Type':'application/json'}})));
    const openDialog=vi.fn(async()=>null);
    const context={handlers:{window:{openDialog}}};
    function Fixture(){const [value,setValue]=useState('Inspect @{record:2 "Owned Beta"}.');return <NamedLookupInput value={value} onChange={setValue} context={context} multiline={multiline} autoResolveAuthored={false}/>;}
    const node=document.createElement('div');document.body.append(node);const root=createRoot(node);
    try {
      await act(async()=>{root.render(<Fixture/>);});
      const chip=[...node.querySelectorAll('.bp6-tag,[data-token]')].find(element=>element.textContent==='Owned Beta');
      expect(chip).toBeTruthy();await act(async()=>chip.click());
      const input=node.querySelector('input[data-testid="named-lookup-chip-input-record"],input[data-testid="named-lookup-inline-editor-record"]');
      expect(input.value).toBe('2');
      // A rapid Ctrl+A/Delete or automation fill selects text before the
      // activation frame runs. That frame must not move the caret afterward.
      input.focus();input.setSelectionRange(0,input.value.length);
      await act(async()=>{const frames=[...pending.values()];pending.clear();frames.forEach(callback=>callback(0));});
      expect([input.selectionStart,input.selectionEnd]).toEqual([0,1]);
      await act(async()=>{
        Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(input,'');
        input.dispatchEvent(new Event('input',{bubbles:true}));
      });
      expect(input.value).toBe('');
      const browse=node.querySelector('button[aria-label="Browse record"],button[title="Browse record"]');
      await act(async()=>browse.click());
      expect(openDialog).toHaveBeenCalledTimes(1);
      expect(openDialog.mock.calls[0][0].parameters).toEqual({});
      expect(openDialog.mock.calls[0][0].execution.args[0]).toBe('record-selector');
    } finally {await act(async()=>root.unmount());}
  });
});
