import { describe, expect, it } from 'vitest';

import { publishPlanFeed, clearPlanFeed, peekPlanFeed } from './planFeedBus';

describe('planFeedBus', () => {
  it('extracts latest plan from canonical executionGroups tool steps', () => {
    clearPlanFeed('conv-1');

    publishPlanFeed({
      conversationId: 'conv-1',
      rows: [
        {
          id: 'assistant-1',
          createdAt: '2026-03-23T10:00:00Z',
          executionGroups: [
            {
              toolSteps: [
                {
                  toolName: 'orchestration/updatePlan',
                  responsePayload: JSON.stringify({
                    explanation: 'Starting campaign performance analysis.',
                    plan: [
                      { status: 'in_progress', step: 'Resolve campaign taxonomy and agency context via AdHierarchy' },
                      { status: 'pending', step: 'Run the appropriate steward analysis agent(s)' },
                    ]
                  })
                }
              ]
            }
          ]
        }
      ]
    });

    const plan = peekPlanFeed();
    expect(plan.conversationId).toBe('conv-1');
    expect(plan.explanation).toBe('Starting campaign performance analysis.');
    expect(plan.steps).toHaveLength(2);
    expect(plan.steps[0]).toMatchObject({
      status: 'in_progress',
      step: 'Resolve campaign taxonomy and agency context via AdHierarchy'
    });
  });

  it('uses only the last updatePlan snapshot within a turn', () => {
    clearPlanFeed('conv-1');

    publishPlanFeed({
      conversationId: 'conv-1',
      rows: [
        {
          id: 'assistant-1a',
          turnId: 'turn-1',
          createdAt: '2026-03-23T10:00:00Z',
          executionGroups: [
            {
              toolSteps: [
                {
                  toolName: 'orchestration/updatePlan',
                  responsePayload: JSON.stringify({
                    explanation: 'Initial plan.',
                    plan: [
                      { status: 'in_progress', step: 'Resolve hierarchy' },
                    ]
                  })
                }
              ]
            }
          ]
        },
        {
          id: 'assistant-1b',
          turnId: 'turn-1',
          createdAt: '2026-03-23T10:00:05Z',
          executionGroups: [
            {
              toolSteps: [
                {
                  toolName: 'orchestration/updatePlan',
                  responsePayload: JSON.stringify({
                    explanation: 'Updated plan.',
                    plan: [
                      { status: 'completed', step: 'Resolve hierarchy' },
                      { status: 'in_progress', step: 'Pull pacing metrics' },
                    ]
                  })
                }
              ]
            }
          ]
        }
      ]
    });

    const plan = peekPlanFeed();
    expect(plan.explanation).toBe('Updated plan.');
    expect(plan.steps).toHaveLength(2);
    expect(plan.steps[1]).toMatchObject({
      status: 'in_progress',
      step: 'Pull pacing metrics'
    });
  });
});
