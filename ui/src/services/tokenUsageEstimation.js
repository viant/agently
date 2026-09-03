const MODEL_PRICING_USD_PER_MILLION = {
  'openai:gpt-5-mini': { input: 0.25, cachedInput: 0.025, output: 2.0 },
  'openai:gpt-5.3-codex': { input: 1.75, cachedInput: 0.175, output: 14.0 },
  'openai:gpt-5.4': { input: 2.5, cachedInput: 0.25, output: 15.0 },
  'openai:gpt-5.4-mini': { input: 0.75, output: 4.5 },
  'openai:gpt-5.4-nano': { input: 0.2, output: 1.25 },
  'openai:gpt-5.5': { input: 5.0, cachedInput: 0.5, output: 30.0 },
  'grok:grok-code-fast-1': { input: 0.2, output: 1.5 },
  'gemini:gemini-3-pro-preview': { input: 2.0, output: 12.0 }
};

export const TOOL_ESTIMATE_BYTES_PER_TOKEN = 4;

function firstText(...values) {
  for (const value of values) {
    const text = String(value || '').trim();
    if (text) return text;
  }
  return '';
}

function toNumber(value) {
  const number = Number(value);
  return Number.isFinite(number) && number >= 0 ? number : null;
}

function normalizePricingKey(provider = '', model = '') {
  const normalizedProvider = String(provider || '').trim().toLowerCase().replace(/[_\s]+/g, '-');
  const normalizedModel = String(model || '')
    .trim()
    .toLowerCase()
    .replace(/[_\s]+/g, '-')
    .replace(/--+/g, '-');
  if (!normalizedProvider || !normalizedModel) return '';
  return `${normalizedProvider}:${normalizedModel}`;
}

function explicitPricingPerMillion(source = {}) {
  const pricing = source?.pricing || source?.tokenPricing || {};
  const inputPerMillion = toNumber(
    pricing?.inputPerMillion
    ?? pricing?.inputUsdPerMillion
    ?? source?.inputTokenPricePerMillion
  );
  const outputPerMillion = toNumber(
    pricing?.outputPerMillion
    ?? pricing?.outputUsdPerMillion
    ?? source?.outputTokenPricePerMillion
  );
  const cachedInputPerMillion = toNumber(
    pricing?.cachedInputPerMillion
    ?? pricing?.cachedInputUsdPerMillion
    ?? source?.cachedTokenPricePerMillion
  );
  if (inputPerMillion != null || outputPerMillion != null || cachedInputPerMillion != null) {
    return { input: inputPerMillion || 0, output: outputPerMillion || 0, cachedInput: cachedInputPerMillion };
  }
  // Runtime model options store prices in USD per 1K tokens.
  const inputPerThousand = toNumber(pricing?.inputTokenPrice ?? source?.inputTokenPrice);
  const outputPerThousand = toNumber(pricing?.outputTokenPrice ?? source?.outputTokenPrice);
  const cachedInputPerThousand = toNumber(pricing?.cachedTokenPrice ?? source?.cachedTokenPrice);
  if (inputPerThousand != null || outputPerThousand != null || cachedInputPerThousand != null) {
    return {
      input: (inputPerThousand || 0) * 1000,
      output: (outputPerThousand || 0) * 1000,
      cachedInput: cachedInputPerThousand == null ? null : cachedInputPerThousand * 1000,
    };
  }
  return null;
}

export function resolveModelPricing(source = {}) {
  const explicit = explicitPricingPerMillion(source);
  if (explicit) return explicit;
  const provider = firstText(source?.pricingProvider, source?.provider);
  const model = firstText(source?.pricingModel, source?.model);
  return MODEL_PRICING_USD_PER_MILLION[normalizePricingKey(provider, model)] || null;
}

function inlinePayloadBody(payload) {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return undefined;
  return payload?.inlineBody ?? payload?.InlineBody;
}

function payloadCompression(payload) {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return '';
  return String(payload?.compression ?? payload?.Compression ?? '').trim().toLowerCase();
}

function payloadTextForEstimate(payload) {
  if (payload == null) return null;
  if (typeof payload === 'string') return payload;
  if (payload instanceof Uint8Array) return payload;
  if (typeof payload !== 'object') return String(payload);
  const compression = payloadCompression(payload);
  const inlineBody = inlinePayloadBody(payload);
  if (typeof inlineBody === 'string') {
    if (compression && compression !== 'none') return null;
    return inlineBody;
  }
  if (compression && compression !== 'none') return null;
  try {
    return JSON.stringify(payload);
  } catch (_) {
    return String(payload);
  }
}

function utf8ByteLength(value) {
  if (value instanceof Uint8Array) return value.byteLength;
  return new TextEncoder().encode(String(value || '')).byteLength;
}

function parseJSON(value) {
  if (typeof value !== 'string') return null;
  const text = value.trim();
  if (!text || (!text.startsWith('{') && !text.startsWith('['))) return null;
  try {
    return JSON.parse(text);
  } catch (_) {
    return null;
  }
}

function nonNegativeInteger(value) {
  const number = Number(value);
  return Number.isFinite(number) && number >= 0 ? Math.trunc(number) : null;
}

function rangeFromValue(value) {
  if (typeof value === 'string') {
    const match = value.trim().match(/^(\d+)\s*-\s*(\d+)$/);
    if (!match) return null;
    const from = Number(match[1]);
    const to = Number(match[2]);
    return { from, to, length: Math.max(0, to - from) };
  }
  if (!value || typeof value !== 'object') return null;
  const bytes = value?.bytes || value?.Bytes || value;
  const from = nonNegativeInteger(bytes?.offset ?? bytes?.offsetBytes ?? bytes?.from);
  const length = nonNegativeInteger(bytes?.length ?? bytes?.lengthBytes);
  const to = nonNegativeInteger(bytes?.to);
  if (from == null && length == null && to == null) return null;
  const normalizedFrom = from || 0;
  const normalizedLength = length != null ? length : Math.max(0, (to || 0) - normalizedFrom);
  return { from: normalizedFrom, to: normalizedFrom + normalizedLength, length: normalizedLength };
}

function yamlNumber(text, key) {
  const match = String(text || '').match(new RegExp(`(?:^|\\n)\\s*${key}\\s*:\\s*(\\d+)`, 'i'));
  return match ? Number(match[1]) : null;
}

export function detectToolOverflow(payload) {
  const rawBody = inlinePayloadBody(payload);
  const candidate = typeof rawBody === 'string' && (!payloadCompression(payload) || payloadCompression(payload) === 'none')
    ? rawBody
    : payload;
  const parsed = typeof candidate === 'string' ? parseJSON(candidate) : candidate;
  if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
    const continuation = parsed?.continuation || parsed?.Continuation || {};
    const overflow = parsed?.overflow === true
      || parsed?.Overflow === true
      || continuation?.hasMore === true
      || continuation?.HasMore === true;
    if (overflow) {
      return {
        overflow: true,
        hasMore: continuation?.hasMore === true || continuation?.HasMore === true || parsed?.overflow === true || parsed?.Overflow === true,
        returned: nonNegativeInteger(parsed?.returned ?? parsed?.Returned ?? continuation?.returned ?? continuation?.Returned),
        remaining: nonNegativeInteger(parsed?.remaining ?? parsed?.Remaining ?? continuation?.remaining ?? continuation?.Remaining),
        nextRange: rangeFromValue(parsed?.nextRange ?? parsed?.NextRange ?? parsed?.bytes ?? parsed?.Bytes ?? continuation?.nextRange ?? continuation?.NextRange),
      };
    }
  }
  if (typeof candidate === 'string' && /(?:^|\n)\s*overflow\s*:\s*true\b/i.test(candidate)) {
    const rangeMatch = candidate.match(/(?:^|\n)\s*nextRange\s*:\s*["']?(\d+\s*-\s*\d+)/i);
    return {
      overflow: true,
      hasMore: true,
      returned: yamlNumber(candidate, 'returned'),
      remaining: yamlNumber(candidate, 'remaining'),
      nextRange: rangeFromValue(rangeMatch?.[1] || ''),
    };
  }
  return { overflow: false, hasMore: false, returned: null, remaining: null, nextRange: null };
}

export function estimatePayloadTokenUsage(payload, bytesPerToken = TOOL_ESTIMATE_BYTES_PER_TOKEN) {
  const overflow = detectToolOverflow(payload);
  const serialized = payloadTextForEstimate(payload);
  if (serialized == null) {
    return {
      available: false,
      bytes: null,
      tokens: null,
      compressed: !!payloadCompression(payload) && payloadCompression(payload) !== 'none',
      overflow
    };
  }
  const bytes = utf8ByteLength(serialized);
  const divisor = Number(bytesPerToken) > 0 ? Number(bytesPerToken) : TOOL_ESTIMATE_BYTES_PER_TOKEN;
  return {
    available: true,
    bytes,
    tokens: bytes === 0 ? 0 : Math.ceil(bytes / divisor),
    compressed: false,
    overflow
  };
}

export function estimateToolTokenUsage(toolCall = {}) {
  const requestPayload = toolCall?.requestPayload
    ?? toolCall?.toolInput
    ?? toolCall?.arguments
    ?? null;
  const responsePayload = toolCall?.responsePayload
    ?? toolCall?.toolOutput
    ?? toolCall?.output
    ?? null;
  const toolInput = estimatePayloadTokenUsage(requestPayload);
  const toolOutput = estimatePayloadTokenUsage(responsePayload);
  const pricingProvider = firstText(toolCall?.pricingProvider, toolCall?.provider);
  const pricingModel = firstText(toolCall?.pricingModel, toolCall?.model);
  const outputPricingProvider = firstText(toolCall?.outputPricingProvider, pricingProvider);
  const outputPricingModel = firstText(toolCall?.outputPricingModel, pricingModel);
  const inputPricingProvider = firstText(toolCall?.inputPricingProvider, pricingProvider);
  const inputPricingModel = firstText(toolCall?.inputPricingModel, pricingModel);
  const outputPricing = resolveModelPricing({
    ...toolCall,
    pricingProvider: outputPricingProvider,
    pricingModel: outputPricingModel,
  });
  const inputPricing = resolveModelPricing({
    ...toolCall,
    pricingProvider: inputPricingProvider,
    pricingModel: inputPricingModel,
  });
  const inputTokens = toolOutput.available ? toolOutput.tokens : 0;
  const outputTokens = toolInput.available ? toolInput.tokens : 0;
  const toolInputCost = outputPricing && toolInput.available
    ? (outputTokens / 1_000_000) * outputPricing.output
    : null;
  const toolOutputCost = inputPricing && toolOutput.available
    ? (inputTokens / 1_000_000) * inputPricing.input
    : null;
  return {
    // Tool arguments are generated by the LLM, so they consume output tokens.
    toolInput: {
      ...toolInput,
      tokenDirection: 'output',
      cost: toolInputCost,
      pricingProvider: outputPricingProvider,
      pricingModel: outputPricingModel,
    },
    // Tool results are presented to the LLM, so they consume input tokens.
    toolOutput: {
      ...toolOutput,
      tokenDirection: 'input',
      cost: toolOutputCost,
      pricingProvider: inputPricingProvider,
      pricingModel: inputPricingModel,
      pricingAssumed: toolCall?.inputPricingAssumed === true,
    },
    inputTokens,
    outputTokens,
    totalTokens: inputTokens + outputTokens,
    totalCost: toolInputCost == null && toolOutputCost == null
      ? null
      : (toolInputCost || 0) + (toolOutputCost || 0),
    pricing: outputPricing,
    inputPricing,
    outputPricing,
    pricingProvider,
    pricingModel,
    bytesPerToken: TOOL_ESTIMATE_BYTES_PER_TOKEN,
    estimated: true
  };
}

function transcriptTurns(transcript = {}) {
  if (Array.isArray(transcript?.conversation?.turns)) return transcript.conversation.turns;
  if (Array.isArray(transcript?.turns)) return transcript.turns;
  return [];
}

function projectionTokensFreed(transcript = {}, turns = []) {
  const nodes = [transcript?.projection, transcript?.conversation?.projection];
  for (const turn of turns) {
    nodes.push(turn?.projection);
    for (const page of Array.isArray(turn?.execution?.pages) ? turn.execution.pages : []) {
      nodes.push(page?.projection);
    }
  }
  return nodes.reduce((sum, node) => sum + (nonNegativeInteger(node?.tokensFreed ?? node?.TokensFreed) || 0), 0);
}

export function summarizeTranscriptToolUsage(transcript = {}, options = {}) {
  const turns = transcriptTurns(transcript);
  const calls = [];
  let unavailablePayloadCount = 0;
  let unpricedPayloadCount = 0;
  let pricedPayloadCount = 0;
  let totalCost = 0;
  for (const turn of turns) {
    const turnId = firstText(turn?.turnId, turn?.id);
    const pages = Array.isArray(turn?.execution?.pages) ? turn.execution.pages : [];
    for (let pageIndex = 0; pageIndex < pages.length; pageIndex += 1) {
      const page = pages[pageIndex];
      const modelSteps = Array.isArray(page?.modelSteps) ? page.modelSteps : [];
      const outputPricingModel = modelSteps[modelSteps.length - 1] || {};
      let inputPricingModel = null;
      for (let nextPageIndex = pageIndex + 1; nextPageIndex < pages.length && !inputPricingModel; nextPageIndex += 1) {
        const nextModelSteps = Array.isArray(pages[nextPageIndex]?.modelSteps) ? pages[nextPageIndex].modelSteps : [];
        inputPricingModel = nextModelSteps[0] || null;
      }
      const resultPricingModel = inputPricingModel || outputPricingModel;
      for (const step of Array.isArray(page?.toolSteps) ? page.toolSteps : []) {
        const estimate = estimateToolTokenUsage({
          ...step,
          pricingProvider: firstText(step?.pricingProvider, step?.provider, outputPricingModel?.provider),
          pricingModel: firstText(step?.pricingModel, step?.model, outputPricingModel?.model),
          outputPricingProvider: firstText(step?.outputPricingProvider, outputPricingModel?.provider),
          outputPricingModel: firstText(step?.outputPricingModel, outputPricingModel?.model),
          inputPricingProvider: firstText(step?.inputPricingProvider, resultPricingModel?.provider),
          inputPricingModel: firstText(step?.inputPricingModel, resultPricingModel?.model),
          inputPricingAssumed: !inputPricingModel,
        });
        if (!estimate.toolInput.available) unavailablePayloadCount += 1;
        if (!estimate.toolOutput.available) unavailablePayloadCount += 1;
        if (estimate.toolInput.cost != null) pricedPayloadCount += 1;
        else if (estimate.toolInput.available) unpricedPayloadCount += 1;
        if (estimate.toolOutput.cost != null) pricedPayloadCount += 1;
        else if (estimate.toolOutput.available) unpricedPayloadCount += 1;
        if (estimate.totalCost != null) totalCost += estimate.totalCost;
        calls.push({
          id: firstText(step?.toolCallId, step?.toolMessageId, `${turnId}:${page?.pageId || calls.length}`),
          toolCallId: firstText(step?.toolCallId),
          toolName: firstText(step?.toolName, 'tool'),
          status: firstText(step?.status, page?.status),
          turnId,
          pageId: firstText(page?.pageId),
          ...estimate,
        });
      }
    }
  }
  const overflowCalls = calls.filter((call) => call?.toolOutput?.overflow?.overflow);
  return {
    calls,
    inputTokens: calls.reduce((sum, call) => sum + call.inputTokens, 0),
    outputTokens: calls.reduce((sum, call) => sum + call.outputTokens, 0),
    totalTokens: calls.reduce((sum, call) => sum + call.totalTokens, 0),
    inputBytes: calls.reduce((sum, call) => sum + (call.toolOutput.bytes || 0), 0),
    outputBytes: calls.reduce((sum, call) => sum + (call.toolInput.bytes || 0), 0),
    totalCost: pricedPayloadCount > 0 ? totalCost : null,
    pricedPayloadCount,
    unpricedPayloadCount,
    unavailablePayloadCount,
    costPartial: pricedPayloadCount > 0 && (unpricedPayloadCount > 0 || unavailablePayloadCount > 0),
    overflowCallCount: overflowCalls.length,
    overflowRemaining: overflowCalls.reduce((sum, call) => sum + (call.toolOutput.overflow.remaining || 0), 0),
    projectionTokensFreed: projectionTokensFreed(transcript, turns)
      + (nonNegativeInteger(options?.projectionTokensFreed) || 0),
    estimated: true,
    bytesPerToken: TOOL_ESTIMATE_BYTES_PER_TOKEN,
  };
}

export function formatUsdEstimate(value) {
  const amount = Number(value);
  if (!Number.isFinite(amount) || amount < 0) return '';
  if (amount === 0) return '$0.00';
  if (amount < 0.0001) return `$${(Math.round((amount + Number.EPSILON) * 1_000_000) / 1_000_000).toFixed(6)}`;
  if (amount < 0.01) return `$${amount.toFixed(4)}`;
  return `$${amount.toFixed(2)}`;
}
