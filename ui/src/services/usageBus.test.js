import { describe, expect, it } from 'vitest';
import { clearUsage, getUsage, publishUsage } from './usageBus';

describe('usageBus', () => {
  it('prefers top-level conversation usage counters over nested usage totals', () => {
    clearUsage();
    publishUsage('conv-1', {
      UsageInputTokens: 120,
      UsageOutputTokens: 30,
      UsageTotalTokens: 150,
      Usage: {
        PromptTokens: 90,
        CompletionTokens: 20,
        TotalTokens: 110,
      },
    });

    expect(getUsage()).toMatchObject({
      conversationId: 'conv-1',
      promptTokens: 120,
      completionTokens: 30,
      totalTokens: 150,
    });
  });
});
