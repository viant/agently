import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('../services/agentlyClient', () => ({
  client: {
    getTranscript: vi.fn(async () => ({})),
    getPayload: vi.fn(async () => ({ data: new TextEncoder().encode('{}'), contentType: 'application/json' })),
  }
}));

vi.mock('../services/canonicalTranscript', () => ({
  transcriptConversationTurns: vi.fn(() => []),
  flattenCanonicalTranscriptSteps: vi.fn(() => [])
}));

import { estimateTokenUsageCost, formatUsdEstimate, hydrateToolCallFromTranscript } from './DetailPanel';
import { client } from '../services/agentlyClient';
import { flattenCanonicalTranscriptSteps, transcriptConversationTurns } from '../services/canonicalTranscript';

describe('DetailPanel pricing helpers', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    global.window = Object.assign(global.window || {}, {
      location: { pathname: '/conversation/conv-1' },
      localStorage: { getItem: () => 'conv-1' }
    });
  });

  it('estimates GPT-5.4 token cost from prompt and completion usage', () => {
    const estimate = estimateTokenUsageCost({
      provider: 'openai',
      model: 'gpt-5.4',
      responsePayload: {
        usage: {
          input_tokens: 1000,
          output_tokens: 200
        }
      }
    });

    expect(estimate).toMatchObject({
      currency: 'USD'
    });
    expect(estimate.total).toBeCloseTo(0.0055, 8);
    expect(formatUsdEstimate(estimate.total)).toBe('$0.0055');
  });

  it('returns null when model pricing is unknown', () => {
    expect(estimateTokenUsageCost({
      provider: 'openai',
      model: 'unknown-model',
      responsePayload: {
        usage: { input_tokens: 1000, output_tokens: 200 }
      }
    })).toBeNull();
  });

  it('hydrates transcript payload data even when a partial payload id is already present', async () => {
    transcriptConversationTurns.mockReturnValue([]);
    flattenCanonicalTranscriptSteps.mockReturnValue([
      {
        kind: 'tool',
        id: 'call-1',
        toolName: 'platform/tree',
        requestPayloadId: 'req-1',
        responsePayloadId: 'resp-1',
        requestPayload: { field: 'IRIS_SEGMENTS' },
        responsePayload: { status: 'failed' }
      }
    ]);

    const hydrated = await hydrateToolCallFromTranscript({
      kind: 'tool',
      id: 'call-1',
      toolName: 'platform/tree',
      responsePayloadId: 'resp-1'
    });

    expect(client.getTranscript).toHaveBeenCalled();
    expect(hydrated.requestPayloadId).toBe('req-1');
    expect(hydrated.requestPayload).toEqual({ field: 'IRIS_SEGMENTS' });
    expect(hydrated.responsePayload).toEqual({ status: 'failed' });
  });

  it('hydrates model payload data for provider/model matched canonical steps', async () => {
    transcriptConversationTurns.mockReturnValue([]);
    flattenCanonicalTranscriptSteps.mockReturnValue([
      {
        kind: 'model',
        id: 'mc-1',
        provider: 'openai',
        model: 'gpt-5-mini',
        providerRequestPayloadId: 'prov-req-1',
        providerResponsePayloadId: 'prov-resp-1',
        providerRequestPayload: { request: true },
        providerResponsePayload: { response: true }
      }
    ]);

    const hydrated = await hydrateToolCallFromTranscript({
      kind: 'model',
      provider: 'openai',
      model: 'gpt-5-mini'
    });

    expect(client.getTranscript).toHaveBeenCalled();
    expect(hydrated.providerRequestPayloadId).toBe('prov-req-1');
    expect(hydrated.providerRequestPayload).toEqual({ request: true });
    expect(hydrated.providerResponsePayload).toEqual({ response: true });
  });
});
