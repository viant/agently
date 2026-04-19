import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

vi.mock('forge/components', () => ({
  AvatarIcon: ({ name = '' }) => React.createElement('span', { 'data-avatar-icon': name }),
}));

import IterationRowBlock from './IterationRowBlock.jsx';
import { rowToLegacyIterationMessage } from './iterationRowLegacyAdapter.js';

const h = React.createElement;

function makeRow(partial = {}) {
  return {
    kind: 'iteration',
    renderKey: 'rk_iter_1',
    turnId: 'tn_1',
    lifecycle: 'running',
    rounds: [],
    elicitation: null,
    linkedConversations: [],
    header: { label: 'Starting turn…', tone: 'running', count: 0 },
    isStreaming: true,
    createdAt: '2025-01-01T00:00:00Z',
    ...partial,
  };
}

describe('IterationRowBlock', () => {
  it('renders nothing for a non-iteration row', () => {
    const html = renderToStaticMarkup(h(IterationRowBlock, { message: null }));
    expect(html).toBe('');
  });

  it('renders the header label from row.header — no (0) for lifecycle-only', () => {
    const html = renderToStaticMarkup(h(IterationRowBlock, { message: rowToLegacyIterationMessage(makeRow({
      header: { label: 'Starting turn…', tone: 'running', count: 0 },
    })) }));
    expect(html).toContain('Execution details');
    expect(html).not.toContain('(0)');
  });

  it('applies tone class from header.tone', () => {
    const html = renderToStaticMarkup(h(IterationRowBlock, { message: rowToLegacyIterationMessage(makeRow({
      header: { label: 'Completed', tone: 'success', count: 0 },
      lifecycle: 'completed',
    })) }));
    expect(html).toContain('tone-success');
    expect(html).toContain('Execution details');
  });

  it('does not render lifecycle entries inline inside the card body', () => {
    const row = makeRow({
      header: { label: 'Starting turn…', tone: 'running', count: 0 },
      rounds: [{
        renderKey: 'rk_round_1',
        iteration: 0,
        phase: 'main',
        modelSteps: [],
        toolCalls: [],
        lifecycleEntries: [{
          renderKey: 'rk_le_1',
          kind: 'turn_started',
          createdAt: '2025-01-01T00:00:00Z',
        }],
        hasContent: false,
        finalResponse: false,
      }],
    });
    const html = renderToStaticMarkup(h(IterationRowBlock, { message: rowToLegacyIterationMessage(row) }));
    expect(html).not.toContain('Turn started');
    expect(html).toContain('Execution details');
  });

  it('renders model steps and tool calls inside a round', () => {
    const row = makeRow({
      header: { label: 'Execution details (1)', tone: 'running', count: 1 },
      rounds: [{
        renderKey: 'rk_r1',
        iteration: 0,
        phase: 'main',
        modelSteps: [{
          renderKey: 'rk_ms1',
          provider: 'openai',
          model: 'gpt-5',
          status: 'started',
        }],
        toolCalls: [{
          renderKey: 'rk_tc1',
          toolName: 'search',
          status: 'completed',
        }],
        lifecycleEntries: [],
        hasContent: true,
        finalResponse: false,
      }],
    });
    const html = renderToStaticMarkup(h(IterationRowBlock, { message: rowToLegacyIterationMessage(row) }));
    expect(html).toContain('openai/gpt-5');
    expect(html).toContain('search');
    expect(html).toContain('Execution details');
    expect(html).toContain('Details');
  });

  it('renders intake rounds through the legacy execution-details structure', () => {
    const row = makeRow({
      rounds: [{
        renderKey: 'rk_r1',
        iteration: 0,
        phase: 'intake',
        preamble: 'Checking intake.',
        modelSteps: [{
          renderKey: 'rk_ms1',
          provider: 'openai',
          model: 'gpt-5-mini',
          status: 'completed',
        }],
        toolCalls: [],
        lifecycleEntries: [],
        hasContent: false,
        finalResponse: false,
      }],
    });
    const html = renderToStaticMarkup(h(IterationRowBlock, { message: rowToLegacyIterationMessage(row) }));
    expect(html).toContain('Checking intake.');
    expect(html).toContain('Execution details');
  });

  it('renders the assistant bubble path from preamble/final content', () => {
    const row = makeRow({
      rounds: [{
        renderKey: 'rk_r1',
        iteration: 0,
        phase: 'main',
        preamble: 'Calling updatePlan.',
        modelSteps: [],
        toolCalls: [],
        lifecycleEntries: [],
        hasContent: false,
        finalResponse: false,
      }],
    });
    const html = renderToStaticMarkup(h(IterationRowBlock, { message: rowToLegacyIterationMessage(row) }));
    expect(html).toContain('Calling updatePlan.');
    expect(html).toContain('app-bubble');
  });
});
