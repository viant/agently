import {beforeEach, expect, it, vi} from 'vitest';

vi.mock('forge/core', () => ({findMetadataSignal: vi.fn()}));
import {findMetadataSignal} from 'forge/core';
import {prepareAgentlyDataConnectorRequest} from './datasourceRequestContext.js';

beforeEach(() => findMetadataSignal.mockReset());

it('carries the opened metadata signal pin and dependency target for a delegated datasource', () => {
  const resource = {providerIdentity: 'owned-provider', uri: 'window://example/StudioRecords', kind: 'working', contentFingerprint: 'window', authorityBinding: 'actor'};
  const resourceTarget = {platform: 'web', selectionToken: 'signed-target', dependencyPins: {records: {providerIdentity: 'owned-provider', uri: 'datasource://example/StudioRecords/records', contentFingerprint: 'descriptor'}}, executionProof: {token: 'provider-proof'}};
  findMetadataSignal.mockImplementation(id => id === 'studio-instance' ? {peek: () => ({resource, resourceTarget})} : null);
  const body = {inputs: {limit: 10}};
  prepareAgentlyDataConnectorRequest({url: '/v1/api/datasources/records/fetch', body, windowState: {windowId: 'studio-instance', windowKey: resource.uri}});
  expect(body.resource).toEqual(resource);
  expect(body.target).toEqual(resourceTarget);
  expect(body.resource).not.toBe(resource);
  expect(body.target).not.toBe(resourceTarget);
  expect(body.inputs).toEqual({limit: 10});
});

it('uses the current instance metadata instead of an earlier descriptor pin', () => {
  const current = {uri: 'window://example/StudioRecords', contentFingerprint: 'current'};
  findMetadataSignal.mockReturnValue({peek: () => ({resource: current, resourceTarget: {selectionToken: 'current-target'}})});
  const body = {inputs: {}};
  prepareAgentlyDataConnectorRequest({url: '/v1/api/datasources/records/fetch', body, windowState: {windowId: 'studio-instance', metadata: {resource: {...current, contentFingerprint: 'old'}}}});
  expect(body.resource).toEqual(current);
  expect(body.target.selectionToken).toBe('current-target');
});

it('does not infer a resource from a datasource alias or a different window instance', () => {
  findMetadataSignal.mockImplementation(id => id === 'different-instance' ? {peek: () => ({resource: {uri: 'window://example/Other'}})} : null);
  const body = {inputs: {}};
  prepareAgentlyDataConnectorRequest({url: '/v1/api/datasources/records/fetch', body, windowState: {windowId: 'unknown-instance', windowKey: 'window://example/StudioRecords'}});
  expect(body).toEqual({inputs: {}});
  expect(findMetadataSignal).toHaveBeenCalledWith('unknown-instance');
});

it('does not resolve or transport datasource proofs on unrelated requests', () => {
  const body = {inputs: {}};
  prepareAgentlyDataConnectorRequest({url: '/v1/api/agently/forge/window/StudioRecords', body, windowState: {windowId: 'studio-instance'}});
  expect(findMetadataSignal).not.toHaveBeenCalled();
  expect(body).toEqual({inputs: {}});
});
