import {describe, expect, it} from 'vitest';
import {prepareAgentlyDataConnectorRequest} from './datasourceRequestContext.js';

const windowState = {
  windowId: 'record-85141',
  conversationId: 'conv-1',
  parameters: {RecordId: [85141]},
};

describe('prepareAgentlyDataConnectorRequest', () => {
  it('keeps complete metadata fetch unchanged', () => {
    const queryParams = new URLSearchParams();
    prepareAgentlyDataConnectorRequest({
      url: '/v1/api/agently/forge/window/record', queryParams, windowState,
    });
    expect(queryParams.toString()).toBe('');
  });

  it('keeps datasource fetch on the existing public contract', () => {
    const body = {inputs: {RecordId: [85141]}};
    prepareAgentlyDataConnectorRequest({
      url: '/v1/api/datasources/record_identity/fetch', body, windowState,
    });
    expect(body).toEqual({inputs: {RecordId: [85141]}, conversationId: 'conv-1', windowParameters: {RecordId: [85141]}});
    expect(body.windowParameters).not.toBe(windowState.parameters);
  });
});

it('carries the opened definition pin without deriving a revision from UI state', () => {
 const resource={uri:'window://example/record',kind:'working',contentFingerprint:'bound',authorityBinding:'actor'};
 const body={inputs:{RecordId:[85141]},resource:{uri:'window://foreign/forged'}};
 prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/record_identity/fetch',body,windowState:{...windowState,metadata:{resource}}});
 expect(body.resource).toEqual(resource);
 expect(body.resource).not.toBe(resource);
 expect(body.inputs).toEqual({RecordId:[85141]});
});

it('forwards the signed target with the exact opened pin', () => {
 const resource={uri:'window://example/project',kind:'working',contentFingerprint:'parent',authorityBinding:'actor',validUntil:'2030-01-01T00:00:00.123456789Z'};
 const resourceTarget={platform:'ios',formFactor:'phone',surface:'app',selectionToken:'verified-presentation'};
 const body={inputs:{ProjectId:[1]},target:{platform:'web'}};
 prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/project_lookup/fetch',body,windowState:{metadata:{resource,resourceTarget}}});
 expect(body.resource).toEqual(resource);
 expect(body.target).toEqual(resourceTarget);
 expect(body.target).not.toBe(resourceTarget);
 expect(body.resource.validUntil).toBe(resource.validUntil);
});

it('routes native workspace data independently of deleted or unsaved chat without altering resource authorization',()=>{
 const resource={uri:'window://workspace/marketplaceDeals',contentFingerprint:'server-selected'};
 const body={inputs:{Ids:[123]},conversationId:'deleted',resource};
 const queryParams=new URLSearchParams('conversationId=deleted');
 prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/marketplace_deals/fetch',body,queryParams,
  windowState:{...windowState,presentation:'hosted',region:'chat.top',metadata:{resource}}});
 expect(body.conversationId).toBeUndefined();expect(queryParams.has('conversationId')).toBe(false);
 expect(body.inputs).toEqual({Ids:[123]});expect(body.resource).toEqual(resource);
});
it('retains conversation routing for conversation-origin workspace apps',()=>{
 const body={inputs:{}};
 prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/records/fetch',body,
  windowState:{...windowState,presentation:'hosted',region:'chat.top',workspaceObject:{origin:{turnId:'turn-one'}}}});
 expect(body.conversationId).toBe('conv-1');
});
