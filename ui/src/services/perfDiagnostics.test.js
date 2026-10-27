// @vitest-environment jsdom
import {beforeEach,expect,it,vi} from 'vitest';
import {createDiagnosticPublishedWindowOpener,installPerfDiagnostics,registerReadyDiagnosticPublishedWindowOpener} from './perfDiagnostics.js';

beforeEach(() => {document.body.innerHTML='';});

it('is absent without the explicit diagnostics query', () => {
  const fetch = vi.fn();
  const window = {location:{href:'http://fixture/'},fetch};
  expect(installPerfDiagnostics({window,document,performance})).toBeNull();
  expect(window.fetch).toBe(fetch);
  expect(document.querySelector('#agently-timing-diagnostics')).toBeNull();
});

it('records a real click, network completion, and committed table rows without row values', async () => {
  const fetch = vi.fn().mockResolvedValue({status:200});
  const window = {location:{href:'http://fixture/?diagnostics=timing'},fetch};
  const trace = installPerfDiagnostics({window,document,performance});
  document.dispatchEvent(new Event('DOMContentLoaded'));
  expect(document.querySelector('#agently-timing-diagnostics summary').textContent).toBe('Timing diagnostics');
  expect(document.querySelector('#agently-timing-diagnostics-published-opener').hidden).toBe(true);
  document.querySelector('#agently-timing-arm').click();
  const button = document.createElement('button');
  button.textContent = 'Connectors';
  document.body.append(button);
  button.click();
  await window.fetch('/v1/api/agently/forge/window/connectors?resourceRef=private');
  await window.fetch('/v1/api/datasources/connectors/fetch?cohort=1&windowId=private-window',{method:'POST',body:'private-request'});
  const section = document.createElement('section');
  section.className = 'app-summary-workspace';
  section.innerHTML = '<span class="app-summary-workspace-title">Connectors</span><div class="app-workspace-renderer" role="tabpanel" hidden aria-hidden="true"><div class="basic-table-wrapper"><table><tbody><tr class="row"><td>hidden-row</td></tr></tbody></table></div></div><div class="app-workspace-renderer" role="tabpanel" aria-hidden="false"><div class="basic-table-wrapper"><table><tbody></tbody></table></div></div>';
  document.body.append(section);
  await new Promise(resolve=>setTimeout(resolve,0));
  expect(trace.snapshot().events.some(event=>event.kind==='rows-dom')).toBe(false);
  section.querySelector('[role="tabpanel"]:not([hidden]) tbody').innerHTML='<tr class="row"><td>private-row</td></tr>';
  await new Promise(resolve=>setTimeout(resolve,0));
  const result = trace.snapshot();
  expect(result.label).toBe('Connectors');
  expect(result.openToMetadataEndMs).not.toBeNull();
  expect(result.openToDataEndMs).not.toBeNull();
  expect(result.events.find(event=>event.category==='data')).toMatchObject({cohortRequest:true,windowIdPresent:true});
  expect(result.openToRowsDOMMs).not.toBeNull();
  expect(result.events.find(event=>event.kind==='rows-dom').rowCount).toBe(1);
  expect(document.querySelector('#agently-timing-output').textContent).not.toMatch(/private-row|private-request|private/);
  document.querySelector('#agently-timing-server').click();
  await window.fetch('/v1/api/datasources/connectors/fetch?cohort=1',{method:'POST'});
  expect(fetch.mock.calls.at(-1)[0]).toBe('/v1/api/datasources/connectors/fetch?cohort=1&perf=1');
});

it('routes a diagnostic locator through the ordinary workspace opener without adding data authority',()=>{
  const open=vi.fn(),activate=vi.fn();
  const invoke=createDiagnosticPublishedWindowOpener({open,activate,parentKey:'chat/new',conversationId:'conv'});
  invoke({providerIdentity:'unknown-provider',uri:'window://dspui/orderList',title:'Order List'});
  expect(open).toHaveBeenCalledWith('window://dspui/orderList','Order List',[],{
    parameters:{},conversationId:'conv',parentKey:'chat/new',presentation:'hosted',region:'chat.top',
    providerIdentity:'unknown-provider',resourceRef:{uri:'window://dspui/orderList'},
  });
  expect(activate).toHaveBeenCalledOnce();
  const denied=createDiagnosticPublishedWindowOpener({open,activate,parentKey:'chat/new',isReady:()=>false});
  expect(()=>denied({providerIdentity:'unknown-provider',uri:'window://dspui/other',title:'Other'})).toThrow(/transport/);
  expect(open).toHaveBeenCalledOnce();
});

it('does not register the opener before navigation and UI transport restore complete',async()=>{
  let release;
  const waitReady=()=>new Promise(resolve=>{release=resolve;});
  const register=vi.fn(()=>vi.fn());
  const pending=registerReadyDiagnosticPublishedWindowOpener({waitReady,getTransport:()=>({sessionId:'negotiated'}),onOpen:vi.fn(),register});
  expect(register).not.toHaveBeenCalled();
  release(true);
  await pending;
  expect(register).toHaveBeenCalledOnce();
  const unavailable=vi.fn();
  await registerReadyDiagnosticPublishedWindowOpener({waitReady:async()=>true,getTransport:()=>null,onOpen:vi.fn(),register:unavailable});
  expect(unavailable).not.toHaveBeenCalled();
});
