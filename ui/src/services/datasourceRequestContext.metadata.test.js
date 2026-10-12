import {beforeEach, expect, it, vi} from 'vitest';

vi.mock('forge/core', () => ({findMetadataSignal: vi.fn(), getActiveUIBridgeTransportContext: vi.fn(() => null)}));
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

it('forwards one complete detached signed admission bundle instead of mixing it with a later signal', () => {
  const selectionBinding = {actor:{issuer:'issuer',subject:'actor',tenantId:'tenant',accountId:'account'},instanceId:'original',parametersDigest:'digest',token:'signed-binding',extra:{preserved:true}};
  const resource={uri:'window://example/Records',contentFingerprint:'original',authorityBinding:'original-actor'};
  const resourceTarget={selectionToken:'original-target',dependencyPins:{rows:{contentFingerprint:'original-rows'}},executionProof:{token:'original-proof'}};
  const openedWindowParameters={EntityID:['9007199254740993'],nested:{selection:{region:'original'}},Limit:7};
  const inlineMetadata={resource,resourceTarget,selectionBinding,view:{content:{id:'original'}}};
  findMetadataSignal.mockReturnValue({peek:()=>({resource:{...resource,contentFingerprint:'fresh-other'},resourceTarget:{selectionToken:'fresh-other'},selectionBinding:{...selectionBinding,token:'fresh-other'}})});
  const body={inputs:{EntityID:['user-input']}};
  prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/rows/fetch',body,windowState:{windowId:'same-ui-frame',parameters:{EntityID:['edited']},openedWindowParameters,inlineMetadata}});
  expect(body).toMatchObject({resource,target:resourceTarget,selectionBinding,windowParameters:openedWindowParameters,inputs:{EntityID:['user-input']}});
  expect(body.selectionBinding).not.toBe(selectionBinding);
  expect(body.selectionBinding.actor).not.toBe(selectionBinding.actor);
  expect(body.target.dependencyPins).not.toBe(resourceTarget.dependencyPins);
  expect(body.windowParameters.nested.selection).not.toBe(openedWindowParameters.nested.selection);
  body.selectionBinding.actor.subject='mutated';body.target.executionProof.token='mutated';body.windowParameters.nested.selection.region='mutated';
  expect(selectionBinding.actor.subject).toBe('actor');expect(resourceTarget.executionProof.token).toBe('original-proof');expect(openedWindowParameters.nested.selection.region).toBe('original');
});

it('forwards a complete top-level signed window without changing datasource inputs or proof fields', () => {
  const selectionBinding={actor:{issuer:'issuer',subject:'actor',tenantId:'tenant',accountId:'account'},instanceId:'opened',parametersDigest:'digest',token:'signed',opaque:{numbers:[1,2]}};
  const resource={uri:'window://example/Records',contentFingerprint:'opened'};
  const resourceTarget={selectionToken:'opened-target',executionProof:{token:'proof'}};
  const parameters={EntityID:['9007199254740993'],nested:{id:17}};
  const body={inputs:{EntityID:['different-input']}};
  prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/rows/fetch',body,windowState:{resource,resourceTarget,selectionBinding,parameters}});
  expect(body).toEqual({inputs:{EntityID:['different-input']},resource,target:resourceTarget,selectionBinding,windowParameters:parameters});
  expect(body.selectionBinding.opaque).not.toBe(selectionBinding.opaque);
  expect(body.windowParameters.nested).not.toBe(parameters.nested);
});
