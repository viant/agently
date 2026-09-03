import { describe, expect, it } from 'vitest';
import {
  detectToolOverflow,
  estimatePayloadTokenUsage,
  estimateToolTokenUsage,
  summarizeTranscriptToolUsage,
} from './tokenUsageEstimation';

describe('tool token usage estimation', () => {
  it('uses UTF-8 byte length and directional model pricing', () => {
    const estimate = estimateToolTokenUsage({
      provider: 'openai',
      model: 'gpt-5.4',
      requestPayload: { query: 'éé' },
      responsePayload: { answer: 'done' },
    });
    const requestBytes = new TextEncoder().encode(JSON.stringify({ query: 'éé' })).byteLength;
    const responseBytes = new TextEncoder().encode(JSON.stringify({ answer: 'done' })).byteLength;
    expect(estimate.toolInput).toMatchObject({
      bytes: requestBytes,
      tokens: Math.ceil(requestBytes / 4),
      tokenDirection: 'output',
    });
    expect(estimate.toolOutput).toMatchObject({
      bytes: responseBytes,
      tokens: Math.ceil(responseBytes / 4),
      tokenDirection: 'input',
    });
    expect(estimate.totalCost).toBeGreaterThan(0);
  });

  it('detects JSON continuation overflow', () => {
    expect(detectToolOverflow({
      data: ['first page'],
      continuation: {
        hasMore: true,
        returned: 1,
        remaining: 9,
        nextRange: { bytes: { offset: 100, length: 200 } },
      },
    })).toMatchObject({
      overflow: true,
      hasMore: true,
      returned: 1,
      remaining: 9,
      nextRange: { from: 100, length: 200 },
    });
  });

  it('detects YAML overflow wrappers and counts only presented bytes', () => {
    const payload = [
      'overflow: true',
      'messageId: tool-message-1',
      'returned: 20',
      'remaining: 80',
      'nextRange: 1024-2048',
    ].join('\n');
    const estimate = estimatePayloadTokenUsage(payload);
    expect(estimate.overflow).toMatchObject({
      overflow: true,
      returned: 20,
      remaining: 80,
      nextRange: { from: 1024, to: 2048, length: 1024 },
    });
    expect(estimate.bytes).toBe(new TextEncoder().encode(payload).byteLength);
    expect(estimate.tokens).toBe(Math.ceil(estimate.bytes / 4));
  });

  it('marks compressed bodies unavailable instead of pricing compressed bytes', () => {
    expect(estimatePayloadTokenUsage({
      id: 'payload-1',
      compression: 'gzip',
      inlineBody: 'compressed-data',
    })).toMatchObject({
      available: false,
      compressed: true,
      bytes: null,
      tokens: null,
    });
  });

  it('aggregates each tool call without adding estimates to provider totals', () => {
    const overflowResult = 'overflow: true\nreturned: 2\nremaining: 3\nnextRange: 20-40\n';
    const summary = summarizeTranscriptToolUsage({
      projection: { tokensFreed: 44 },
      conversation: {
        turns: [{
          turnId: 'turn-1',
          execution: {
            pages: [{
              pageId: 'page-1',
              status: 'completed',
              modelSteps: [{ provider: 'openai', model: 'gpt-5.4' }],
              toolSteps: [
                {
                  toolCallId: 'call-1',
                  toolName: 'resources/read',
                  status: 'completed',
                  requestPayload: { path: '/tmp/data.json' },
                  responsePayload: { rows: [1, 2, 3] },
                },
                {
                  toolCallId: 'call-2',
                  toolName: 'message/show',
                  status: 'completed',
                  requestPayload: { messageId: 'tool-message-1' },
                  responsePayload: overflowResult,
                },
              ],
            }, {
              pageId: 'page-2',
              modelSteps: [{ provider: 'openai', model: 'gpt-5-mini' }],
              toolSteps: [],
            }],
          },
        }],
      },
    });

    expect(summary.calls).toHaveLength(2);
    expect(summary.calls.map((call) => call.toolName)).toEqual(['resources/read', 'message/show']);
    expect(summary.calls[0].toolInput.pricingModel).toBe('gpt-5.4');
    expect(summary.calls[0].toolOutput.pricingModel).toBe('gpt-5-mini');
    expect(summary.totalTokens).toBe(summary.inputTokens + summary.outputTokens);
    expect(summary.totalCost).toBeGreaterThan(0);
    expect(summary.costPartial).toBe(false);
    expect(summary.overflowCallCount).toBe(1);
    expect(summary.overflowRemaining).toBe(3);
    expect(summary.projectionTokensFreed).toBe(44);
  });
});
