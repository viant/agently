import { describe, expect, it } from 'vitest';
import { conversationUsageHref, formatUsageCost, shouldShowConversationUsage, summarizeConversationUsage } from './conversationUsage';

describe('conversation usage', () => {
  it('normalizes aggregate and per-model backend usage', () => {
    const summary = summarizeConversationUsage({
      Id: 'c-1',
      Title: 'Review changes',
      Usage: {
        Cost: 1.2788405,
        PromptTokens: 1000,
        CompletionTokens: 50,
        TotalTokens: 1050,
        Model: [
          { Model: 'gpt-small', PromptTokens: 300, CompletionTokens: 20, TotalTokens: 320, Cost: 0.02 },
          { Model: 'gpt-large', PromptTokens: 700, CompletionTokens: 30, TotalTokens: 730, Cost: 1.2588405 },
        ],
      },
    });
    expect(summary).toMatchObject({ conversationId: 'c-1', inputTokens: 1000, outputTokens: 50, totalTokens: 1050, cost: 1.2788405 });
    expect(summary.models.map((entry) => entry.model)).toEqual(['gpt-large', 'gpt-small']);
  });

  it('keeps sidecar usage separate when it uses the same model', () => {
    const summary = summarizeConversationUsage({
      Usage: {
        Model: [
          { Provider: 'openai', Model: 'gpt-5-mini', ExecutionRole: 'react', PromptTokens: 800, CompletionTokens: 80 },
          { Provider: 'openai', Model: 'gpt-5-mini', ExecutionRole: 'intake', PromptTokens: 200, CompletionTokens: 20 },
        ],
      },
    });
    expect(summary.models).toHaveLength(2);
    expect(summary.models.map((entry) => entry.executionRole)).toEqual(['react', 'intake']);
    expect(summary.totalTokens).toBe(1100);
  });

  it('uses durable top-level conversation totals as a fallback', () => {
    expect(summarizeConversationUsage({ UsageInputTokens: 25, UsageOutputTokens: 5 })).toMatchObject({
      inputTokens: 25,
      outputTokens: 5,
      totalTokens: 30,
    });
  });

  it('builds only persisted conversation links', () => {
    expect(conversationUsageHref('')).toBe('');
    expect(conversationUsageHref('c/1')).toBe('/conversation/c%2F1/usage');
    expect(shouldShowConversationUsage('')).toBe(false);
    expect(shouldShowConversationUsage('conversation-1')).toBe(true);
    expect(formatUsageCost(null)).toBe('Not reported');
  });
});
