import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { activeIteration, conversationTokenFallback, groupsFromRow, TokenDetails, toolProgressText } from './TurnProgressStatus';

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

  it('selects the latest active iteration only', () => {
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

  it('formats explicit tool-state counts', () => {
    expect(toolProgressText({
      identityComplete: true,
      totalToolCount: 5,
      completedToolCount: 2,
      activeToolCount: 2,
      queuedToolCount: 0,
      failedToolCount: 1,
    })).toBe('2/5 done · 2 active · 1 failed');
  });

  it('does not present default workspace initialization as an active turn', () => {
    expect(renderToStaticMarkup(React.createElement(TurnProgressStatus, { conversationId: '' }))).toBe('');
    expect(renderToStaticMarkup(React.createElement(TurnProgressStatus, { conversationId: 'conversation-1' }))).toBe('');
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
