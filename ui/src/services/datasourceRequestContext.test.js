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
    expect(body).toEqual({inputs: {AdvertiserId: [85141]}, conversationId: 'conv-1'});
  });
});
