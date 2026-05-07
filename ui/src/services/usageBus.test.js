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

  it('chooses the primary billed model when usage contains multiple models', () => {
    clearUsage();
    publishUsage('conv-2', {
      UsageInputTokens: 677625,
      UsageOutputTokens: 7705,
      UsageTotalTokens: 685330,
      Usage: {
        Cost: 0.07353625,
        Model: [
          { Model: 'gpt-5-mini', TotalTokens: 1718, Cost: 0 },
          { Model: 'gpt-5.4', TotalTokens: 42344, Cost: 0.07353625 },
        ],
      },
    });

    expect(getUsage()).toMatchObject({
      conversationId: 'conv-2',
      model: 'gpt-5.4',
      costText: '$0.074',
      totalTokensText: '685 330',
    });
  });
});
