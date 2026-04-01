import { resolvePayload } from './chatRuntime';

function firstText(...values) {
  for (const value of values) {
    const text = String(value || '').trim();
    if (text) return text;
  }
  return '';
}

function firstNumber(...values) {
  for (const value of values) {
    const num = Number(value);
    if (Number.isFinite(num)) return num;
  }
  return 0;
}

export function transcriptConversationTurns(payload = {}) {
  return Array.isArray(payload?.conversation?.turns) ? payload.conversation.turns : [];
}

export function canonicalExecutionPages(turn = {}) {
  return Array.isArray(turn?.execution?.pages) ? turn.execution.pages : [];
}

export function normalizeCanonicalModelStep(step = {}, page = {}) {
  const provider = firstText(step?.provider);
  const model = firstText(step?.model);
  return {
    id: firstText(step?.assistantMessageId, step?.modelCallId, page?.assistantMessageId, page?.pageId),
    kind: 'model',
    reason: page?.finalResponse ? 'final_response' : 'thinking',
    toolName: firstText(provider && model ? `${provider}/${model}` : '', model, provider, 'model'),
    provider,
    model,
    status: firstText(step?.status, page?.status),
    latencyMs: 0,
    errorMessage: firstText(step?.errorMessage, page?.errorMessage),
    requestPayloadId: firstText(step?.requestPayloadId),
    responsePayloadId: firstText(step?.responsePayloadId),
    providerRequestPayloadId: firstText(step?.providerRequestPayloadId),
    providerResponsePayloadId: firstText(step?.providerResponsePayloadId),
    streamPayloadId: firstText(step?.streamPayloadId),
    requestPayload: resolvePayload(step?.requestPayload),
    responsePayload: resolvePayload(step?.responsePayload),
    providerRequestPayload: resolvePayload(step?.providerRequestPayload),
    providerResponsePayload: resolvePayload(step?.providerResponsePayload),
    streamPayload: resolvePayload(step?.streamPayload),
    linkedConversationId: '',
  };
}

export function normalizeCanonicalToolStep(step = {}, page = {}) {
  return {
    id: firstText(step?.toolMessageId, step?.toolCallId, page?.assistantMessageId, page?.pageId),
    kind: 'tool',
    reason: 'tool_call',
    toolName: firstText(step?.toolName, 'tool'),
    status: firstText(step?.status, page?.status),
    latencyMs: 0,
    errorMessage: firstText(step?.errorMessage, page?.errorMessage),
    linkedConversationId: firstText(step?.linkedConversationId),
    requestPayloadId: firstText(step?.requestPayloadId),
    responsePayloadId: firstText(step?.responsePayloadId),
    providerRequestPayloadId: firstText(step?.providerRequestPayloadId),
    providerResponsePayloadId: firstText(step?.providerResponsePayloadId),
    streamPayloadId: firstText(step?.streamPayloadId),
    requestPayload: resolvePayload(step?.requestPayload),
    responsePayload: resolvePayload(step?.responsePayload),
    providerRequestPayload: resolvePayload(step?.providerRequestPayload),
    providerResponsePayload: resolvePayload(step?.providerResponsePayload),
    streamPayload: resolvePayload(step?.streamPayload),
  };
}

export function flattenCanonicalTranscriptSteps(turns = []) {
  const out = [];
  for (const turn of Array.isArray(turns) ? turns : []) {
    for (const page of canonicalExecutionPages(turn)) {
      for (const step of Array.isArray(page?.modelSteps) ? page.modelSteps : []) {
        out.push(normalizeCanonicalModelStep(step, page));
      }
      for (const step of Array.isArray(page?.toolSteps) ? page.toolSteps : []) {
        out.push(normalizeCanonicalToolStep(step, page));
      }
    }
  }
  return out;
}

export function extractCanonicalExecutionGroups(turns = []) {
  const groups = [];
  for (const turn of Array.isArray(turns) ? turns : []) {
    const turnId = firstText(turn?.turnId);
    const turnStatus = firstText(turn?.status);
    for (const page of canonicalExecutionPages(turn)) {
      groups.push({
        ...page,
        turnId,
        turnStatus,
        assistantMessageId: firstText(page?.assistantMessageId, page?.pageId),
        parentMessageId: firstText(page?.parentMessageId),
        sequence: firstNumber(page?.pageIndex, page?.iteration),
        iteration: firstNumber(page?.iteration),
        preamble: firstText(page?.preamble),
        content: firstText(page?.content),
        status: firstText(page?.status, turnStatus),
        finalResponse: Boolean(page?.finalResponse),
        modelSteps: Array.isArray(page?.modelSteps) ? page.modelSteps : [],
        toolSteps: Array.isArray(page?.toolSteps) ? page.toolSteps : [],
        toolCallsPlanned: Array.isArray(page?.toolCallsPlanned) ? page.toolCallsPlanned : []
      });
    }
  }
  return groups.sort((left, right) => {
    if (left.sequence !== right.sequence) return left.sequence - right.sequence;
    return String(left.assistantMessageId || '').localeCompare(String(right.assistantMessageId || ''));
  });
}
