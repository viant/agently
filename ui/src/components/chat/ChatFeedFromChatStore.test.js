import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

vi.mock('forge/components', () => ({
  AvatarIcon: ({ name = '' }) => React.createElement('span', { 'data-avatar-icon': name }),
}));

const iterationRowBlockSpy = vi.fn(({ iterationRow }) => React.createElement(
  'div',
  {
    'data-testid': 'iteration-row-block',
    'data-row-render-key': String(iterationRow?.renderKey || ''),
  },
  `Execution details`,
));

vi.mock('./IterationRowBlock.jsx', () => ({
  default: (props) => iterationRowBlockSpy(props),
}));

import ChatFeedFromChatStore from './ChatFeedFromChatStore.jsx';

const h = React.createElement;

describe('ChatFeedFromChatStore', () => {
  it('passes the canonical iteration row directly to IterationRowBlock', () => {
    iterationRowBlockSpy.mockClear();
    const rows = [
      {
        kind: 'iteration',
        renderKey: 'rk_iter',
        turnId: 'tn_1',
        lifecycle: 'running',
        rounds: [{
          renderKey: 'rk_round',
          modelSteps: [{
            renderKey: 'rk_model',
            modelCallId: 'mc_intake',
            assistantMessageId: 'msg_intake',
            requestPayloadId: 'req_intake',
            providerRequestPayloadId: 'prov_req_intake',
            responsePayloadId: 'resp_intake',
          }],
          toolCalls: [],
          lifecycleEntries: [],
          hasContent: true,
          finalResponse: false,
        }],
        elicitation: null,
        linkedConversations: [],
        header: { label: 'Execution details (1)', tone: 'running', count: 1 },
        isStreaming: true,
        createdAt: '2025-01-01T00:00:00Z',
      },
    ];

    renderToStaticMarkup(
      React.createElement(ChatFeedFromChatStore, { conversationId: 'c', rowsOverride: rows }),
    );

    expect(iterationRowBlockSpy).toHaveBeenCalledTimes(1);
    // IterationRowBlock consumes the canonical row directly; no adapter, no
    // synthesized "legacy message" prop.
    expect(iterationRowBlockSpy.mock.calls[0][0].iterationRow).toBe(rows[0]);
    expect(iterationRowBlockSpy.mock.calls[0][0].message).toBeUndefined();
  });

  it('renders nothing when the projection is empty', () => {
    const html = renderToStaticMarkup(
      h(ChatFeedFromChatStore, { conversationId: 'c', rowsOverride: [] }),
    );
    expect(html).toBe('');
  });

  it('renders a user bubble with stable render-key data attribute', () => {
    const rows = [
      {
        kind: 'user',
        renderKey: 'rk_u1',
        turnId: 'tn_1',
        messageId: 'msg_1',
        clientRequestId: 'crid_1',
        content: 'hello',
        createdAt: '2025-01-01T00:00:00Z',
      },
    ];
    const html = renderToStaticMarkup(
      h(ChatFeedFromChatStore, { conversationId: 'c', rowsOverride: rows }),
    );
    expect(html).toContain('app-bubble-row-user');
    expect(html).toContain('data-render-key="rk_u1"');
    expect(html).toContain('hello');
  });

  it('renders steering turn in order: [user, iteration, user, user]', () => {
    const rows = [
      { kind: 'user', renderKey: 'rk_u0', turnId: 'tn_1', content: 'first', createdAt: 't0' },
      {
        kind: 'iteration',
        renderKey: 'rk_iter',
        turnId: 'tn_1',
        lifecycle: 'running',
        rounds: [],
        elicitation: null,
        linkedConversations: [],
        header: { label: 'Starting turn…', tone: 'running', count: 0 },
        isStreaming: true,
        createdAt: 't0.1',
      },
      { kind: 'user', renderKey: 'rk_u1', turnId: 'tn_1', content: 'steer-1', createdAt: 't10' },
      { kind: 'user', renderKey: 'rk_u2', turnId: 'tn_1', content: 'steer-2', createdAt: 't20' },
    ];
    const html = renderToStaticMarkup(
      h(ChatFeedFromChatStore, { conversationId: 'c', rowsOverride: rows }),
    );
    const idxU0 = html.indexOf('first');
    const idxIter = html.indexOf('Execution details');
    const idxU1 = html.indexOf('steer-1');
    const idxU2 = html.indexOf('steer-2');
    expect(idxU0).toBeGreaterThanOrEqual(0);
    expect(idxIter).toBeGreaterThan(idxU0);
    expect(idxU1).toBeGreaterThan(idxIter);
    expect(idxU2).toBeGreaterThan(idxU1);
  });

  it('renders standalone assistant bubbles from transcript-backed rows', () => {
    const rows = [
      { kind: 'user', renderKey: 'rk_u0', turnId: 'tn_1', content: 'first', createdAt: 't0' },
      { kind: 'assistant', renderKey: 'rk_a0', turnId: 'tn_1', messageId: 'msg_a0', content: 'PRELIMINARY NOTE', createdAt: 't1' },
      {
        kind: 'iteration',
        renderKey: 'rk_iter',
        turnId: 'tn_1',
        lifecycle: 'completed',
        rounds: [],
        elicitation: null,
        linkedConversations: [],
        header: { label: 'Completed', tone: 'success', count: 0 },
        isStreaming: false,
        createdAt: 't2',
      },
    ];
    const html = renderToStaticMarkup(
      h(ChatFeedFromChatStore, { conversationId: 'c', rowsOverride: rows }),
    );
    const idxUser = html.indexOf('first');
    const idxAssistant = html.indexOf('PRELIMINARY NOTE');
    const idxIter = html.indexOf('Execution details');
    expect(idxAssistant).toBeGreaterThan(idxUser);
    expect(idxIter).toBeGreaterThan(idxAssistant);
    expect(html).toContain('app-bubble-row-assistant');
  });

  it('never produces "(0)" for lifecycle-only turn_started', () => {
    const rows = [
      {
        kind: 'iteration',
        renderKey: 'rk_iter',
        turnId: 'tn_1',
        lifecycle: 'running',
        rounds: [{
          renderKey: 'rk_r0',
          iteration: 0,
          phase: 'main',
          modelSteps: [],
          toolCalls: [],
          lifecycleEntries: [{
            renderKey: 'rk_le1',
            kind: 'turn_started',
            createdAt: '2025-01-01T00:00:00Z',
          }],
          hasContent: false,
          finalResponse: false,
        }],
        elicitation: null,
        linkedConversations: [],
        header: { label: 'Starting turn…', tone: 'running', count: 0 },
        isStreaming: true,
        createdAt: '2025-01-01T00:00:00Z',
      },
    ];
    const html = renderToStaticMarkup(
      h(ChatFeedFromChatStore, { conversationId: 'c', rowsOverride: rows }),
    );
    expect(html).not.toContain('(0)');
    expect(html).toContain('Execution details');
    expect(html).not.toContain('Turn started');
  });
});
