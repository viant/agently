import { describe, it, expect } from 'vitest';

import {
  defaultNormalizeMessages,
  normalizeLegacyMessages,
  hasActiveExecutions,
  isTerminalTurnStatus,
  resolveLastTurnStatus,
  lastIndexByRole,
  formatTokensCompact,
  formatCostCompact,
  shouldKeepLocalQueuedPreview,
} from '../../../../forge/src/components/chatLegacyHelpers.js';

describe('chatLegacyHelpers', () => {
  it('defaultNormalizeMessages clones rows', () => {
    const raw = [{ id: '1', content: 'hi' }];
    const normalized = defaultNormalizeMessages(raw);
    expect(normalized).toEqual(raw);
    expect(normalized[0]).not.toBe(raw[0]);
  });

  it('normalizeLegacyMessages preserves synthetic chat rows', () => {
    const raw = [{ id: 'i1', _type: 'iteration', content: 'x' }];
    const result = normalizeLegacyMessages(raw, () => [{ id: 'nope' }]);
    expect(result.synthetic).toBe(true);
    expect(result.rows).toBe(raw);
  });

  it('normalizeLegacyMessages uses provided normalizer for plain rows', () => {
    const raw = [{ id: 'u1', role: 'user', content: 'hi' }];
    const result = normalizeLegacyMessages(raw, () => [{ id: 'norm' }]);
    expect(result.synthetic).toBe(false);
    expect(result.rows).toEqual([{ id: 'norm' }]);
  });

  it('hasActiveExecutions detects active execution groups and statuses', () => {
    expect(hasActiveExecutions([
      {
        executionGroups: [{ status: 'running' }],
      },
    ])).toBe(true);
    expect(hasActiveExecutions([
      {
        _iterationData: {
          executionGroups: [{ toolSteps: [{ status: 'pending' }] }],
        },
      },
    ])).toBe(true);
    expect(hasActiveExecutions([{ status: 'completed' }])).toBe(false);
  });

  it('resolveLastTurnStatus prefers the latest user turn terminal status', () => {
    const messages = [
      { role: 'user', turnId: 't1', content: 'older' },
      { role: 'assistant', turnId: 't1', status: 'completed' },
      { role: 'user', turnId: 't2', content: 'latest' },
      { role: 'assistant', turnId: 't2', turnStatus: 'failed' },
    ];
    expect(resolveLastTurnStatus(messages)).toBe('failed');
  });

  it('lastIndexByRole finds the last matching role', () => {
    expect(lastIndexByRole([{ role: 'user' }, { role: 'assistant' }, { role: 'user' }], 'user')).toBe(2);
    expect(lastIndexByRole([{ role: 'assistant' }], 'user')).toBe(-1);
  });

  it('formatTokensCompact and formatCostCompact format compact summaries', () => {
    expect(formatTokensCompact(950)).toBe('950');
    expect(formatTokensCompact(1500)).toBe('1.5k');
    expect(formatCostCompact(2.345)).toBe('$2.35');
  });

  it('isTerminalTurnStatus covers success/failure/cancel forms', () => {
    expect(isTerminalTurnStatus('completed')).toBe(true);
    expect(isTerminalTurnStatus('cancelled')).toBe(true);
    expect(isTerminalTurnStatus('running')).toBe(false);
  });

  it('shouldKeepLocalQueuedPreview drops active-turn echoes and handled transcript turns', () => {
    const messages = [
      { role: 'user', turnId: 't1', content: 'queued' },
      { role: 'assistant', turnId: 't1', content: 'handled' },
    ];
    expect(shouldKeepLocalQueuedPreview(messages, 'queued', 't1', new Set(), false)).toBe(false);
    expect(shouldKeepLocalQueuedPreview(messages, 'handled', '', new Set(), false)).toBe(false);
  });

  it('shouldKeepLocalQueuedPreview keeps local preview while conversation is still active', () => {
    const messages = [{ role: 'user', turnId: 't1', content: 'queued' }];
    expect(shouldKeepLocalQueuedPreview(messages, 'queued-later', '', new Set(), true)).toBe(true);
  });
});
