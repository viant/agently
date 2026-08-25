function numberValue(value) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function field(source, ...names) {
  for (const name of names) {
    if (source?.[name] != null) return source[name];
  }
  return undefined;
}

export function formatTokenCount(value) {
  return new Intl.NumberFormat('en-US').format(Math.max(0, Math.trunc(numberValue(value))));
}

export function formatUsageCost(value) {
  if (value == null || value === '') return 'Not reported';
  const cost = Number(value);
  if (!Number.isFinite(cost)) return 'Not reported';
  if (cost === 0) return '$0.00';
  if (cost < 0.01) return `$${cost.toFixed(4)}`;
  return `$${cost.toFixed(2)}`;
}

function normalizeModelUsage(entry = {}, index = 0) {
  const inputTokens = numberValue(field(entry, 'PromptTokens', 'promptTokens', 'inputTokens', 'InputTokens'));
  const outputTokens = numberValue(field(entry, 'CompletionTokens', 'completionTokens', 'outputTokens', 'OutputTokens'));
  const cachedInputTokens = numberValue(field(entry, 'PromptCachedTokens', 'promptCachedTokens', 'cachedInputTokens', 'CachedInputTokens'));
  const reasoningTokens = numberValue(field(entry, 'CompletionReasoningTokens', 'completionReasoningTokens', 'reasoningTokens', 'ReasoningTokens'));
  const totalTokens = numberValue(field(entry, 'TotalTokens', 'totalTokens')) || inputTokens + outputTokens;
  const rawCost = field(entry, 'Cost', 'cost');
  return {
    id: `${String(field(entry, 'Provider', 'provider') || '')}:${String(field(entry, 'Model', 'model') || index)}`,
    provider: String(field(entry, 'Provider', 'provider') || '').trim(),
    model: String(field(entry, 'Model', 'model') || 'Unknown model').trim(),
    inputTokens,
    outputTokens,
    cachedInputTokens,
    reasoningTokens,
    totalTokens,
    cost: rawCost == null ? null : numberValue(rawCost),
  };
}

export function summarizeConversationUsage(conversation = {}) {
  const usage = field(conversation, 'Usage', 'usage') || {};
  const rawModels = field(usage, 'Model', 'model', 'Models', 'models');
  const models = (Array.isArray(rawModels) ? rawModels : [])
    .map(normalizeModelUsage)
    .sort((left, right) => right.totalTokens - left.totalTokens);
  const inputTokens = numberValue(field(usage, 'PromptTokens', 'promptTokens'))
    || numberValue(field(conversation, 'UsageInputTokens', 'usageInputTokens'))
    || models.reduce((sum, model) => sum + model.inputTokens, 0);
  const outputTokens = numberValue(field(usage, 'CompletionTokens', 'completionTokens'))
    || numberValue(field(conversation, 'UsageOutputTokens', 'usageOutputTokens'))
    || models.reduce((sum, model) => sum + model.outputTokens, 0);
  const cachedInputTokens = numberValue(field(usage, 'PromptCachedTokens', 'promptCachedTokens'))
    || models.reduce((sum, model) => sum + model.cachedInputTokens, 0);
  const reasoningTokens = numberValue(field(usage, 'CompletionReasoningTokens', 'completionReasoningTokens'))
    || models.reduce((sum, model) => sum + model.reasoningTokens, 0);
  const totalTokens = numberValue(field(usage, 'TotalTokens', 'totalTokens')) || inputTokens + outputTokens;
  const rawCost = field(usage, 'Cost', 'cost');
  return {
    conversationId: String(field(conversation, 'Id', 'id', 'conversationId', 'ConversationId') || '').trim(),
    title: String(field(conversation, 'Title', 'title', 'Summary', 'summary') || 'Conversation').trim(),
    model: String(field(conversation, 'DefaultModel', 'defaultModel') || '').trim(),
    updatedAt: field(conversation, 'LastActivity', 'lastActivity', 'UpdatedAt', 'updatedAt'),
    inputTokens,
    outputTokens,
    cachedInputTokens,
    reasoningTokens,
    totalTokens,
    cost: rawCost == null ? null : numberValue(rawCost),
    models,
  };
}

export function conversationUsageHref(conversationId = '') {
  const id = String(conversationId || '').trim();
  return id ? `/conversation/${encodeURIComponent(id)}/usage` : '';
}

export function shouldShowConversationUsage(conversationId = '') {
  return String(conversationId || '').trim().length > 0;
}
