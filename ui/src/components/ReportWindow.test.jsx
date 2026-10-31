// @vitest-environment jsdom
import React from 'react';
import {afterEach, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';

const {services, runtime} = vi.hoisted(() => ({services: {reportStore: {getReport: vi.fn()}}, runtime: vi.fn()}));
vi.mock('forge/core', () => ({useSetting: () => ({services})}));
vi.mock('forge/components', async () => ({ReportFilterBar:(await import('forge/components/dashboard/ReportFilterBar.jsx')).default, ReportResourceRuntime: props => {runtime(props); return <div>Shared report runtime</div>;}}));
import ReportWindow from './ReportWindow.jsx';

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
  expect(runtime.mock.calls.at(-1)[0].container.parameters.reportCapabilities.run).toBe(true);
  expect(runtime.mock.calls.at(-1)[0].container.parameters.prefill).toEqual({});
  fireEvent.click(screen.getByRole('button',{name:'Apply filters'}));
  expect(runtime.mock.calls.at(-1)[0].container.parameters.prefill).toEqual({from:'2026-01-02',to:'2026-01-03'});
  expect(screen.queryByText(/Complete these inputs/)).toBeNull();
  fireEvent.change(start,{target:{value:''}});
  expect(runtime.mock.calls.at(-1)[0].container.parameters.reportCapabilities.run).toBe(false);
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
