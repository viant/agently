// @vitest-environment jsdom
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { activeIteration, conversationTokenFallback, groupsFromRow, TokenDetails, toolProgressText, statusLabel, documentStepSummary } from './TurnProgressStatus';

let projectedRows = [];

vi.mock('../services/chatStore', () => ({
  useChatProjection: () => projectedRows,
}));

vi.mock('../services/stageBus', () => ({
  useStage: () => ({ phase: 'thinking', text: 'Initializing…' }),
  setStage: vi.fn(),
}));

vi.mock('../services/usageBus', () => ({
  getUsage: () => ({}),
  onUsageChange: () => () => {},
}));

vi.mock('../services/agentlyClient', () => ({
  client: { cancelTurn: vi.fn() },
}));

import TurnProgressStatus from './TurnProgressStatus';

describe('TurnProgressStatus helpers', () => {
  beforeEach(() => {
    projectedRows = [];
  });

  it('keeps failure distinct from completion in tool details', () => {
    expect(statusLabel('failed')).toBe('Failed');
    expect(statusLabel('error')).toBe('Failed');
    expect(statusLabel('completed')).toBe('completed');
  });

  it('selects the latest active iteration only', () => {
    expect(activeIteration([{kind:'iteration',turnId:'active',lifecycle:'running'},{kind:'iteration',turnId:'queued',lifecycle:'pending'}])?.turnId).toBe('active');
    expect(activeIteration([
      { kind: 'iteration', turnId: 'done', lifecycle: 'completed' },
      { kind: 'iteration', turnId: 'active', lifecycle: 'running' },
    ])?.turnId).toBe('active');
    expect(activeIteration([{ kind: 'iteration', lifecycle: 'failed' }])).toBeNull();
  });

  it('maps projected rounds into canonical progress groups', () => {
    expect(groupsFromRow({
      turnId: 'turn-1',
      rounds: [{
        pageId: 'page-1',
        phase: 'main',
        modelSteps: [{ modelCallId: 'model-1', usage: { totalTokens: 10 } }],
        toolCalls: [{ toolCallId: 'tool-1', toolName: 'Read', status: 'running' }],
        toolCallsPlanned: [{ toolCallId: 'tool-1', toolName: 'Read' }],
      }],
    })[0]).toMatchObject({
      turnId: 'turn-1',
      pageId: 'page-1',
      toolSteps: [{ toolCallId: 'tool-1' }],
      toolCallsPlanned: [{ toolCallId: 'tool-1' }],
    });
  });

  it('labels conversation usage fallback explicitly', () => {
    expect(conversationTokenFallback({ totalTokens: 100, promptTokens: 80, completionTokens: 20 })).toEqual({
      scope: 'conversation',
      totalTokens: 100,
      inputTokens: 80,
      outputTokens: 20,
      cachedInputTokens: 0,
    });
  });

  it('shows only a small warning icon for incomplete tools while continuing', () => {
    projectedRows = [{kind: 'iteration', turnId: 'active', lifecycle: 'running', rounds: [{
      pageId: 'page-1', phase: 'main', status: 'running',
      toolCalls: [{toolCallId: 'one', toolName: 'Read', status: 'failed'}, {toolCallId: 'two', toolName: 'Search', status: 'running'}],
      toolCallsPlanned: [{toolCallId: 'one', toolName: 'Read'}, {toolCallId: 'two', toolName: 'Search'}],
    }]}];
    const html = renderToStaticMarkup(React.createElement(TurnProgressStatus, {conversationId: 'c'}));
    expect(html).toContain('app-turn-progress-warning');
    expect(html).not.toContain('has-failure');
    expect(html).not.toContain('Needs attention');
    expect(html).not.toContain('failed');
  });

  it('formats explicit tool-state counts', () => {
    expect(toolProgressText({
      identityComplete: true,
      totalToolCount: 5,
      completedToolCount: 2,
      activeToolCount: 2,
      queuedToolCount: 0,
      failedToolCount: 1,
    })).toBe('2/5 done · 2 active');
  });

  it('does not present default workspace initialization as an active turn', () => {
    expect(renderToStaticMarkup(React.createElement(TurnProgressStatus, { conversationId: '' }))).toBe('');
    expect(renderToStaticMarkup(React.createElement(TurnProgressStatus, { conversationId: 'conversation-1' }))).toBe('');
  });

  it('hides progress for completed, failed and cancelled turns', () => {
    for (const lifecycle of ['completed', 'failed', 'cancelled']) {
      projectedRows = [{kind: 'iteration', lifecycle, turnId: 'finished'}];
      expect(renderToStaticMarkup(React.createElement(TurnProgressStatus, {conversationId: 'c'}))).toBe('');
    }
  });

  it('shows pending submission before the server assigns a turn id', () => {
    projectedRows = [{kind: 'iteration', lifecycle: 'pending', turnId: ''}];
    const html = renderToStaticMarkup(React.createElement(TurnProgressStatus, {conversationId: 'c'}));
    expect(html).toContain('Submitting request');
    expect(html).not.toContain('Stop current request');
  });

  it('bridges the OAuth callback hydration gap with persisted blocking state', () => {
    const html = renderToStaticMarkup(React.createElement(TurnProgressStatus, {
      conversationId: 'conversation-1',
      connectionResumePending: { conversationId: 'conversation-1', elicitationId: 'elic-1' },
    }));
    expect(html).toContain('Completing connection');
  });

  it('presents progress only when the conversation has an active turn', () => {
    projectedRows = [{
      kind: 'iteration',
      turnId: 'turn-1',
      lifecycle: 'running',
      rounds: [{ pageId: 'page-1', status: 'running', phase: 'main' }],
    }];
    const html = renderToStaticMarkup(React.createElement(TurnProgressStatus, { conversationId: 'conversation-1' }));
    expect(html).toContain('Working on your request');
  });

  it('presents waiting-for-user without a running spinner or stop action', () => {
    projectedRows = [{
      kind: 'iteration',
      turnId: 'turn-waiting',
      lifecycle: 'running',
      rounds: [{ pageId: 'page-1', status: 'waiting_for_user', phase: 'main' }],
    }];
    const html = renderToStaticMarkup(React.createElement(TurnProgressStatus, { conversationId: 'conversation-1' }));
    expect(html).toContain('Needs your input');
    expect(html).not.toContain('bp6-spinner');
    expect(html).not.toContain('Stop current request');
  });

  it('shows client execution as working while requested tools with approvals or unknown waits stay waiting', () => {
    for (const waitKind of ['client_tool', 'approval', 'elicitation', 'unknown', undefined]) {
      projectedRows = [{kind: 'iteration', turnId: 'current', lifecycle: 'running', waitKind,
        rounds: [{pageId: 'page', status: 'waiting_for_user', phase: 'main', toolCalls: [{toolCallId: 'call', toolName: 'Fixture tool', status: 'requested'}]}]}];
      const html = renderToStaticMarkup(React.createElement(TurnProgressStatus, {conversationId: 'c'}));
      if (waitKind === 'client_tool') {
        expect(html).toContain('Working on your request');expect(html).toContain('Executing local tool');expect(html).toContain('bp6-spinner');expect(html).not.toContain('Needs your input');
      } else {
        // Includes approval with an associated requested tool and no elicitation.
        expect(html).toContain('Needs your input');expect(html).not.toContain('Executing local tool');
      }
    }
  });
  it('keeps open and legacy human elicitations waiting alongside a client-tool hint', () => {
    for (const status of ['pending', 'open', 'waiting_for_user', 'running', undefined, ' OPEN ']) {
      for (const field of ['elicitation', 'elicitations']) {
        const item={elicitationId:'approval', ...(status === undefined ? {} : {status})};
        projectedRows=[{kind:'iteration',turnId:'current',lifecycle:'running',waitKind:'client_tool',
          [field]:field==='elicitation'?item:[item],rounds:[{pageId:'page',status:'waiting_for_user'}]}];
        const html=renderToStaticMarkup(React.createElement(TurnProgressStatus,{conversationId:'c'}));
        expect(html).toContain('Needs your input');expect(html).not.toContain('Executing local tool');
      }
    }
  });
  it('does not let resolved elicitation history hide current client-tool work', () => {
    for (const status of ['accepted', 'declined', 'canceled', 'failed']) {
      projectedRows=[{kind:'iteration',turnId:'current',lifecycle:'running',waitKind:'client_tool',elicitation:{status},rounds:[{pageId:'page',status:'waiting_for_user'}]}];
      const html=renderToStaticMarkup(React.createElement(TurnProgressStatus,{conversationId:'c'}));
      expect(html).toContain('Executing local tool');expect(html).not.toContain('Needs your input');
    }
  });
  it('does not carry client execution into another turn or a terminal turn', () => {
    projectedRows = [{kind: 'iteration', turnId: 'old', lifecycle: 'completed', waitKind: 'client_tool', rounds: []},
      {kind: 'iteration', turnId: 'new', lifecycle: 'running', rounds: [{pageId: 'page', status: 'waiting_for_user'}]}];
    const html = renderToStaticMarkup(React.createElement(TurnProgressStatus, {conversationId: 'c'}));
    expect(html).toContain('Needs your input');expect(html).not.toContain('Executing local tool');
    projectedRows = projectedRows.slice(0, 1);expect(renderToStaticMarkup(React.createElement(TurnProgressStatus, {conversationId: 'c'}))).toBe('');
  });
  it('renders aggregate and per-model token breakdowns with unavailable fields explicit', () => {
    const html = renderToStaticMarkup(React.createElement(TokenDetails, {
      usage: {
        scope: 'turn',
        totalTokens: 160,
        inputTokens: 120,
        outputTokens: 40,
        models: [{
          modelCallId: 'model-1',
          provider: 'openai',
          model: 'gpt-5',
          totalTokens: 160,
          inputTokens: 120,
          outputTokens: 40,
          cachedInputTokens: 20,
          reasoningTokens: 5,
        }],
      },
    }));
    expect(html).toContain('openai/gpt-5');
    expect(html).toContain('Cached input');
    expect(html).toContain('Reasoning');
    expect(html).toContain('Not reported');
  });
});

it('places narration only in the scoped active turn status card', () => {
  projectedRows = [
    {kind:'iteration',turnId:'old',lifecycle:'completed',operationalNarration:'Old completed narration',rounds:[]},
    {kind:'iteration',turnId:'current',lifecycle:'running',operationalNarration:'Preparing current report data',rounds:[{pageId:'current-page',status:'running'}]},
  ];
  const html=renderToStaticMarkup(React.createElement(TurnProgressStatus,{conversationId:'current-conversation'}));
  expect(html).toContain('turn-progress-status');expect(html).toContain('turn-progress-narration');
  expect(html).toContain('Preparing current report data');expect(html).not.toContain('Old completed narration');
  projectedRows=[{...projectedRows[1],lifecycle:'completed'}];
  expect(renderToStaticMarkup(React.createElement(TurnProgressStatus,{conversationId:'current-conversation'}))).toBe('');
});

it('shows real step progress and a plain activity label in document chat', () => {
  projectedRows = [{kind:'iteration',turnId:'active',lifecycle:'running',rounds:[{
    pageId:'page',status:'running',content:'PRIVATE_MODEL_TEXT',
    toolCalls:[{toolCallId:'one',toolName:'PRIVATE_TOOL_NAME',status:'completed'},
      {toolCallId:'two',toolName:'ui/view/open',status:'running'}],
  }]}];
  const html=renderToStaticMarkup(React.createElement(TurnProgressStatus,{conversationId:'c',appearance:'document'}));
  expect(html).toContain('Opening workspace');
  expect(html).toContain('1 step completed');
  expect(html).toContain('1 step running');
  expect(html).not.toMatch(/Thinking|ui\/view\/open|PRIVATE_TOOL_NAME|PRIVATE_MODEL_TEXT|tokens/);
  expect(documentStepSummary({identityComplete:false,completedToolCount:4})).toBe('');
});
