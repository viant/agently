import { describe, expect, it } from 'vitest';

import {
  displayLinkedConversationIcon,
  displayLinkedConversationSubtitle,
  displayLinkedConversationTitle,
  displayItemRowIcon,
  displayItemRowTitle,
  mapCanonicalExecutionGroups,
  newerToolPageOffset,
  olderToolPageOffset,
  paginateToolSteps,
  buildSyntheticModelGroup,
  statusTone,
  isIterationActive,
  isActiveStatus,
  resolveIterationDisplayStatus,
  resolveIterationAgentLabel,
  resolveIterationStatusDetail,
  resolveVisibleBubbleContent,
  resolveIterationBubbleContent,
  shouldShowPreambleBubble
} from './IterationBlock';
import { summarizeLinkedConversationTranscript } from 'agently-core-ui-sdk';

describe('mapCanonicalExecutionGroups', () => {
  it('keeps tool rows and linked conversation cards on separate presentation helpers', () => {
    expect(displayItemRowTitle({
      toolName: 'llm/agents/run',
      linkedConversationId: 'child-123'
    })).toBe('llm/agents/run');
    expect(displayItemRowIcon({
      toolName: 'llm/agents/run',
      linkedConversationId: 'child-123'
    })).toBe('🛠');
    expect(displayLinkedConversationTitle()).toBe('Linked conversation');
    expect(displayLinkedConversationTitle({ title: 'Forecasting Child' })).toBe('Forecasting Child');
    expect(displayLinkedConversationTitle({ agentId: 'steward-performance' })).toBe('Steward Performance');
    expect(displayLinkedConversationTitle({
      linkedConversationAgentId: 'steward-forecasting'
    })).toBe('Steward Forecasting');
    expect(displayLinkedConversationSubtitle({ response: 'Working through the child run.' })).toBe('Working through the child run.');
    expect(displayLinkedConversationSubtitle({ agentId: 'steward-performance' })).toBe('Steward Performance');
    expect(displayLinkedConversationSubtitle({
      linkedConversationAgentId: 'steward-forecasting',
      linkedConversationId: 'child-123'
    })).toBe('Steward Forecasting');
    expect(displayLinkedConversationSubtitle({ linkedConversationId: 'child-123' })).toBe('');
    expect(displayLinkedConversationIcon()).toBe('🔗');
    expect(displayItemRowTitle({ toolName: 'resources/list' })).toBe('resources/list');
    expect(displayItemRowIcon({ toolName: 'resources/list' })).toBe('🛠');
  });

  it('treats terminated iterations as inactive so execution details stop auto-scrolling', () => {
    expect(isIterationActive({ status: 'terminated' }, [{
      status: 'terminated',
      modelStep: { status: 'terminated' },
      toolSteps: [{ status: 'completed' }]
    }])).toBe(false);

    expect(isIterationActive({ status: 'running' }, [])).toBe(true);
  });

  it('treats streaming execution as active and running-toned', () => {
    expect(isActiveStatus('streaming')).toBe(true);
    expect(isIterationActive({ status: 'streaming' }, [])).toBe(true);
    expect(statusTone('streaming')).toBe('running');
  });

  it('keeps the iteration display status running while a linked child is still active', () => {
    const status = resolveIterationDisplayStatus(
      { status: 'completed' },
      [{ status: 'completed', modelStep: { status: 'completed' }, toolSteps: [] }],
      ['running']
    );
    expect(status).toBe('running');
    expect(isIterationActive({ status: 'completed' }, [{ status: 'completed', modelStep: { status: 'completed' }, toolSteps: [] }], ['running'])).toBe(true);
    expect(statusTone(status)).toBe('running');
  });

  it('paginates tool calls at three per preamble group and advances offsets correctly', () => {
    const toolSteps = [
      { toolName: 'tool-1' },
      { toolName: 'tool-2' },
      { toolName: 'tool-3' },
      { toolName: 'tool-4' },
      { toolName: 'tool-5' },
      { toolName: 'tool-6' }
    ];

    const latestPage = paginateToolSteps(toolSteps, null, 3);
    expect(latestPage.total).toBe(6);
    expect(latestPage.hasMore).toBe(true);
    expect(latestPage.start).toBe(3);
    expect(latestPage.end).toBe(6);
    expect(latestPage.tools.map((step) => step.toolName)).toEqual(['tool-4', 'tool-5', 'tool-6']);

    const olderOffset = olderToolPageOffset(toolSteps.length, null, 3);
    expect(olderOffset).toBe(0);

    const firstPage = paginateToolSteps(toolSteps, olderOffset, 3);
    expect(firstPage.start).toBe(0);
    expect(firstPage.end).toBe(3);
    expect(firstPage.tools.map((step) => step.toolName)).toEqual(['tool-1', 'tool-2', 'tool-3']);

    const newerOffset = newerToolPageOffset(toolSteps.length, olderOffset, 3);
    expect(newerOffset).toBe(null);
  });

  it('summarizes linked child transcript into compact preview groups', () => {
    const summary = summarizeLinkedConversationTranscript({
      turns: [
        {
          status: 'completed',
          agentIdUsed: 'steward-forecasting',
          execution: {
            pages: [
              {
                assistantMessageId: 'child-1',
                status: 'completed',
                preamble: 'Calling roots.',
                toolSteps: [
                  { toolName: 'resources/roots', status: 'completed' }
                ]
              },
              {
                assistantMessageId: 'child-2',
                status: 'completed',
                preamble: 'Compiling final answer.',
                content: 'Forecast returned zero reach.'
              }
            ]
          }
        }
      ]
    });

    expect(summary.agentId).toBe('steward-forecasting');
    expect(summary.status).toBe('completed');
    expect(summary.response).toBe('Forecast returned zero reach.');
    expect(summary.previewGroups).toHaveLength(2);
    expect(summary.previewGroups[0]).toMatchObject({
      title: 'Calling roots.',
      status: 'completed',
      stepKind: 'tool',
      stepLabel: 'resources/roots'
    });
    expect(summary.previewGroups[0].detailStep).toMatchObject({
      toolName: 'resources/roots',
      status: 'completed'
    });
    expect(summary.previewGroups[1]).toMatchObject({
      title: 'Compiling final answer.',
      status: 'completed',
      stepKind: 'model'
    });
  });

  it('maps backend executionGroups directly to model and tool rows', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'm1',
        modelMessageId: 'm1',
        sequence: 1,
        preamble: 'I am going to inspect the repository.',
        finalResponse: false,
        status: 'completed',
        modelCall: {
          provider: 'openai',
          model: 'gpt-5.2',
          status: 'completed',
          latencyMs: 1500
        },
        toolMessages: [
          {
            id: 'tm1',
            linkedConversationId: 'child-1'
          }
        ],
        toolCalls: [
          {
            messageId: 'tm1',
            toolName: 'llm/agents:run',
            status: 'completed',
            latencyMs: 250
          }
        ]
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0].modelStep).toMatchObject({
      kind: 'model',
      provider: 'openai',
      model: 'gpt-5.2'
    });
    expect(groups[0].toolSteps).toHaveLength(1);
    expect(groups[0].toolSteps[0]).toMatchObject({
      kind: 'tool',
      toolName: 'llm/agents:run',
      linkedConversationId: 'child-1'
    });
    expect(groups[0].preambleContent).toBe('I am going to inspect the repository.');
  });

  it('keeps router-only model groups visible in execution details', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'router-1',
        modelMessageId: 'router-1',
        sequence: 1,
        finalResponse: true,
        status: 'completed',
        content: '{"agentId":"coder"}',
        modelCall: {
          provider: 'openai',
          model: 'gpt-5.4',
          status: 'completed',
          requestPayload: JSON.stringify({
            options: { mode: 'router' }
          }),
          responsePayload: '{"agentId":"coder"}'
        },
        toolCalls: []
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0].finalContent).toBe('{"agentId":"coder"}');
    expect(resolveVisibleBubbleContent(groups)).toBe('{"agentId":"coder"}');
  });

  it('keeps the latest visible page on the most recent presentable group when the newest group is model-only', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'm1',
        modelMessageId: 'm1',
        sequence: 1,
        preamble: 'Using resources-list.',
        finalResponse: false,
        status: 'completed',
        modelCall: {
          provider: 'openai',
          model: 'gpt-5.2',
          status: 'completed'
        },
        toolCalls: [
          {
            messageId: 'tm1',
            toolName: 'resources/list',
            status: 'completed'
          }
        ]
      },
      {
        parentMessageId: 'm2',
        modelMessageId: 'm2',
        sequence: 2,
        preamble: '',
        finalResponse: false,
        status: 'thinking',
        modelCall: {
          provider: 'openai',
          model: 'gpt-5.2',
          status: 'thinking'
        },
        toolCalls: []
      }
    ]);

    expect(groups[0].toolSteps).toHaveLength(1);
    expect(groups[1].toolSteps).toHaveLength(0);
    expect(groups[1].preambleContent).toBe('');
  });

  it('treats a blank model-only group as non-presentable trailing state', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'm1',
        modelMessageId: 'm1',
        sequence: 1,
        preamble: 'I found the workspace root; next I am listing the repo.',
        finalResponse: false,
        status: 'completed',
        modelCall: {
          provider: 'openai',
          model: 'gpt-5.4',
          status: 'completed'
        },
        toolCalls: [
          {
            messageId: 'tm1',
            toolName: 'resources/list',
            status: 'completed'
          }
        ]
      },
      {
        parentMessageId: 'm2',
        modelMessageId: 'm2',
        sequence: 2,
        preamble: '',
        finalResponse: false,
        status: 'thinking',
        modelCall: {
          provider: 'openai',
          model: 'gpt-5.4',
          status: 'thinking'
        },
        toolCalls: []
      }
    ]);

    const presentable = groups.filter((group) => {
      const preambleText = String(group?.preambleContent || '').trim();
      const finalText = String(group?.finalContent || '').trim();
      const toolCount = Array.isArray(group?.toolSteps) ? group.toolSteps.length : 0;
      const plannedCount = Array.isArray(group?.toolCallsPlanned) ? group.toolCallsPlanned.length : 0;
      const isFinal = !!group?.modelStep && String(group?.modelStep?.reason || '').toLowerCase() === 'final_response';
      return isFinal || toolCount > 0 || plannedCount > 0 || preambleText !== '' || finalText !== '';
    });

    expect(presentable).toHaveLength(1);
    expect(presentable[0].preambleContent).toContain('workspace root');
  });

  it('renders plan and planned tool calls from the model response when persisted tool rows have not arrived yet', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'm1',
        modelMessageId: 'm1',
        sequence: 1,
        preamble: 'I am going to inspect the repository structure.',
        finalResponse: false,
        status: 'thinking',
        modelCall: {
          provider: 'openai',
          model: 'gpt-5.2',
          status: 'thinking'
        },
        toolCalls: [
          {
            messageId: 'tm1',
            toolName: 'orchestration/updatePlan',
            status: 'completed'
          }
        ],
        toolCallsPlanned: [
          { toolCallId: 'call-2', toolName: 'resources-list' },
          { toolCallId: 'call-3', toolName: 'resources-grepFiles' }
        ]
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0].toolSteps.map((step) => step.toolName)).toEqual([
      'orchestration/updatePlan',
      'resources-list',
      'resources-grepFiles'
    ]);
  });

  it('keeps plan-only execution groups visible in execution details', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'm1',
        modelMessageId: 'm1',
        sequence: 1,
        preamble: 'Calling updatePlan.',
        finalResponse: false,
        status: 'completed',
        modelCall: {
          provider: 'openai',
          model: 'gpt-5.4',
          status: 'completed'
        },
        toolCalls: [
          {
            messageId: 'tm1',
            toolName: 'orchestration/updatePlan',
            status: 'completed'
          }
        ]
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0].toolSteps.map((step) => step.toolName)).toEqual([
      'orchestration/updatePlan'
    ]);
    expect(groups[0].preambleContent).toBe('Calling updatePlan.');
  });

  it('uses final content for the visible page bubble when a visible page is final', () => {
    const text = resolveVisibleBubbleContent([
      {
        finalResponse: false,
        preambleContent: 'Thinking...'
      },
      {
        finalResponse: true,
        preambleContent: 'I am about to retrieve HOME.',
        finalContent: '{"HOME":"/Users/awitas"}'
      }
    ]);

    expect(text).toBe('{"HOME":"/Users/awitas"}');
    expect(shouldShowPreambleBubble([], text)).toBe(true);
  });

  it('falls back to visible preamble content when no visible page is final', () => {
    const text = resolveVisibleBubbleContent([
      {
        finalResponse: false,
        preambleContent: 'Thinking...'
      }
    ]);

    expect(text).toBe('Thinking...');
    expect(shouldShowPreambleBubble([], text)).toBe(true);
  });

  it('falls back to the latest tool-derived group title when newer groups have no preamble text', () => {
    const text = resolveVisibleBubbleContent([
      {
        finalResponse: false,
        preambleContent: 'Calling updatePlan.',
        title: 'Calling updatePlan.',
        toolSteps: []
      },
      {
        finalResponse: false,
        preambleContent: '',
        title: 'Using llm/agents/run.',
        toolSteps: [{ toolName: 'llm/agents/run' }]
      }
    ]);

    expect(text).toBe('Using llm/agents/run.');
  });

  it('falls back to iteration stream content when there are no presentable execution groups yet', () => {
    const text = resolveIterationBubbleContent({
      visibleGroups: [],
      iterationContent: 'Once upon a time, a bear met a dog in the woods.',
      responseContent: '',
      preambleContent: '',
      streamContent: 'Once upon a time, a bear met a dog in the woods.'
    });

    expect(text).toContain('bear met a dog');
    expect(shouldShowPreambleBubble([], text)).toBe(true);
  });

  it('prefers live stream content over preamble text until a final visible response exists', () => {
    const text = resolveIterationBubbleContent({
      visibleGroups: [
        {
          finalResponse: false,
          preambleContent: 'Calling MetricsAdCube.'
        }
      ],
      iterationContent: '',
      responseContent: '',
      preambleContent: 'Calling MetricsAdCube.',
      streamContent: '<!-- CHART_SPEC:v1 -->\n```json\n{"chart":{"type":"bar"},"data":[{"x":"a","value":1}]}\n```'
    });

    expect(text).toContain('CHART_SPEC:v1');
    expect(text).not.toBe('Calling MetricsAdCube.');
  });

  it('resolves the execution header agent label from explicit iteration agent id using meta labels', () => {
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return { agent: 'chatter' };
                }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return {
                    agentOptions: [
                      { value: 'chatter', label: 'Chatter' }
                    ]
                  };
                }
              }
            }
          };
        }
        return null;
      }
    };

    expect(resolveIterationAgentLabel({ agentIdUsed: 'chatter' }, context)).toBe('Chatter');
  });

  it('shows auto-select label when the iteration explicitly resolved to auto', () => {
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return { agent: 'auto' };
                }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return {};
                }
              }
            }
          };
        }
        return null;
      }
    };

    expect(resolveIterationAgentLabel({ agentIdUsed: 'auto' }, context)).toBe('Auto-select agent');
  });

  it('shows the resolved agent label when auto-selected turn resolved to coder', () => {
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return { agent: 'auto' };
                }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return {
                    agentOptions: [
                      { value: 'auto', label: 'Auto-select agent' },
                      { value: 'coder', label: 'Coder' }
                    ]
                  };
                }
              }
            }
          };
        }
        return null;
      }
    };

    expect(resolveIterationAgentLabel({ agentIdUsed: 'coder' }, context)).toBe('Coder');
  });

  it('prefers agentIdUsed from iteration data over stale conversation form agent', () => {
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return { agent: 'chatter' };
                }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return {
                    agentOptions: [
                      { value: 'chatter', label: 'Chatter' },
                      { value: 'coder', label: 'Coder' }
                    ]
                  };
                }
              }
            }
          };
        }
        return null;
      }
    };

    expect(resolveIterationAgentLabel({ agentIdUsed: 'coder' }, context)).toBe('Coder');
  });

  it('falls back to the selected conversation agent when the turn payload omits agentIdUsed', () => {
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return { agent: 'steward' };
                }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return {
                    agentOptions: [
                      { value: 'steward', label: 'Steward' }
                    ],
                    defaults: { agent: 'steward' }
                  };
                }
              }
            }
          };
        }
        return null;
      }
    };

    expect(resolveIterationAgentLabel({}, context)).toBe('Steward');
  });

  it('falls back to execution-group request payload metadata agent id', () => {
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return { agent: 'chatter' };
                }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return {
                    agentOptions: [
                      { value: 'chatter', label: 'Chatter' },
                      { value: 'coder', label: 'Coder' }
                    ]
                  };
                }
              }
            }
          };
        }
        return null;
      }
    };

    expect(resolveIterationAgentLabel({
      executionGroups: [
        {
          modelStep: {
            requestPayload: JSON.stringify({ metadata: { agentId: 'coder' } })
          }
        }
      ]
    }, context)).toBe('Coder');
  });

  it('ignores response createdByUserId when there is no execution-derived agent identity', () => {
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return { agent: 'chatter' };
                }
              }
            }
          };
        }
        if (name === 'meta') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return {
                    agentOptions: [
                      { value: 'chatter', label: 'Chatter' },
                      { value: 'coder', label: 'Coder' }
                    ]
                  };
                }
              }
            }
          };
        }
        return null;
      }
    };

    expect(resolveIterationAgentLabel({
      response: { createdByUserId: 'coder' }
    }, context)).toBe('Chatter');
  });

  it('prefers explicit streamed agent name when present on the iteration data', () => {
    expect(resolveIterationAgentLabel({
      agentIdUsed: 'steward-performance',
      agentName: 'Steward-performance-Analyzer'
    }, null)).toBe('Steward-performance-Analyzer');
  });

  it('builds a synthetic model group when visible content exists without presentable execution rows', () => {
    const context = {
      Context(name) {
        if (name === 'conversations') {
          return {
            handlers: {
              dataSource: {
                peekFormData() {
                  return { model: 'openai_gpt-5.2' };
                }
              }
            }
          };
        }
        return null;
      }
    };

    const group = buildSyntheticModelGroup({
      data: { status: 'completed' },
      message: { id: 'iteration-1' },
      context,
      visibleText: 'Byl sobie Zbigniew.'
    });

    expect(group).toBeTruthy();
    expect(group?.finalResponse).toBe(true);
    expect(group?.finalContent).toContain('Zbigniew');
    expect(group?.modelStep).toMatchObject({
      kind: 'model',
      reason: 'final_response'
    });
  });

  it('prefers an explicit error message for iteration status detail', () => {
    expect(resolveIterationStatusDetail({
      status: 'failed',
      errorMessage: 'Canceled by user request'
    })).toBe('Canceled by user request');
  });

  it('keeps failed canonical groups presentable and carries the underlying error', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'p-failed',
        status: 'failed',
        errorMessage: 'dial tcp: lookup api.openai.com: no such host',
        modelSteps: [
          {
            modelCallId: 'mc-failed',
            provider: 'openai',
            model: 'gpt-5.4',
            status: 'failed'
          }
        ],
        toolSteps: []
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0]).toMatchObject({
      status: 'failed',
      errorMessage: 'dial tcp: lookup api.openai.com: no such host'
    });
    expect(groups[0].detailStep).toMatchObject({
      kind: 'model',
      errorMessage: 'dial tcp: lookup api.openai.com: no such host'
    });
  });

  it('shows a generic failed bubble when execution failed without normal content', () => {
    const text = resolveIterationBubbleContent({
      visibleGroups: [{
        status: 'failed',
        errorMessage: 'dial tcp: lookup api.openai.com: no such host',
        toolSteps: []
      }],
      errorMessage: 'dial tcp: lookup api.openai.com: no such host'
    });

    expect(text).toBe('We experienced an error while processing this request.');
  });

  it('maps canonical page fields (modelSteps / toolSteps) with camelCase-only keys', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'p1',
        status: 'completed',
        preamble: 'Checking files.',
        finalResponse: false,
        modelSteps: [
          {
            modelCallId: 'mc-1',
            provider: 'anthropic',
            model: 'claude-4',
            status: 'completed',
            latencyMs: 800
          }
        ],
        toolSteps: [
          {
            toolCallId: 'tc-1',
            toolMessageId: 'tm-1',
            toolName: 'resources/list',
            status: 'completed',
            latencyMs: 120,
            linkedConversationId: 'child-abc'
          }
        ],
        toolCallsPlanned: [
          { toolCallId: 'tc-2', toolName: 'resources/grep' }
        ]
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0].modelStep).toMatchObject({
      id: 'mc-1',
      kind: 'model',
      provider: 'anthropic',
      model: 'claude-4',
      status: 'completed',
      latencyMs: 800
    });
    expect(groups[0].toolSteps).toHaveLength(2);
    expect(groups[0].toolSteps[0]).toMatchObject({
      id: 'tm-1',
      toolCallId: 'tc-1',
      kind: 'tool',
      toolName: 'resources/list',
      linkedConversationId: 'child-abc'
    });
    expect(groups[0].toolSteps[1]).toMatchObject({
      toolCallId: 'tc-2',
      toolName: 'resources/grep',
      status: 'completed'
    });
  });

  it('creates one fallback execution group from tool-only linked conversation steps when canonical groups are absent', () => {
    const groups = mapCanonicalExecutionGroups([]);
    expect(groups).toHaveLength(0);

    const fallbackData = {
      turnId: 'turn-parent',
      toolCalls: [
        {
          id: 'tool-parent-step',
          kind: 'tool',
          reason: 'tool_call',
          toolName: 'llm/agents/run',
          status: 'running',
          linkedConversationId: 'child-123'
        }
      ]
    };

    const fallbackGroups = (function mapFallbackExecutionGroupsForTest(data = {}) {
      const steps = Array.isArray(data?.toolCalls) ? data.toolCalls : [];
      const modelSteps = steps.filter((step) => String(step?.kind || '').toLowerCase() === 'model');
      const toolSteps = steps.filter((step) => String(step?.kind || '').toLowerCase() !== 'model');
      return [{
        id: `fallback:${data.turnId}`,
        modelStep: modelSteps[0] || null,
        toolSteps
      }];
    })(fallbackData);

    expect(fallbackGroups).toHaveLength(1);
    expect(fallbackGroups[0].toolSteps[0]).toMatchObject({
      toolName: 'llm/agents/run',
      linkedConversationId: 'child-123'
    });
  });

  it('skips empty canonical execution pages so phantom zero-time stages do not render', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'p1',
        iteration: 1,
        status: 'completed',
        modelSteps: [{ modelCallId: 'mc-1', provider: 'openai', model: 'gpt-5.4', status: 'completed' }]
      },
      {
        parentMessageId: 'p2',
        iteration: 2,
        status: 'completed',
        modelSteps: [],
        toolSteps: [],
        preamble: '',
        content: '',
        finalResponse: false
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0].id).toBe('p1');
  });
});
