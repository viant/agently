import { describe, expect, it } from 'vitest';

import { estimateTokenUsageCost, formatUsdEstimate } from './DetailPanel';

describe('DetailPanel pricing helpers', () => {
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
});
