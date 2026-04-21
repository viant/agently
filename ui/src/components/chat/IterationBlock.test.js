import { describe, expect, it } from 'vitest';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

import IterationBlock, {
  displayLinkedConversationIcon,
  displayLinkedConversationSubtitle,
  displayLinkedConversationTitle,
  isQueuedLinkedConversationPreview,
  displayItemRowIcon,
  displayItemRowTitle,
  mapCanonicalExecutionGroups,
  buildSyntheticModelGroup,
  statusTone,
  isIterationActive,
  isActiveStatus,
  resolveIterationDisplayStatus,
  resolveIterationElapsedAnchor,
  resolveIterationAgentLabel,
  resolveIterationStatusDetail,
  resolveVisibleBubbleContent,
  resolveIterationBubbleContent,
  shouldAutoScrollExecutionGroups,
  shouldShowPreambleBubble,
  hasPendingElicitationStep,
  phaseBadgeLabel,
  toolStepSummaryText
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
    expect(displayItemRowTitle({ kind: 'elicitation', toolName: 'Needs input' })).toBe('Needs input');
    expect(displayItemRowTitle({ kind: 'turn', reason: 'turn_started', toolName: 'turn_started' })).toBe('Turn started');
    expect(displayItemRowIcon({ kind: 'turn', reason: 'turn_started', toolName: 'turn_started' })).toBe('⏺');
  });

  it('treats queued next/queued linked previews as queue UI, not linked conversation cards', () => {
    expect(isQueuedLinkedConversationPreview({ status: 'queued', title: 'Next' })).toBe(true);
    expect(isQueuedLinkedConversationPreview({ status: 'pending', linkedConversationTitle: 'Queued' })).toBe(true);
    expect(isQueuedLinkedConversationPreview({ status: 'running', title: 'Next' })).toBe(false);
    expect(isQueuedLinkedConversationPreview({ status: 'queued', title: 'Forecasting Child' })).toBe(false);
  });

  it('treats terminated iterations as inactive so execution details stop auto-scrolling', () => {
    expect(isIterationActive({ status: 'terminated' }, [{
      status: 'terminated',
      modelStep: { status: 'terminated' },
      toolSteps: [{ status: 'completed' }]
    }])).toBe(false);

    expect(isIterationActive({ status: 'running' }, [])).toBe(true);
  });

  it('never auto-scrolls execution details once the turn is terminal', () => {
    expect(shouldAutoScrollExecutionGroups({
      collapsed: false,
      isActiveIteration: true,
      iterationDisplayStatus: 'completed',
    })).toBe(false);
    expect(shouldAutoScrollExecutionGroups({
      collapsed: false,
      isActiveIteration: true,
      iterationDisplayStatus: 'failed',
    })).toBe(false);
    expect(shouldAutoScrollExecutionGroups({
      collapsed: false,
      isActiveIteration: true,
      iterationDisplayStatus: 'running',
    })).toBe(true);
  });

  it('keeps a completed parent turn inactive even if linked child conversations are still running', () => {
    expect(resolveIterationDisplayStatus(
      { status: 'completed' },
      [],
      ['running']
    )).toBe('completed');
    expect(isIterationActive(
      { status: 'completed' },
      [],
      ['running']
    )).toBe(false);
  });

  it('treats streaming execution as active and running-toned', () => {
    expect(isActiveStatus('streaming')).toBe(true);
    expect(isIterationActive({ status: 'streaming' }, [])).toBe(true);
    expect(statusTone('streaming')).toBe('running');
  });

  it('surfaces tool progress text from response payloads for execution rows', () => {
    expect(toolStepSummaryText({
      toolName: 'llm/agents/status',
      responsePayload: {
        message: 'Reviewing site pressure and supply constraints now.'
      }
    })).toBe('Reviewing site pressure and supply constraints now.');
  });

  it('prefers canonical tool step content for execution-row summaries', () => {
    expect(toolStepSummaryText({
      toolName: 'llm/agents/status',
      content: 'Checking blocker diagnosis on each order in parallel.'
    })).toBe('Checking blocker diagnosis on each order in parallel.');
  });

  it('preserves canonical tool step content when mapping execution groups', () => {
    const groups = mapCanonicalExecutionGroups([{
      assistantMessageId: 'msg-1',
      status: 'running',
      toolSteps: [{
        toolMessageId: 'tool-msg-1',
        toolCallId: 'call-1',
        toolName: 'llm/agents/status',
        content: 'Reviewing blocker diagnosis in parallel.',
        status: 'running'
      }]
    }]);

    expect(groups[0].toolSteps[0]).toMatchObject({
      toolName: 'llm/agents/status',
      content: 'Reviewing blocker diagnosis in parallel.',
      status: 'running'
    });
  });

  it('treats resolved elicitation statuses as terminal and success-toned when appropriate', () => {
    expect(statusTone('accepted')).toBe('success');
    expect(statusTone('submitted')).toBe('success');
    expect(statusTone('declined')).toBe('error');
    expect(statusTone('canceled')).toBe('error');
  });

  it('keeps the iteration display status completed when the parent turn is already terminal, even if a linked child is still active', () => {
    const status = resolveIterationDisplayStatus(
      { status: 'completed' },
      [{ status: 'completed', modelStep: { status: 'completed' }, toolSteps: [] }],
      ['running']
    );
    expect(status).toBe('completed');
    expect(isIterationActive({ status: 'completed' }, [{ status: 'completed', modelStep: { status: 'completed' }, toolSteps: [] }], ['running'])).toBe(false);
    expect(statusTone(status)).toBe('success');
  });

  it('anchors active elapsed time to the latest active execution frontier instead of the oldest completed step', () => {
    const anchor = resolveIterationElapsedAnchor(
      { status: 'running', startedAt: '2026-04-14T12:00:00Z' },
      [
        {
          status: 'completed',
          modelStep: { status: 'completed', startedAt: '2026-04-14T01:00:00Z' },
          toolSteps: []
        },
        {
          status: 'thinking',
          modelStep: { status: 'thinking', startedAt: '2026-04-14T12:10:00Z' },
          toolSteps: []
        }
      ],
      [],
      { createdAt: '2026-04-14T11:59:00Z' },
      true
    );

    expect(anchor).toBe(Date.parse('2026-04-14T12:10:00Z'));
  });

  it('falls back to the latest active preamble timestamp when active groups are all historical', () => {
    const anchor = resolveIterationElapsedAnchor(
      {
        status: 'running',
        preamble: { createdAt: '2026-04-14T12:20:00Z', content: 'Working…' },
        preambles: [{ createdAt: '2026-04-14T12:19:30Z', content: 'Starting…' }]
      },
      [
        {
          status: 'completed',
          modelStep: { status: 'completed', startedAt: '2026-04-14T01:00:00Z' },
          toolSteps: []
        }
      ],
      [],
      { createdAt: '2026-04-14T01:00:00Z' },
      true
    );

    expect(anchor).toBe(Date.parse('2026-04-14T12:20:00Z'));
  });

  it('uses streamCreatedAt as the active elapsed anchor when the live frontier is a separate stream-owned bubble', () => {
    const anchor = resolveIterationElapsedAnchor(
      {
        status: 'running',
        streamCreatedAt: '2026-04-14T12:30:00Z',
        preamble: { createdAt: '2026-04-14T01:00:00Z', content: 'Old preamble' }
      },
      [
        {
          status: 'completed',
          modelStep: { status: 'completed', startedAt: '2026-04-14T01:00:00Z' },
          toolSteps: []
        }
      ],
      [],
      { createdAt: '2026-04-14T01:00:00Z' },
      true
    );

    expect(anchor).toBe(Date.parse('2026-04-14T12:30:00Z'));
  });

  it('prefers turnStartedAt over historical step timestamps', () => {
    const anchor = resolveIterationElapsedAnchor(
      {
        status: 'running',
        turnStartedAt: '2026-04-14T12:40:00Z',
        streamCreatedAt: '2026-04-14T12:30:00Z'
      },
      [
        {
          status: 'completed',
          modelStep: { status: 'completed', startedAt: '2026-04-14T01:00:00Z' },
          toolSteps: []
        }
      ],
      [],
      { createdAt: '2026-04-14T01:00:00Z' },
      true
    );

    expect(anchor).toBe(Date.parse('2026-04-14T12:40:00Z'));
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

  it('keeps explicit intake groups visible even when they only contain a model step', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'm-intake',
        modelMessageId: 'm-intake',
        sequence: 0,
        phase: 'intake',
        finalResponse: false,
        status: 'completed',
        modelCall: {
          provider: 'openai',
          model: 'gpt-5-mini',
          status: 'completed',
        },
        toolSteps: [],
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0]).toMatchObject({
      groupKind: 'intake',
    });
  });

  it('preserves lifecycle steps as turn events inside canonical execution groups', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'm1',
        modelMessageId: 'm1',
        sequence: 1,
        preamble: '',
        finalResponse: false,
        status: 'thinking',
        modelCall: {
          provider: 'openai',
          model: 'gpt-5.4',
          status: 'thinking'
        },
        toolSteps: [
          {
            id: 'turn_started:turn-1',
            kind: 'turn',
            reason: 'turn_started',
            toolName: 'turn_started',
            status: 'running',
            startedAt: '2026-03-16T10:00:00Z'
          }
        ]
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0].groupKind).toBe('model');
    expect(groups[0].toolSteps).toHaveLength(1);
    expect(groups[0].toolSteps[0]).toMatchObject({
      kind: 'turn',
      reason: 'turn_started',
      toolName: 'turn_started',
      status: 'running'
    });
    expect(displayItemRowTitle(groups[0].toolSteps[0])).toBe('Turn started');
    expect(displayItemRowIcon(groups[0].toolSteps[0])).toBe('⏺');
  });

  it('classifies lifecycle-only groups separately from model sidecars', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'm-turn',
        status: 'running',
        toolSteps: [
          {
            id: 'turn_started:turn-1',
            kind: 'turn',
            reason: 'turn_started',
            toolName: 'turn_started',
            status: 'running'
          }
        ]
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0]).toMatchObject({
      groupKind: 'lifecycle',
      modelStep: null,
      title: 'Turn started'
    });
    expect(groups[0].toolSteps[0]).toMatchObject({
      kind: 'turn',
      reason: 'turn_started'
    });
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
          phase: 'intake',
          provider: 'openai',
          model: 'gpt-5.4',
          status: 'completed',
          responsePayload: '{"agentId":"coder"}'
        },
        toolCalls: []
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0].groupKind).toBe('intake');
    expect(groups[0].finalContent).toBe('{"agentId":"coder"}');
    expect(resolveVisibleBubbleContent(groups)).toBe('');
  });

  it('treats llm/agents:status as a normal tool row, not a delegated-run title source', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'status-1',
        status: 'running',
        toolSteps: [
          {
            id: 'tool-status-1',
            kind: 'tool',
            reason: 'tool_call',
            toolName: 'llm/agents/status',
            status: 'running',
            responsePayload: {
              assistantResponse: 'Pulling the analyst results now.'
            }
          }
        ]
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0]).toMatchObject({
      title: 'Using llm/agents/status.',
      groupKind: 'tool'
    });
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

    expect(text).toBe('I am about to retrieve HOME.');
    expect(shouldShowPreambleBubble([], text)).toBe(true);
  });

  it('uses the elicitation prompt when final visible page content is embedded elicitation JSON', () => {
    const text = resolveVisibleBubbleContent([
      {
        finalResponse: true,
        preambleContent: 'Need input.',
        finalContent: '{"type":"elicitation","message":"Please provide the environment variable name.","requestedSchema":{"type":"object"}}'
      }
    ]);

    expect(text).toBe('Please provide the environment variable name.');
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

  it('prefers streamed text over preamble while execution groups are still live', () => {
    const text = resolveIterationBubbleContent({
      visibleGroups: [
        {
          finalResponse: false,
          preambleContent: 'Delegating the initial repository analysis…'
        }
      ],
      streamContent: 'Scanning the repository root now…'
    });

    expect(text).toBe('Scanning the repository root now…');
  });

  it('treats open elicitation execution steps as a visible prompt owner for the turn', () => {
    expect(hasPendingElicitationStep([
      {
        toolSteps: [
          {
            kind: 'elicitation',
            status: 'pending',
            message: 'Please provide your favorite color.'
          }
        ]
      }
    ])).toBe(true);

    expect(hasPendingElicitationStep([
      {
        toolSteps: [
          {
            kind: 'elicitation',
            status: 'accepted',
            message: 'Please provide your favorite color.'
          }
        ]
      }
    ])).toBe(false);
  });

  it('keeps terminal turn lifecycle entries after elicitation steps', () => {
    const groups = mapCanonicalExecutionGroups([{
      id: 'group-1',
      groupKind: 'sidecar',
      title: 'ReAct',
      status: 'completed',
      modelStep: {
        id: 'model-1',
        kind: 'model',
        provider: 'openai',
        model: 'gpt-5-mini',
        status: 'completed',
      },
      toolSteps: [
        { id: 'tool-1', kind: 'tool', toolName: 'system/os/getEnv', status: 'completed' },
        { id: 'turn-complete', kind: 'turn', reason: 'turn_completed', toolName: 'turn_completed', status: 'succeeded' },
      ],
      finalContent: '',
    }]);

    expect(groups).toHaveLength(1);
    expect(groups[0].toolSteps.map((step) => step.reason)).toContain('turn_completed');
  });

  it('maps intake, worker, and default model groups to semantic role badges', () => {
    expect(phaseBadgeLabel({
      groupKind: 'intake',
      modelStep: { kind: 'model', executionRole: 'intake', provider: 'openai', model: 'gpt-5-mini' }
    })).toBe('⇢');

    expect(phaseBadgeLabel({
      groupKind: 'tool',
      toolSteps: [{ kind: 'tool', executionRole: 'worker', toolName: 'llm/agents:start', requestPayload: JSON.stringify({ agentId: 'coder' }) }]
    })).toBe('⚙');

    expect(phaseBadgeLabel({
      groupKind: 'model',
      modelStep: { kind: 'model', executionRole: 'react', provider: 'openai', model: 'gpt-5-mini' }
    })).toBe('⌬');

    expect(phaseBadgeLabel({
      groupKind: 'model',
      executionRole: 'react',
      modelStep: { kind: 'model', provider: 'openai', model: 'gpt-5-mini' }
    })).toBe('⌬');
  });

  it('prefers the elicitation prompt text over a generic fallback label', () => {
    expect(displayItemRowTitle({
      kind: 'elicitation',
      toolName: 'Needs input',
      message: 'Please confirm the exact folder path to check.'
    })).toBe('Please confirm the exact folder path to check.');
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

  it('does not promote live stream sidecar content into the main bubble while execution details are active', () => {
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

    expect(text).toBe('Calling MetricsAdCube.');
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

  it('keeps the iteration header running while turn_started exists without a terminal turn event', () => {
    expect(resolveIterationDisplayStatus({}, [
      {
        groupKind: 'lifecycle',
        status: 'running',
        toolSteps: [
          {
            kind: 'turn',
            reason: 'turn_started',
            toolName: 'turn_started',
            status: 'running'
          }
        ]
      },
      {
        groupKind: 'tool',
        status: 'completed',
        toolSteps: [
          {
            kind: 'tool',
            reason: 'tool_call',
            toolName: 'llm/agents/status',
            status: 'completed'
          }
        ]
      }
    ], [])).toBe('running');
  });

  it('uses terminal lifecycle events to settle the iteration header status', () => {
    expect(resolveIterationDisplayStatus({}, [
      {
        groupKind: 'lifecycle',
        status: 'completed',
        toolSteps: [
          {
            kind: 'turn',
            reason: 'turn_completed',
            toolName: 'turn_completed',
            status: 'completed'
          }
        ]
      }
    ], [])).toBe('completed');
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
      id: 'tc-1',
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
