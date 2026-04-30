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
  buildToolStepTree,
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
  buildIterationDataFromCanonicalRow,
  resolveCanonicalDetailStep,
  shouldAutoScrollExecutionGroups,
  shouldShowNarrationBubble,
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

  it('prefers canonical intake model payload identities for detail clicks', () => {
    const canonicalRow = {
      rounds: [{
        modelSteps: [{
          renderKey: 'rk_model_intake',
          modelCallId: 'mc_intake',
          assistantMessageId: 'msg_intake',
          requestPayloadId: 'req_intake',
          responsePayloadId: 'resp_intake',
          providerRequestPayloadId: 'prov_req_intake',
          providerResponsePayloadId: 'prov_resp_intake',
          streamPayloadId: 'stream_intake',
          requestPayload: { request: true },
          responsePayload: { response: true },
        }],
        toolCalls: [],
      }],
    };

    const resolved = resolveCanonicalDetailStep(canonicalRow, {
      kind: 'model',
      modelCallId: 'mc_intake',
      assistantMessageId: '',
      requestPayloadId: '',
      responsePayloadId: '',
      providerRequestPayloadId: '',
      providerResponsePayloadId: '',
      streamPayloadId: '',
    });

    expect(resolved).toMatchObject({
      modelCallId: 'mc_intake',
      assistantMessageId: 'msg_intake',
      requestPayloadId: 'req_intake',
      responsePayloadId: 'resp_intake',
      providerRequestPayloadId: 'prov_req_intake',
      providerResponsePayloadId: 'prov_resp_intake',
      streamPayloadId: 'stream_intake',
      requestPayload: { request: true },
      responsePayload: { response: true },
    });
  });

  it('rehydrates model details by assistant message identity when the row lacks a model call id', () => {
    const canonicalRow = {
      rounds: [{
        modelSteps: [{
          renderKey: 'rk_model_assistant',
          modelCallId: 'mc_assistant',
          assistantMessageId: 'msg_assistant',
          requestPayloadId: 'req_assistant',
          responsePayloadId: 'resp_assistant',
          providerRequestPayloadId: 'prov_req_assistant',
          providerResponsePayloadId: 'prov_resp_assistant',
          streamPayloadId: 'stream_assistant',
        }],
        toolCalls: [],
      }],
    };

    const resolved = resolveCanonicalDetailStep(canonicalRow, {
      kind: 'model',
      id: 'msg_assistant',
      assistantMessageId: 'msg_assistant',
      modelCallId: '',
    });

    expect(resolved).toMatchObject({
      modelCallId: 'mc_assistant',
      assistantMessageId: 'msg_assistant',
      requestPayloadId: 'req_assistant',
      responsePayloadId: 'resp_assistant',
      providerRequestPayloadId: 'prov_req_assistant',
      providerResponsePayloadId: 'prov_resp_assistant',
      streamPayloadId: 'stream_assistant',
    });
  });

  it('rehydrates tool details by tool message identity when the row lacks a tool call id', () => {
    const canonicalRow = {
      rounds: [{
        modelSteps: [],
        toolCalls: [{
          toolCallId: 'tc_nested',
          toolMessageId: 'tm_nested',
          parentMessageId: 'tm_parent',
          toolName: 'llm/agents:start',
          requestPayloadId: 'req_nested',
          responsePayloadId: 'resp_nested',
          linkedConversationId: 'child_nested'
        }],
      }],
    };

    const resolved = resolveCanonicalDetailStep(canonicalRow, {
      kind: 'tool',
      id: 'tm_nested',
      toolMessageId: 'tm_nested',
      toolCallId: '',
    });

    expect(resolved).toMatchObject({
      toolCallId: 'tc_nested',
      toolMessageId: 'tm_nested',
      parentMessageId: 'tm_parent',
      requestPayloadId: 'req_nested',
      responsePayloadId: 'resp_nested',
      linkedConversationId: 'child_nested'
    });
  });

  it('builds iteration data directly from canonical intake rounds', () => {
    const canonicalRow = {
      kind: 'iteration',
      turnId: 'turn_1',
      lifecycle: 'running',
      createdAt: '2025-01-01T00:00:00Z',
      isStreaming: true,
      linkedConversations: [],
      rounds: [{
        renderKey: 'round_intake',
        pageId: 'page_intake',
        iteration: 0,
        phase: 'intake',
        narration: 'Classifying request.',
        content: '',
        status: 'running',
        finalResponse: false,
        modelSteps: [{
          renderKey: 'rk_model',
          modelCallId: 'mc_intake',
          assistantMessageId: 'msg_intake',
          executionRole: 'intake',
          requestPayloadId: 'req_intake',
          responsePayloadId: 'resp_intake',
          providerRequestPayloadId: 'prov_req_intake',
          providerResponsePayloadId: 'prov_resp_intake',
          streamPayloadId: 'stream_intake',
        }],
        toolCalls: [],
      }],
    };

    const data = buildIterationDataFromCanonicalRow(canonicalRow, { _iterationData: {} });
    expect(data.executionGroups[0]).toMatchObject({
      pageId: 'page_intake',
      phase: 'intake',
      narration: 'Classifying request.',
    });
    expect(data.executionGroups[0].modelSteps[0]).toMatchObject({
      modelCallId: 'mc_intake',
      assistantMessageId: 'msg_intake',
      executionRole: 'intake',
      requestPayloadId: 'req_intake',
      responsePayloadId: 'resp_intake',
      providerRequestPayloadId: 'prov_req_intake',
      providerResponsePayloadId: 'prov_resp_intake',
      streamPayloadId: 'stream_intake',
    });
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

  it('falls back to the latest active narration timestamp when active groups are all historical', () => {
    const anchor = resolveIterationElapsedAnchor(
      {
        status: 'running',
        narration: { createdAt: '2026-04-14T12:20:00Z', content: 'Working…' },
        narrations: [{ createdAt: '2026-04-14T12:19:30Z', content: 'Starting…' }]
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
        narration: { createdAt: '2026-04-14T01:00:00Z', content: 'Old narration' }
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
                narration: 'Calling roots.',
                toolSteps: [
                  { toolName: 'resources/roots', status: 'completed' }
                ]
              },
              {
                assistantMessageId: 'child-2',
                status: 'completed',
                narration: 'Compiling final answer.',
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
        narration: 'I am going to inspect the repository.',
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
    expect(groups[0].narrationContent).toBe('I am going to inspect the repository.');
  });

  it('builds a nested tool tree from exact parent tool message ids', () => {
    const roots = buildToolStepTree([
      {
        id: 'parent-step',
        toolCallId: 'tc_parent',
        toolMessageId: 'tm_parent',
        toolName: 'llm/skills:activate',
        status: 'completed'
      },
      {
        id: 'child-step',
        toolCallId: 'tc_child',
        toolMessageId: 'tm_child',
        parentMessageId: 'tm_parent',
        toolName: 'llm/agents:start',
        status: 'completed'
      },
      {
        id: 'sibling-step',
        toolCallId: 'tc_sibling',
        toolMessageId: 'tm_sibling',
        toolName: 'message:add',
        status: 'completed'
      }
    ]);

    expect(roots).toHaveLength(2);
    expect(roots[0]).toMatchObject({
      toolMessageId: 'tm_parent',
      childToolSteps: [
        expect.objectContaining({
          toolMessageId: 'tm_child',
          parentMessageId: 'tm_parent',
          toolName: 'llm/agents:start'
        })
      ]
    });
    expect(roots[1]).toMatchObject({
      toolMessageId: 'tm_sibling',
      childToolSteps: []
    });
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
        narration: '',
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
    expect(groups[0].modelStep?.executionRole).toBe('intake');
    expect(groups[0].finalContent).toBe('{"agentId":"coder"}');
    expect(resolveVisibleBubbleContent(groups)).toBe('');
  });

  it('preserves narrator executionRole on canonical model groups', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'narrator-1',
        modelMessageId: 'narrator-1',
        sequence: 2,
        status: 'running',
        narration: 'Delegated to agent "coder" to list top-level files in the workspace; awaiting results...',
        modelSteps: [
          {
            modelCallId: 'narrator-step-1',
            assistantMessageId: 'narrator-1',
            executionRole: 'narrator',
            status: 'running',
            responsePayload: {
              content: 'Delegated to agent "coder" to list top-level files in the workspace; awaiting results...'
            }
          }
        ],
        toolSteps: []
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0].modelStep?.executionRole).toBe('narrator');
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
      title: 'Execution step',
      groupKind: 'tool'
    });
  });

  it('keeps the latest visible page on the most recent presentable group when the newest group is model-only', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'm1',
        modelMessageId: 'm1',
        sequence: 1,
        narration: 'Using resources-list.',
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
        narration: '',
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
    expect(groups[1].narrationContent).toBe('');
  });

  it('treats a blank model-only group as non-presentable trailing state', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'm1',
        modelMessageId: 'm1',
        sequence: 1,
        narration: 'I found the workspace root; next I am listing the repo.',
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
        narration: '',
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
      const preambleText = String(group?.narrationContent || '').trim();
      const finalText = String(group?.finalContent || '').trim();
      const toolCount = Array.isArray(group?.toolSteps) ? group.toolSteps.length : 0;
      const plannedCount = Array.isArray(group?.toolCallsPlanned) ? group.toolCallsPlanned.length : 0;
      const isFinal = !!group?.modelStep && String(group?.modelStep?.reason || '').toLowerCase() === 'final_response';
      return isFinal || toolCount > 0 || plannedCount > 0 || preambleText !== '' || finalText !== '';
    });

    expect(presentable).toHaveLength(1);
    expect(presentable[0].narrationContent).toContain('workspace root');
  });

  it('renders plan and planned tool calls from the model response when persisted tool rows have not arrived yet', () => {
    const groups = mapCanonicalExecutionGroups([
      {
        parentMessageId: 'm1',
        modelMessageId: 'm1',
        sequence: 1,
        narration: 'I am going to inspect the repository structure.',
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
        narration: 'Calling updatePlan.',
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
    expect(groups[0].narrationContent).toBe('Calling updatePlan.');
  });

  it('uses final content for the visible page bubble when a visible page is final', () => {
    const text = resolveVisibleBubbleContent([
      {
        finalResponse: false,
        narrationContent: 'Thinking...'
      },
      {
        finalResponse: true,
        narrationContent: 'I am about to retrieve HOME.',
        finalContent: '{"HOME":"/Users/awitas"}'
      }
    ]);

    expect(text).toBe('I am about to retrieve HOME.');
    expect(shouldShowNarrationBubble([], text)).toBe(true);
  });

  it('uses the elicitation prompt when final visible page content is embedded elicitation JSON', () => {
    const text = resolveVisibleBubbleContent([
      {
        finalResponse: true,
        narrationContent: 'Need input.',
        finalContent: '{"type":"elicitation","message":"Please provide the environment variable name.","requestedSchema":{"type":"object"}}'
      }
    ]);

    expect(text).toBe('Please provide the environment variable name.');
  });

  it('falls back to visible narration content when no visible page is final', () => {
    const text = resolveVisibleBubbleContent([
      {
        finalResponse: false,
        narrationContent: 'Thinking...'
      }
    ]);

    expect(text).toBe('Thinking...');
    expect(shouldShowNarrationBubble([], text)).toBe(true);
  });

  it('prefers streamed text over narration while execution groups are still live', () => {
    const text = resolveIterationBubbleContent({
      visibleGroups: [
        {
          finalResponse: false,
          narrationContent: 'Delegating the initial repository analysis…'
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

  it('preserves canonical elicitation payload when building iteration data from projector rows', () => {
    const canonicalRow = {
      kind: 'iteration',
      turnId: 'tn_elic_done',
      lifecycle: 'running',
      createdAt: '2026-04-18T19:00:00Z',
      elicitation: {
        renderKey: 'rk_elic_done',
        elicitationId: 'elic-done-1',
        status: 'accepted',
        message: 'Please confirm the exact folder path you want counted.',
        requestedSchema: {
          type: 'object',
          properties: { path: { type: 'string' } },
          required: ['path']
        }
      },
      rounds: [{
        renderKey: 'rk_round',
        pageId: 'msg_assistant_done',
        iteration: 1,
        phase: 'main',
        narration: 'Using prompt/get.',
        content: '',
        status: 'running',
        finalResponse: false,
        modelSteps: [{
          renderKey: 'rk_model',
          assistantMessageId: 'msg_assistant_done',
          modelCallId: 'mc_done',
          provider: 'openai',
          model: 'gpt-5-mini',
          status: 'running'
        }],
        toolCalls: [],
        lifecycleEntries: [],
        hasContent: true
      }],
      linkedConversations: [],
      header: { label: 'Execution details (1)', tone: 'running', count: 1 },
      isStreaming: true
    };

    const data = buildIterationDataFromCanonicalRow(canonicalRow, {});
    expect(data.elicitation).toMatchObject({
      elicitationId: 'elic-done-1',
      status: 'accepted',
      message: 'Please confirm the exact folder path you want counted.'
    });
    expect(data.response.elicitation).toMatchObject({
      elicitationId: 'elic-done-1',
      status: 'accepted',
      message: 'Please confirm the exact folder path you want counted.'
    });
    expect(data.elicitationStatus).toBe('accepted');
    expect(data.response.elicitationStatus).toBe('accepted');
  });

  it('does not advance a live wall-clock for transcript-owned running history rows', () => {
    const html = renderToStaticMarkup(React.createElement(IterationBlock, {
      canonicalRow: {
        kind: 'iteration',
        turnId: 'tn_history_running',
        lifecycle: 'running',
        createdAt: '2026-04-18T19:00:00Z',
        isStreaming: false,
        rounds: [{
          renderKey: 'rk_history_round',
          pageId: 'pg_history_running',
          iteration: 1,
          phase: 'main',
          narration: 'Using prompt/get.',
          content: '',
          status: 'running',
          finalResponse: false,
          modelSteps: [{
            renderKey: 'rk_history_model',
            assistantMessageId: 'msg_assistant_done',
            modelCallId: 'mc_history_running',
            provider: 'openai',
            model: 'gpt-5-mini',
            status: 'running'
          }],
          toolCalls: [],
          lifecycleEntries: [],
          hasContent: true
        }],
        linkedConversations: [],
        header: { label: 'Execution details (1)', tone: 'running', count: 1 },
      },
      context: null,
      showToolFeedDetail: true
    }));

    expect(html).toContain('Execution details (1)');
    expect(html).toContain('00:00');
    expect(html).not.toContain('14287:43');
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
      toolSteps: [{ kind: 'tool', executionRole: 'bootstrap', toolName: 'llm/agents:list' }]
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

  it('titles bootstrap execution groups explicitly', () => {
    const groups = mapCanonicalExecutionGroups([{
      pageId: 'turn-1:bootstrap',
      phase: 'bootstrap',
      executionRole: 'bootstrap',
      toolSteps: [
        { kind: 'tool', executionRole: 'bootstrap', toolName: 'llm/agents:list', status: 'completed' },
        { kind: 'tool', executionRole: 'bootstrap', toolName: 'llm/skills:list', status: 'completed' }
      ]
    }]);

    expect(groups).toHaveLength(1);
    expect(groups[0]).toMatchObject({
      groupKind: 'bootstrap',
      title: 'Bootstrap',
      toolSteps: expect.arrayContaining([
        expect.objectContaining({ toolName: 'llm/agents:list', executionRole: 'bootstrap' }),
        expect.objectContaining({ toolName: 'llm/skills:list', executionRole: 'bootstrap' })
      ])
    });
  });

  it('renders a visible header row for bootstrap tool-only groups', () => {
    const html = renderToStaticMarkup(
      React.createElement(IterationBlock, {
        message: {
          _iterationData: {
            turnId: 'turn-1',
            status: 'running',
            isLatestIteration: true,
            executionGroups: [{
              id: 'bootstrap-group',
              phase: 'bootstrap',
              executionRole: 'bootstrap',
              groupKind: 'bootstrap',
              title: 'Bootstrap',
              fullTitle: 'Bootstrap',
              status: 'running',
              elapsed: '00:04',
              toolSteps: [
                { id: 'tool-1', kind: 'tool', toolName: 'llm/agents:list', status: 'running' },
                { id: 'tool-2', kind: 'tool', toolName: 'llm/skills:list', status: 'running' }
              ]
            }]
          }
        },
        context: null
      })
    );

    expect(html).toContain('Bootstrap');
    expect(html).toContain('llm/agents:list');
    expect(html).toContain('llm/skills:list');
  });

  it('renders nested child tool calls under their parent execution row', () => {
    const html = renderToStaticMarkup(
      React.createElement(IterationBlock, {
        message: {
          _iterationData: {
            turnId: 'turn-nested',
            status: 'running',
            isLatestIteration: true,
            executionGroups: [{
              id: 'nested-group',
              phase: 'sidecar',
              status: 'completed',
              toolSteps: [
                {
                  id: 'tool-parent',
                  kind: 'tool',
                  toolCallId: 'tc_parent',
                  toolMessageId: 'tm_parent',
                  toolName: 'llm/skills:activate',
                  status: 'completed'
                },
                {
                  id: 'tool-child',
                  kind: 'tool',
                  toolCallId: 'tc_child',
                  toolMessageId: 'tm_child',
                  parentMessageId: 'tm_parent',
                  toolName: 'llm/agents:start',
                  status: 'completed'
                }
              ]
            }]
          }
        },
        context: null
      })
    );

    expect(html).toContain('llm/skills:activate');
    expect(html).toContain('llm/agents:start');
    expect(html).toContain('app-iteration-tool-list-nested');
  });

  it('prefers the elicitation prompt text over a generic fallback label', () => {
    expect(displayItemRowTitle({
      kind: 'elicitation',
      toolName: 'Needs input',
      message: 'Please confirm the exact folder path to check.'
    })).toBe('Please confirm the exact folder path to check.');
  });

  it('does not surface a tool-derived title as the visible bubble when no narration text exists', () => {
    const text = resolveVisibleBubbleContent([
      {
        finalResponse: false,
        narrationContent: '',
        title: 'Calling updatePlan.',
        toolSteps: []
      },
      {
        finalResponse: false,
        narrationContent: '',
        title: 'Using llm/agents/run.',
        toolSteps: [{ toolName: 'llm/agents/run' }]
      }
    ]);

    expect(text).toBe('');
  });

  it('falls back to iteration stream content when there are no presentable execution groups yet', () => {
    const text = resolveIterationBubbleContent({
      visibleGroups: [],
      iterationContent: 'Once upon a time, a bear met a dog in the woods.',
      responseContent: '',
      narrationContent: '',
      streamContent: 'Once upon a time, a bear met a dog in the woods.'
    });

    expect(text).toContain('bear met a dog');
    expect(shouldShowNarrationBubble([], text)).toBe(true);
  });

  it('does not promote live stream sidecar content into the main bubble while execution details are active', () => {
    const text = resolveIterationBubbleContent({
      visibleGroups: [
        {
          finalResponse: false,
          narrationContent: 'Calling MetricsAdCube.'
        }
      ],
      iterationContent: '',
      responseContent: '',
      narrationContent: 'Calling MetricsAdCube.',
      streamContent: '<!-- CHART_SPEC:v1 -->\n```json\n{"chart":{"type":"bar"},"data":[{"x":"a","value":1}]}\n```'
    });

    expect(text).toBe('Calling MetricsAdCube.');
  });

  it('prefers the latest execution-group narration over stale top-level narration while a turn is still active', () => {
    const text = resolveIterationBubbleContent({
      visibleGroups: [
        {
          finalResponse: false,
          narrationContent: 'Translating the baseline targeting stack into forecast parameters and checking the last three complete days one day at a time.'
        }
      ],
      iterationContent: '',
      responseContent: '',
      narrationContent: 'I have the initial baseline and I’m running the deeper cross-check now to confirm whether anything beyond setup is materially contributing to the delivery issue.',
      streamContent: ''
    });

    expect(text).toBe('Translating the baseline targeting stack into forecast parameters and checking the last three complete days one day at a time.');
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
        narration: 'Checking files.',
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
        narration: '',
        content: '',
        finalResponse: false
      }
    ]);

    expect(groups).toHaveLength(1);
    expect(groups[0].id).toBe('p1');
  });
});
