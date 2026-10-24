// @vitest-environment jsdom
import React from 'react';
import {afterEach, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';

const {services, runtime} = vi.hoisted(() => ({services: {reportStore: {getReport: vi.fn()}}, runtime: vi.fn()}));
vi.mock('forge/core', () => ({useSetting: () => ({services})}));
vi.mock('forge/components', () => ({ReportResourceRuntime: props => {runtime(props); return <div>Shared report runtime</div>;}}));
import ReportWindow from './ReportWindow.jsx';

const pin = {uri: 'report://ar/overview', providerIdentity: 'producer-native', kind: 'working', contentFingerprint: 'bytes', authorityBinding: 'opaque', validUntil: '2030-01-01T00:00:00Z'};
const entry = {windowKey: pin.uri, resourceRef: {uri: pin.uri}, windowTitle: 'Overview', parameters: {AdvertiserIds: [42]}};
afterEach(() => {cleanup(); vi.clearAllMocks();});

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
  services.reportStore.getReport.mockRejectedValueOnce(new Error('Producer denied report')).mockResolvedValueOnce({resource: pin});
  render(<ReportWindow window={entry}/>);
  await screen.findByText('Producer denied report');
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
