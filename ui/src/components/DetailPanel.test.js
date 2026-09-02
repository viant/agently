import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const { flattenCanonicalTranscriptSteps, transcriptConversationTurns } = vi.hoisted(() => ({
  flattenCanonicalTranscriptSteps: vi.fn(() => []),
  transcriptConversationTurns: vi.fn(() => []),
}));

vi.mock('../services/agentlyClient', () => ({
  client: {
    getTranscript: vi.fn(async () => {
      const steps = flattenCanonicalTranscriptSteps();
      return {
        conversation: {
          turns: [{
            turnId: 'turn-test',
            execution: {
              pages: [{
                pageId: 'page-test',
                modelSteps: steps.filter((step) => step?.kind === 'model'),
                toolSteps: steps.filter((step) => step?.kind !== 'model'),
              }],
            },
          }],
        },
      };
    }),
    getPayload: vi.fn(async () => ({ data: new TextEncoder().encode('{}'), contentType: 'application/json' })),
  }
}));

import {
  estimatePayloadTokenUsage,
  estimateTokenUsageCost,
  estimateToolTokenUsage,
  formatUsdEstimate,
  hydrateToolCallFromTranscript
} from './DetailPanel';
import { client } from '../services/agentlyClient';
import DetailPanel from './DetailPanel';

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

  it('estimates payload tokens from UTF-8 bytes rather than JavaScript character count', () => {
    expect(estimatePayloadTokenUsage('éé')).toEqual({
      available: true,
      bytes: 4,
      tokens: 1
    });
  });

  it('breaks tool payload estimates into model output and input directions with pricing', () => {
    const requestPayload = { query: 'abcdefgh' };
    const responsePayload = { answer: 'abcdefghijklmnop' };
    const estimate = estimateToolTokenUsage({
      pricingProvider: 'openai',
      pricingModel: 'gpt-5.4',
      requestPayload,
      responsePayload
    });
    const requestBytes = new TextEncoder().encode(JSON.stringify(requestPayload)).byteLength;
    const responseBytes = new TextEncoder().encode(JSON.stringify(responsePayload)).byteLength;
    const outputTokens = Math.ceil(requestBytes / 4);
    const inputTokens = Math.ceil(responseBytes / 4);

    expect(estimate.toolInput).toMatchObject({
      bytes: requestBytes,
      tokens: outputTokens,
      tokenDirection: 'output'
    });
    expect(estimate.toolOutput).toMatchObject({
      bytes: responseBytes,
      tokens: inputTokens,
      tokenDirection: 'input'
    });
    expect(estimate.outputTokens).toBe(outputTokens);
    expect(estimate.inputTokens).toBe(inputTokens);
    expect(estimate.toolInput.cost).toBeCloseTo((outputTokens / 1_000_000) * 15, 12);
    expect(estimate.toolOutput.cost).toBeCloseTo((inputTokens / 1_000_000) * 2.5, 12);
    expect(estimate.totalCost).toBeCloseTo(
      (outputTokens / 1_000_000) * 15 + (inputTokens / 1_000_000) * 2.5,
      12
    );
    expect(formatUsdEstimate(estimate.totalCost)).not.toBe('$0.0000');
  });

  it('keeps token estimates when model pricing is unavailable', () => {
    const estimate = estimateToolTokenUsage({
      requestPayload: { query: 'x' },
      responsePayload: { answer: 'y' }
    });
    expect(estimate.totalTokens).toBeGreaterThan(0);
    expect(estimate.totalCost).toBeNull();
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

  it('hydrates completed-route tool payloads for exec and patch steps via table-driven cases', async () => {
    const testCases = [
      {
        name: 'system_exec execute',
        transcriptSteps: [
          {
            kind: 'tool',
            id: 'exec-1',
            toolName: 'system_exec-execute',
            requestPayloadId: 'req-exec-1',
            responsePayloadId: 'resp-exec-1',
            requestPayload: { commands: ['pwd', 'ls'], workdir: '/Users/awitas/go/src/github.com/viant' },
            responsePayload: { stdout: '/Users/awitas/go/src/github.com/viant' }
          }
        ],
        input: {
          kind: 'tool',
          id: 'exec-1',
          toolName: 'system_exec-execute'
        },
        expected: {
          requestPayloadId: 'req-exec-1',
          responsePayloadId: 'resp-exec-1',
          requestPayload: { commands: ['pwd', 'ls'], workdir: '/Users/awitas/go/src/github.com/viant' },
          responsePayload: { stdout: '/Users/awitas/go/src/github.com/viant' }
        }
      },
      {
        name: 'system_patch apply',
        transcriptSteps: [
          {
            kind: 'tool',
            id: 'patch-1',
            toolName: 'system_patch-apply',
            requestPayloadId: 'req-patch-1',
            responsePayloadId: 'resp-patch-1',
            requestPayload: { patch: '*** Begin Patch', workdir: '/Users/awitas/go/src/github.com/viant' },
            responsePayload: { stats: { added: 11 }, status: 'ok' }
          }
        ],
        input: {
          kind: 'tool',
          id: 'patch-1',
          toolName: 'system_patch-apply'
        },
        expected: {
          requestPayloadId: 'req-patch-1',
          responsePayloadId: 'resp-patch-1',
          requestPayload: { patch: '*** Begin Patch', workdir: '/Users/awitas/go/src/github.com/viant' },
          responsePayload: { stats: { added: 11 }, status: 'ok' }
        }
      }
    ];

    for (const testCase of testCases) {
      flattenCanonicalTranscriptSteps.mockReturnValueOnce(testCase.transcriptSteps);
      const hydrated = await hydrateToolCallFromTranscript(testCase.input);
      expect(hydrated).toMatchObject(testCase.expected);
    }
  });

  it('hydrates tool payload ids by tool name even when the live row id differs', async () => {
    transcriptConversationTurns.mockReturnValue([]);
    flattenCanonicalTranscriptSteps.mockReturnValue([
      {
        kind: 'tool',
        id: 'stored-call-1',
        toolName: 'llm/agents/start',
        requestPayloadId: 'req-start-1',
        responsePayloadId: 'resp-start-1'
      }
    ]);

    const hydrated = await hydrateToolCallFromTranscript({
      kind: 'tool',
      id: 'live-row-7',
      toolName: 'llm/agents/start'
    });

    expect(hydrated.requestPayloadId).toBeUndefined();
    expect(hydrated.responsePayloadId).toBeUndefined();
  });

  it('does not hydrate model payload data without an exact step id', async () => {
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
    expect(hydrated.providerRequestPayloadId).toBeUndefined();
  });

  it('hydrates model payload data with an exact step id', async () => {
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
      id: 'mc-1',
      provider: 'openai',
      model: 'gpt-5-mini'
    });

    expect(hydrated.providerRequestPayloadId).toBe('prov-req-1');
    expect(hydrated.providerRequestPayload).toEqual({ request: true });
    expect(hydrated.providerResponsePayload).toEqual({ response: true });
  });

  it('hydrates model payload data with an exact modelCallId even when id is absent', async () => {
    transcriptConversationTurns.mockReturnValue([]);
    flattenCanonicalTranscriptSteps.mockReturnValue([
      {
        kind: 'model',
        id: 'mc-1',
        modelCallId: 'mc-1',
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
      modelCallId: 'mc-1',
      provider: 'openai',
      model: 'gpt-5-mini'
    });

    expect(hydrated.providerRequestPayloadId).toBe('prov-req-1');
    expect(hydrated.providerResponsePayload).toEqual({ response: true });
  });

  it('hydrates model payload data with an exact assistant message id', async () => {
    transcriptConversationTurns.mockReturnValue([]);
    flattenCanonicalTranscriptSteps.mockReturnValue([
      {
        kind: 'model',
        id: 'mc-1',
        modelCallId: 'mc-1',
        assistantMessageId: 'msg-1',
        provider: 'openai',
        model: 'gpt-5-mini',
        providerRequestPayloadId: 'prov-req-1',
        providerResponsePayloadId: 'prov-resp-1',
        streamPayloadId: 'stream-1'
      }
    ]);

    const hydrated = await hydrateToolCallFromTranscript({
      kind: 'model',
      assistantMessageId: 'msg-1',
      provider: 'openai',
      model: 'gpt-5-mini'
    });

    expect(hydrated.providerRequestPayloadId).toBe('prov-req-1');
    expect(hydrated.providerResponsePayloadId).toBe('prov-resp-1');
    expect(hydrated.streamPayloadId).toBe('stream-1');
  });

  it('hydrates tool payload ids with an exact toolCallId even when id is absent', async () => {
    transcriptConversationTurns.mockReturnValue([]);
    flattenCanonicalTranscriptSteps.mockReturnValue([
      {
        kind: 'tool',
        id: 'call-1',
        toolCallId: 'call-1',
        toolName: 'llm/agents/start',
        requestPayloadId: 'req-start-1',
        responsePayloadId: 'resp-start-1'
      }
    ]);

    const hydrated = await hydrateToolCallFromTranscript({
      kind: 'tool',
      toolCallId: 'call-1',
      toolName: 'llm/agents/start'
    });

    expect(hydrated.requestPayloadId).toBe('req-start-1');
    expect(hydrated.responsePayloadId).toBe('resp-start-1');
  });

  it('hydrates tool payload ids with an exact tool message id', async () => {
    transcriptConversationTurns.mockReturnValue([]);
    flattenCanonicalTranscriptSteps.mockReturnValue([
      {
        kind: 'tool',
        id: 'call-1',
        toolCallId: 'call-1',
        toolMessageId: 'tm-1',
        toolName: 'llm/agents/start',
        requestPayloadId: 'req-start-1',
        responsePayloadId: 'resp-start-1'
      }
    ]);

    const hydrated = await hydrateToolCallFromTranscript({
      kind: 'tool',
      toolMessageId: 'tm-1',
      toolName: 'llm/agents/start'
    });

    expect(hydrated.requestPayloadId).toBe('req-start-1');
    expect(hydrated.responsePayloadId).toBe('resp-start-1');
  });

  it('hydrates the correct model row when multiple steps share the same provider/model', async () => {
    transcriptConversationTurns.mockReturnValue([]);
    flattenCanonicalTranscriptSteps.mockReturnValue([
      {
        kind: 'model',
        id: 'mc-1',
        modelCallId: 'mc-1',
        provider: 'openai',
        model: 'gpt-5-mini',
        providerRequestPayloadId: 'prov-req-1',
        providerRequestPayload: { step: 1 }
      },
      {
        kind: 'model',
        id: 'mc-2',
        modelCallId: 'mc-2',
        provider: 'openai',
        model: 'gpt-5-mini',
        providerRequestPayloadId: 'prov-req-2',
        providerRequestPayload: { step: 2 }
      }
    ]);

    const hydrated = await hydrateToolCallFromTranscript({
      kind: 'model',
      modelCallId: 'mc-2',
      provider: 'openai',
      model: 'gpt-5-mini'
    });

    expect(hydrated.providerRequestPayloadId).toBe('prov-req-2');
    expect(hydrated.providerRequestPayload).toEqual({ step: 2 });
  });

  it('hydrates the correct tool row when multiple steps share the same tool name', async () => {
    transcriptConversationTurns.mockReturnValue([]);
    flattenCanonicalTranscriptSteps.mockReturnValue([
      {
        kind: 'tool',
        id: 'call-1',
        toolCallId: 'call-1',
        toolName: 'resources-list',
        requestPayloadId: 'req-1',
        requestPayload: { path: '/tmp/one' }
      },
      {
        kind: 'tool',
        id: 'call-2',
        toolCallId: 'call-2',
        toolName: 'resources-list',
        requestPayloadId: 'req-2',
        requestPayload: { path: '/tmp/two' }
      }
    ]);

    const hydrated = await hydrateToolCallFromTranscript({
      kind: 'tool',
      toolCallId: 'call-2',
      toolName: 'resources-list'
    });

    expect(hydrated.requestPayloadId).toBe('req-2');
    expect(hydrated.requestPayload).toEqual({ path: '/tmp/two' });
  });

  it('renders an Open Review action for elicitation-backed detail rows', () => {
    const html = renderToStaticMarkup(React.createElement(DetailPanel, {
      toolCall: {
        kind: 'elicitation',
        toolName: 'Needs input',
        status: 'rejected',
        elicitationId: 'elic-review-1',
        requestedSchema: {
          type: 'object',
          properties: {
            rows: { type: 'array' }
          }
        },
        message: 'Review the selected site recommendation changes before patching.'
      },
      onClose: () => {}
    }));

    expect(html).toContain('Open Review');
  });

  it('renders directional tool token and pricing attribution without calling it billed usage', () => {
    const html = renderToStaticMarkup(React.createElement(DetailPanel, {
      toolCall: {
        kind: 'tool',
        reason: 'tool_call',
        toolName: 'resources/read',
        status: 'completed',
        pricingProvider: 'openai',
        pricingModel: 'gpt-5.4',
        requestPayload: { path: '/tmp/report.json' },
        responsePayload: { content: 'example output' }
      },
      onClose: () => {}
    }));

    expect(html).toContain('Arguments ≈');
    expect(html).toContain('output tokens');
    expect(html).toContain('Result ≈');
    expect(html).toContain('input tokens');
    expect(html).toContain('openai/gpt-5.4 pricing');
    expect(html).toContain('not an addition to provider-reported totals');
  });
});
