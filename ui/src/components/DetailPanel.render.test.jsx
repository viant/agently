// @vitest-environment jsdom
import React, {act} from 'react';
import {createRoot} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {Dialog} from '@blueprintjs/core';
import DetailPanel from './DetailPanel.jsx';
import {client} from '../services/agentlyClient';
vi.mock('@uiw/react-codemirror',()=>({default:({value})=><pre data-testid="payload-code">{value}</pre>}));
vi.mock('../services/agentlyClient',()=>({client:{getTranscript:vi.fn(async()=>({conversation:{turns:[]}})),getPayload:vi.fn()}}));
globalThis.IS_REACT_ACT_ENVIRONMENT=true;
let root,host;
beforeEach(()=>{vi.clearAllMocks();window.history.replaceState(null,'','/conversation/inspector-test');});
afterEach(async()=>{if(root)await act(async()=>root.unmount());host?.remove();root=null;});
async function mount(step){if(!root){host=document.createElement('div');document.body.append(host);root=createRoot(host);}await act(async()=>root.render(<Dialog isOpen aria-labelledby="test-step-heading"><DetailPanel toolCall={step} headingId="test-step-heading" appearance="document" onClose={()=>{}}/></Dialog>));}
const tab=name=>[...document.querySelectorAll('[role="tab"]')].find(node=>node.textContent===name);
const press=async name=>act(async()=>tab(name).click());
const read=()=>document.querySelector('[role="tabpanel"]')?.textContent;
const model={id:'model',kind:'model',model:'Example model',provider:'Example provider',status:'completed',requestPayload:{scope:'one'},responsePayload:{result:'two'},streamPayload:[],providerRequestPayload:{raw:'three'},providerResponsePayload:{raw:'four'}};

it('shows all model payloads in one inspector and preserves exact copied data',async()=>{
 const writeText=vi.fn(async()=>{});Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText}});
 await mount(model);
 expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(1);
 expect(document.querySelectorAll('[role="tab"]')).toHaveLength(6);
 for(const [name,value] of [['Request',model.requestPayload],['Response',model.responsePayload],['Stream',[]],['Provider request',model.providerRequestPayload],['Provider response',model.providerResponsePayload]]){
  await press(name);expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(1);
  expect(document.querySelector('[data-testid="payload-code"]').textContent).toBe(JSON.stringify(value,null,2));
 }
 await act(async()=>[...document.querySelectorAll('button')].find(node=>node.textContent==='Copy').click());
 expect(writeText).toHaveBeenCalledWith(JSON.stringify(model.providerResponsePayload,null,2));
 await press('Overview');expect(read()).toContain('Example provider');
 expect(document.querySelector('[aria-label="Close execution details"]')).not.toBeNull();
});
it('keeps loading, unavailable and retryable failures in the same panel',async()=>{
 let finish;client.getPayload.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
 await mount({...model,requestPayload:null,requestPayloadId:'loading-inspector'});
 await press('Request');expect(read()).toContain('Loading request');
 await act(async()=>finish({data:new TextEncoder().encode('{"loaded":true}'),contentType:'application/json'}));
 expect(read()).toContain('loaded');
 await mount({...model,id:'missing',requestPayload:null});await press('Request');expect(read()).toContain('No data available');
 client.getPayload.mockRejectedValueOnce(new Error('Read failed')).mockResolvedValueOnce({data:new TextEncoder().encode('false'),contentType:'application/json'});
 await mount({...model,id:'error',requestPayload:null,requestPayloadId:'retry-inspector'});await press('Request');expect(read()).toContain('Read failed');
 await act(async()=>[...document.querySelectorAll('button')].find(node=>node.textContent==='Try again').click());
 expect(document.querySelector('[data-testid="payload-code"]').textContent).toBe('false');
 expect(document.querySelectorAll('[role="dialog"]')).toHaveLength(1);
});
it('ignores a slow previous payload and uses immediate shared keyboard tab selection',async()=>{
 let finish;client.getPayload.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
 await mount({...model,requestPayload:null,requestPayloadId:'race-inspector'});
 await press('Request');await press('Response');
 await act(async()=>finish({data:new TextEncoder().encode('{"old":true}'),contentType:'application/json'}));
 expect(tab('Response').getAttribute('aria-selected')).toBe('true');expect(read()).toContain('two');expect(read()).not.toContain('old');
 await act(async()=>tab('Response').dispatchEvent(new KeyboardEvent('keydown',{key:'ArrowRight',bubbles:true})));
 expect(tab('Stream').getAttribute('aria-selected')).toBe('true');
});
it('limits tool views and preserves false and zero payloads',async()=>{
 await mount({id:'tool',kind:'tool',toolName:'example/read',requestPayload:0,responsePayload:false});
 expect(document.querySelectorAll('[role="tab"]')).toHaveLength(3);
 await press('Request');expect(document.querySelector('[data-testid="payload-code"]').textContent).toBe('0');
 await press('Response');expect(document.querySelector('[data-testid="payload-code"]').textContent).toBe('false');
});
