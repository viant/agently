function normalizeStatus(value = '') {
  const text = String(value || '').trim().toLowerCase();
  if (text === 'cancelled') return 'canceled';
  if (text === 'success') return 'succeeded';
  if (text === 'danger') return 'failed';
  return text;
}

function deriveVisibleText(rounds = []) {
  const list = Array.isArray(rounds) ? rounds : [];
  for (let index = list.length - 1; index >= 0; index -= 1) {
    const round = list[index] || {};
    const content = String(round?.content || '').trim();
    if (content) return content;
    const preamble = String(round?.preamble || '').trim();
    if (preamble) return preamble;
    const toolTitle = (Array.isArray(round?.toolCalls) ? round.toolCalls : [])
      .map((step) => String(step?.toolName || step?.toolCallId || '').trim())
      .find(Boolean);
    if (toolTitle) return `Calling ${toolTitle}.`;
  }
  return '';
}

function mapModelStep(step = {}, round = {}) {
  return {
    id: step.renderKey,
    kind: 'model',
    reason: round.finalResponse ? 'final_response' : 'thinking',
    phase: round.phase || '',
    provider: step.provider || '',
    model: step.model || '',
    status: normalizeStatus(step.status || ''),
    errorMessage: step.errorMessage || '',
    requestPayloadId: step.requestPayloadId || '',
    responsePayloadId: step.responsePayloadId || '',
    providerRequestPayloadId: step.providerRequestPayloadId || '',
    providerResponsePayloadId: step.providerResponsePayloadId || '',
    streamPayloadId: step.streamPayloadId || '',
    startedAt: step.startedAt || '',
    completedAt: step.completedAt || '',
  };
}

function mapToolCall(step = {}) {
  return {
    id: step.renderKey,
    toolCallId: step.toolCallId || '',
    kind: 'tool',
    reason: 'tool_call',
    toolName: step.toolName || step.toolCallId || 'tool',
    status: normalizeStatus(step.status || ''),
    errorMessage: step.errorMessage || '',
    requestPayloadId: step.requestPayloadId || '',
    responsePayloadId: step.responsePayloadId || '',
    linkedConversationId: step.linkedConversationId || '',
    linkedConversationAgentId: step.linkedConversationAgentId || '',
    linkedConversationTitle: step.linkedConversationTitle || '',
    startedAt: step.startedAt || '',
    completedAt: step.completedAt || '',
    asyncOperation: step.asyncOperation || null,
  };
}

function mapRoundToExecutionGroup(round = {}) {
  const modelSteps = Array.isArray(round.modelSteps) ? round.modelSteps.map((step) => mapModelStep(step, round)) : [];
  const toolSteps = [
    ...(Array.isArray(round.toolCalls) ? round.toolCalls.map(mapToolCall) : []),
  ];
  return {
    id: round.renderKey,
    phase: round.phase || '',
    status: normalizeStatus(
      modelSteps[modelSteps.length - 1]?.status
      || toolSteps[toolSteps.length - 1]?.status
      || ''
    ),
    preamble: String(round.preamble || '').trim(),
    content: String(round.content || '').trim(),
    finalResponse: !!round.finalResponse,
    modelSteps,
    toolSteps,
  };
}

export function rowToLegacyIterationMessage(row) {
  const rounds = Array.isArray(row?.rounds) ? row.rounds : [];
  const executionGroups = rounds.map(mapRoundToExecutionGroup);
  const visibleText = deriveVisibleText(rounds);
  const firstPreamble = rounds
    .map((round) => String(round?.preamble || '').trim())
    .find(Boolean) || '';
  const finalContent = [...rounds]
    .reverse()
    .map((round) => String(round?.content || '').trim())
    .find(Boolean) || '';
  const linkedConversations = Array.isArray(row?.linkedConversations)
    ? row.linkedConversations.map((entry) => ({
        conversationId: entry.conversationId || '',
        title: entry.title || '',
        status: entry.status || '',
        response: entry.response || '',
        updatedAt: entry.updatedAt || '',
      }))
    : [];
  const elicitation = row?.elicitation
    ? {
        message: row.elicitation.message || 'Needs input',
        requestedSchema: row.elicitation.requestedSchema || null,
        status: row.elicitation.status || '',
      }
    : null;

  return {
    id: row?.renderKey || '',
    role: 'assistant',
    status: normalizeStatus(row?.lifecycle || ''),
    turnStatus: normalizeStatus(row?.lifecycle || ''),
    content: visibleText,
    elicitation,
    elicitationId: row?.elicitation?.elicitationId || '',
    generatedFiles: [],
    _iterationData: {
      turnId: row?.turnId || '',
      status: normalizeStatus(row?.lifecycle || ''),
      turnStartedAt: row?.createdAt || '',
      preamble: firstPreamble ? { content: firstPreamble } : null,
      streamContent: !finalContent ? visibleText : '',
      response: {
        content: finalContent || visibleText,
        status: normalizeStatus(row?.lifecycle || ''),
        elicitation: elicitation || undefined,
      },
      executionGroups,
      linkedConversations,
      isLatestIteration: !!row?.isStreaming || normalizeStatus(row?.lifecycle || '') === 'running' || normalizeStatus(row?.lifecycle || '') === 'pending',
    },
  };
}
