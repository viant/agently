/**
 * Usage tracking singleton. Updated from conversation data after each
 * transcript refresh. UsageBar subscribes and renders cost/token info.
 */
const listeners = new Set();

let currentUsage = {
  conversationId: '',
  cost: null,
  costText: '',
  totalTokens: 0,
  totalTokensText: '',
  promptTokens: 0,
  completionTokens: 0,
  promptCachedTokens: 0,
  tokensWithCacheText: '',
  model: '',
  updatedAt: 0,
};

function notify() {
  for (const fn of listeners) fn();
}

function formatThousands(n) {
  const v = Number(n);
  if (!Number.isFinite(v) || v === 0) return '';
  return String(Math.trunc(v)).replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
}

/**
 * Extract and publish usage from a conversation object (from API).
 */
export function publishUsage(conversationId, conv) {
  if (!conv) return;
  const usage = conv?.Usage || conv?.usage || {
    promptTokens: conv?.UsageInputTokens ?? conv?.usageInputTokens,
    completionTokens: conv?.UsageOutputTokens ?? conv?.usageOutputTokens,
    totalTokens: conv?.Usage?.TotalTokens ?? conv?.usage?.totalTokens,
    promptCachedTokens: conv?.Usage?.PromptCachedTokens ?? conv?.usage?.promptCachedTokens,
  };
  if (!usage) return;

  const promptTokens = Number(usage.PromptTokens ?? usage.promptTokens ?? usage.Prompt ?? 0) || 0;
  const completionTokens = Number(usage.CompletionTokens ?? usage.completionTokens ?? usage.Completion ?? 0) || 0;
  const totalTokens = Number(usage.TotalTokens ?? usage.totalTokens ?? usage.Total ?? 0) || 0;
  const promptCachedTokens = Number(usage.PromptCachedTokens ?? usage.promptCachedTokens ?? 0) || 0;

  // Derive cost.
  let cost = null;
  try {
    if (usage.Cost != null) {
      cost = Number(usage.Cost);
    } else if (usage.cost != null) {
      cost = Number(usage.cost);
    } else if (Array.isArray(usage.Model || usage.model)) {
      const models = usage.Model || usage.model;
      const costs = models
        .map((m) => (m?.Cost ?? m?.cost) != null ? Number(m.Cost ?? m.cost) : 0)
        .filter((v) => !Number.isNaN(v));
      if (costs.length) cost = costs.reduce((a, b) => a + b, 0);
    }
  } catch (_) {}

  const costText = (cost != null && !Number.isNaN(cost) && cost > 0) ? `$${cost.toFixed(3)}` : '';
  const totalTokensText = formatThousands(totalTokens);
  const cachedText = formatThousands(promptCachedTokens);
  const tokensWithCacheText = cachedText ? `${totalTokensText} (cached ${cachedText})` : totalTokensText;

  // Normalize model name.
  let model = '';
  const rawModel = usage.Model ?? usage.model ?? conv?.DefaultModel ?? conv?.defaultModel ?? '';
  if (typeof rawModel === 'string') model = rawModel;
  else if (Array.isArray(rawModel) && rawModel[0]) model = String(rawModel[0]?.Model || rawModel[0]?.model || '');
  else if (rawModel && typeof rawModel === 'object') model = String(rawModel.Model || rawModel.model || '');

  currentUsage = {
    conversationId: String(conversationId || ''),
    cost,
    costText,
    totalTokens,
    totalTokensText,
    promptTokens,
    completionTokens,
    promptCachedTokens,
    tokensWithCacheText,
    model,
    updatedAt: Date.now(),
  };
  notify();
}

export function clearUsage() {
  currentUsage = { ...currentUsage, cost: null, costText: '', totalTokens: 0, totalTokensText: '', tokensWithCacheText: '', updatedAt: Date.now() };
  notify();
}

export function getUsage() {
  return currentUsage;
}

export function onUsageChange(fn) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}
