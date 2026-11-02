// @vitest-environment jsdom
import React from 'react';
import {afterEach, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';

const {services, runtime} = vi.hoisted(() => ({services: {reportStore: {getReport: vi.fn()}}, runtime: vi.fn()}));
vi.mock('forge/core', () => ({useSetting: () => ({services})}));
vi.mock('forge/components', async () => ({ReportFilterBar:(await import('forge/components/dashboard/ReportFilterBar.jsx')).default, ReportResourceRuntime: props => {runtime(props); return <div>Shared report runtime</div>;}}));
import ReportWindow from './ReportWindow.jsx';
import 'forge/packs/blueprint';

const pin = {uri: 'report://ar/overview', providerIdentity: 'producer-native', kind: 'working', contentFingerprint: 'bytes', authorityBinding: 'opaque', validUntil: '2030-01-01T00:00:00Z'};
const entry = {windowKey: pin.uri, resourceRef: {uri: pin.uri}, windowTitle: 'Overview', parameters: {AdvertiserIds: [42]}};
afterEach(() => {cleanup(); vi.clearAllMocks();});

it('uses the authored report title while required inputs are still incomplete', async () => {
  services.reportStore.getReport.mockResolvedValue({resource:pin,title:'314',resourceDefinition:{
    reportDocument:{title:'Service coverage'},
    builderDefinition:{reportBuilder:{predicates:[{id:'region',label:'Region',kind:'value',required:true,paramPath:'filters.region'}]}},
  }});
  render(<ReportWindow window={{...entry,parameters:{}}}/>);
  await screen.findByLabelText('Region');
  expect(screen.getByText(/Complete these inputs/)).toBeTruthy();
  expect(runtime.mock.calls.at(-1)[0].container.parameters.reportTitle).toBe('Service coverage');
  expect(runtime.mock.calls.at(-1)[0].container.parameters.reportCapabilities.run).toBe(false);
});

it('requires both declared date endpoints before enabling report execution', async () => {
  services.reportStore.getReport.mockResolvedValue({resource: pin, resourceDefinition:{builderDefinition:{reportBuilder:{predicates:[
    {id:'window',label:'Date range',kind:'dateRange',required:true,startParamPath:'filters.start',endParamPath:'filters.end',prefill:{start:'from',end:'to'}},
  ]}}}});
  render(<ReportWindow window={{...entry,parameters:{}}}/>);
  const start=await screen.findByLabelText('Date range from');
  expect(start.getAttribute('aria-required')).toBe('true');
  expect(screen.getByText(/Complete these inputs/).textContent).toContain('Date range');
  expect(runtime.mock.calls.at(-1)[0].container.parameters.reportCapabilities.run).toBe(false);
  fireEvent.change(start,{target:{value:'2026-01-02'}});
  expect(runtime.mock.calls.at(-1)[0].container.parameters.reportCapabilities.run).toBe(false);
  fireEvent.change(screen.getByLabelText('Date range to'),{target:{value:'2026-01-03'}});
  expect(runtime.mock.calls.at(-1)[0].container.parameters.reportCapabilities.run).toBe(false);
  expect(runtime.mock.calls.at(-1)[0].container.parameters.prefill).toEqual({});
  fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
  expect(runtime.mock.calls.at(-1)[0].container.parameters.prefill).toEqual({from:'2026-01-02',to:'2026-01-03'});
  expect(screen.queryByText(/Complete these inputs/)).toBeNull();
  fireEvent.change(start,{target:{value:''}});
  expect(runtime.mock.calls.at(-1)[0].container.parameters.reportCapabilities.run).toBe(true);
  expect(screen.getByRole('button',{name:'Apply filters'}).disabled).toBe(true);
});

it('resolves through the existing host and mounts the shared runtime with its opaque pin and current parameters', async () => {
  services.reportStore.getReport.mockResolvedValue({resource: pin, title: 'Producer overview'});
  const view = render(<ReportWindow window={entry}/>);
  await screen.findByText('Shared report runtime');
  expect(services.reportStore.getReport).toHaveBeenCalledWith({resource: entry.resourceRef});
  let props = runtime.mock.calls.at(-1)[0];
  expect(props.container.parameters.reportResource).toBe(pin);
  expect(props.container.parameters.prefill).toEqual({AdvertiserIds: [42]});
  expect(props.context.handlers).toBe(services);
  view.rerender(<ReportWindow window={{...entry, parameters: {AdvertiserIds: [0]}}}/>);
  props = runtime.mock.calls.at(-1)[0];
  expect(props.container.parameters.prefill).toEqual({AdvertiserIds: [0]});
  expect(services.reportStore.getReport).toHaveBeenCalledOnce();
});

it('shows a backend failure and retries the same selected resource', async () => {
  services.reportStore.getReport.mockRejectedValueOnce(new Error('automatic MCP protocol attempts failed: internal server details')).mockResolvedValueOnce({resource: pin});
  render(<ReportWindow window={entry}/>);
  await screen.findByText('This report could not be loaded. Please try again.');
  expect(screen.queryByText(/automatic MCP protocol/)).toBeNull();
  expect(runtime).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText('Retry'));
  await screen.findByText('Shared report runtime');
  expect(services.reportStore.getReport).toHaveBeenCalledTimes(2);
});

it('ignores a late response after selecting another report', async () => {
  let resolveFirst;
  services.reportStore.getReport.mockImplementationOnce(() => new Promise(resolve => {resolveFirst = resolve;}))
    .mockResolvedValueOnce({resource: {...pin, uri: 'report://ar/geo'}});
  const view = render(<ReportWindow window={entry}/>);
  await waitFor(() => expect(services.reportStore.getReport).toHaveBeenCalledOnce());
  view.rerender(<ReportWindow window={{...entry, windowKey: 'report://ar/geo', resourceRef: {uri: 'report://ar/geo'}}}/>);
  await screen.findByText('Shared report runtime');
  resolveFirst({resource: pin});
  await waitFor(() => expect(runtime.mock.calls.at(-1)[0].container.parameters.reportResource.uri).toBe('report://ar/geo'));
});

it('renders declared inputs, preserves empty/zero/false values, and resets edits on parent parameter changes', async () => {
  services.reportStore.getReport.mockResolvedValue({resource: pin, resourceDefinition: {builderDefinition: {reportBuilder: {predicates: [
    {id: 'entityId', label: 'Entity', kind: 'number', paramPath: 'filters.entityId'},
    {id: 'active', label: 'Active', kind: 'boolean', paramPath: 'filters.active'},
  ]}}}});
  const initial = {...entry, parameters: {entityId: 9, active: true}};
  const view = render(<ReportWindow window={initial}/>);
  fireEvent.change(await screen.findByLabelText('Entity'), {target: {value: '0'}});
  fireEvent.click(screen.getByLabelText('Active'));
  expect(runtime.mock.calls.at(-1)[0].container.parameters.prefill).toEqual({entityId: 9, active: true});
  fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
  expect(runtime.mock.calls.at(-1)[0].container.parameters.prefill).toEqual({entityId: 0, active: false});
  fireEvent.change(screen.getByLabelText('Entity'), {target: {value: ''}});
  fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
  expect(runtime.mock.calls.at(-1)[0].container.parameters.prefill.entityId).toBe('');
  view.rerender(<ReportWindow window={{...initial, parameters: {entityId: 4, active: false}}}/>);
  expect(screen.getByLabelText('Entity').value).toBe('4');
  expect(runtime.mock.calls.at(-1)[0].container.parameters.prefill).toEqual({entityId: 4, active: false});
});

it('restores a logical report bookmark without treating it as a window',async()=>{
 services.reportStore.getReport.mockResolvedValue({resource:pin});
 render(<ReportWindow window={{windowKey:pin.uri,windowTitle:'Overview',parameters:{}}}/>);
 await screen.findByText('Shared report runtime');
 expect(services.reportStore.getReport).toHaveBeenCalledWith({resource:{uri:pin.uri}});
});

it('opts into opening execution with authored defaults and excludes the control from datasource inputs', async () => {
 services.reportStore.getReport.mockResolvedValue({resource:pin,resourceDefinition:{metadata:{executeOnOpen:true},builderDefinition:{reportBuilder:{predicates:[{id:'region',label:'Region',required:true,default:'west'}]}}}});
 const view=render(<ReportWindow window={{...entry,parameters:{executeOnOpen:true}}}/>);
 const field=await screen.findByLabelText('Region');
 const read=()=>runtime.mock.calls.at(-1)[0].container.parameters;
 expect(read().executeOnOpen).toBe(true);
 expect(read().prefill).toEqual({region:'west'});
 fireEvent.change(field,{target:{value:''}});
 expect(read().prefill).toEqual({region:'west'});
 expect(read().reportCapabilities.run).toBe(true);
 expect(screen.getByRole('button',{name:'Apply filters'}).disabled).toBe(true);
 view.rerender(<ReportWindow window={{...entry,parameters:{executeOnOpen:false}}}/>);
 expect(read().executeOnOpen).toBe(false);
});
it('does not auto-execute without complete required initial inputs or explicit opt-in', async () => {
 services.reportStore.getReport.mockResolvedValue({resource:pin,resourceDefinition:{metadata:{executeOnOpen:true},builderDefinition:{reportBuilder:{predicates:[{id:'region',label:'Region',required:true}]}}}});
 render(<ReportWindow window={{...entry,parameters:{}}}/>);
 await screen.findByLabelText('Region');
 expect(runtime.mock.calls.at(-1)[0].container.parameters.executeOnOpen).toBe(false);
 fireEvent.change(screen.getByLabelText('Region'),{target:{value:'west'}});
 expect(runtime.mock.calls.at(-1)[0].container.parameters.executeOnOpen).toBe(false);
 fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
 expect(runtime.mock.calls.at(-1)[0].container.parameters.executeOnOpen).toBe(true);
});

it('retains published calendar presets and waits for authoritative defaults before allowing Apply',async()=>{
  vi.useFakeTimers({toFake:['Date']});vi.setSystemTime(new Date('2026-01-01T07:59:00Z'));
  const calendar={calendar:{dataSourceRef:'calendar',input:{name:'ids',parameter:'entity',array:true},identityField:'id',timeZoneField:'zone'},default:'completed',options:[{value:'completed',label:'Completed days',range:{kind:'relativeDays',startOffset:-7,endOffset:-1}},{value:'custom',label:'Custom'}]};
  services.reportStore.getReport.mockResolvedValue({resource:pin,resourceDefinition:{dataSources:{calendar:{cache:{enabled:false}}},builderDefinition:{reportBuilder:{predicates:[{id:'period',label:'Period',kind:'dateRange',prefill:{start:'from',end:'to'},datePresets:calendar}]}}}});
  let finish;const fetchMock=vi.fn(()=>new Promise(resolve=>{finish=resolve;}));vi.stubGlobal('fetch',fetchMock);
  try{
    render(<ReportWindow window={{...entry,parameters:{entity:17}}}/>);
    await waitFor(()=>expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(screen.getByRole('button',{name:'Apply filters'}).disabled).toBe(true);
    expect(runtime.mock.calls.at(-1)[0].container.parameters.reportCapabilities.run).toBe(false);
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({owner:{uri:pin.uri},inputs:{ids:[17]}});
    finish({ok:true,json:async()=>({rows:[{id:17,zone:'America/Los_Angeles'}]})});
    await waitFor(()=>expect(screen.getByLabelText('Period from').value).toBe('2025-12-24'));
    expect(screen.getByLabelText('Period to').value).toBe('2025-12-30');
    expect(screen.getByRole('button',{name:'Apply filters'}).disabled).toBe(false);
    fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
    expect(runtime.mock.calls.at(-1)[0].container.parameters.prefill).toEqual({entity:17,from:'2025-12-24',to:'2025-12-30'});
  }finally{vi.useRealTimers();vi.unstubAllGlobals();}
});

it('failed calendar metadata blocks default execution and a deliberate manual pair recovers',async()=>{
  const datePresets={calendar:{dataSourceRef:'calendar',input:{name:'ids',parameter:'entity',array:true},identityField:'id',timeZoneField:'zone'},default:'yesterday',options:[{value:'yesterday',label:'Yesterday'},{value:'custom',label:'Custom'}]};
  services.reportStore.getReport.mockResolvedValue({resource:pin,resourceDefinition:{dataSources:{calendar:{cache:{enabled:false}}},builderDefinition:{reportBuilder:{predicates:[{id:'period',label:'Period',kind:'dateRange',prefill:{start:'from',end:'to'},datePresets}]}}}});
  vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:true,json:async()=>({rows:[{id:18,zone:'UTC'}]})}));
  try{
    render(<ReportWindow window={{...entry,parameters:{entity:17}}}/>);
    await screen.findByText(/Calendar timezone is unavailable/);
    expect(screen.getByRole('button',{name:'Apply filters'}).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText('Period from'),{target:{value:'2026-01-02'}});
    fireEvent.change(screen.getByLabelText('Period to'),{target:{value:'2026-01-03'}});
    expect(screen.getByRole('button',{name:'Apply filters'}).disabled).toBe(false);
    fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
    expect(runtime.mock.calls.at(-1)[0].container.parameters.prefill).toEqual({entity:17,from:'2026-01-02',to:'2026-01-03'});
  }finally{vi.unstubAllGlobals();}
});
