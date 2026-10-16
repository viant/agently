import {describe, expect, it} from 'vitest';
import {prepareAgentlyDataConnectorRequest} from './datasourceRequestContext.js';

const windowState = {
  windowId: 'advertiser-85141',
  conversationId: 'conv-1',
  parameters: {AdvertiserId: [85141]},
};

describe('prepareAgentlyDataConnectorRequest', () => {
  it('keeps complete metadata fetch unchanged', () => {
    const queryParams = new URLSearchParams();
    prepareAgentlyDataConnectorRequest({
      url: '/v1/api/agently/forge/window/advertiser', queryParams, windowState,
    });
    expect(queryParams.toString()).toBe('');
  });

  it('keeps datasource fetch on the existing public contract', () => {
    const body = {inputs: {AdvertiserId: [85141]}};
    prepareAgentlyDataConnectorRequest({
      url: '/v1/api/datasources/advertiser_identity/fetch', body, windowState,
    });
    expect(body).toEqual({inputs: {AdvertiserId: [85141]}, conversationId: 'conv-1', windowParameters: {AdvertiserId: [85141]}});
    expect(body.windowParameters).not.toBe(windowState.parameters);
  });
});

it('carries the opened definition pin without deriving a revision from UI state', () => {
 const resource={uri:'window://steward/advertiser',kind:'working',contentFingerprint:'bound',authorityBinding:'actor'};
 const body={inputs:{AdvertiserId:[85141]},resource:{uri:'window://foreign/forged'}};
 prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/advertiser_identity/fetch',body,windowState:{...windowState,metadata:{resource}}});
 expect(body.resource).toEqual(resource);
 expect(body.resource).not.toBe(resource);
 expect(body.inputs).toEqual({AdvertiserId:[85141]});
});

it('forwards the signed target with the exact opened pin', () => {
 const resource={uri:'window://steward/campaign',kind:'working',contentFingerprint:'parent',authorityBinding:'actor',validUntil:'2030-01-01T00:00:00.123456789Z'};
 const resourceTarget={platform:'ios',formFactor:'phone',surface:'app',selectionToken:'verified-presentation'};
 const body={inputs:{CampaignId:[1]},target:{platform:'web'}};
 prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/campaign_lookup/fetch',body,windowState:{metadata:{resource,resourceTarget}}});
 expect(body.resource).toEqual(resource);
 expect(body.target).toEqual(resourceTarget);
 expect(body.target).not.toBe(resourceTarget);
 expect(body.resource.validUntil).toBe(resource.validUntil);
});
