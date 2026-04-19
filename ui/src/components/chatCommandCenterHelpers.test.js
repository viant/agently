import { describe, expect, it } from 'vitest';

import {
  normalizeString,
  normalizeBool,
  ensureStringArray,
  defaultAgentTools,
  defaultAgentModel,
  resolveCurrentModel,
  resolveQueuedCount,
  mergeQueuedTurns,
} from '../../../../forge/src/components/chatCommandCenterHelpers.js';

describe('chatCommandCenterHelpers', () => {
  it('normalizes primitive values consistently', () => {
    expect(normalizeString('  hi  ')).toBe('hi');
    expect(normalizeBool('yes')).toBe(true);
    expect(normalizeBool('off')).toBe(false);
    expect(ensureStringArray(['a', 2, ''])).toEqual(['a', '2']);
  });

  it('resolves preferred agent model and tools from snapshot', () => {
    const snapshot = {
      agentOptions: [
        { value: 'steward', label: 'Steward', modelRef: 'gpt-5.4', tools: ['planner'] },
      ],
      agentInfo: {
        steward: { tools: ['planner', 'search'] },
      },
    };

    expect(defaultAgentModel(snapshot, 'steward')).toBe('gpt-5.4');
    expect(defaultAgentTools(snapshot, 'steward')).toEqual(['planner', 'search']);
  });

  it('prefers the selected agent model when current model is empty or default', () => {
    const snapshot = {
      agent: 'steward',
      model: '',
      defaults: { model: 'gpt-5-mini' },
      agentOptions: [{ value: 'steward', modelRef: 'gpt-5.4' }],
    };
    expect(resolveCurrentModel(snapshot)).toBe('gpt-5.4');

    expect(resolveCurrentModel({
      ...snapshot,
      model: 'custom-model',
    })).toBe('custom-model');
  });

  it('normalizes queued count and merges queued turns without duplicates', () => {
    expect(resolveQueuedCount({ queuedCount: '3' })).toBe(3);
    expect(resolveQueuedCount({ queuedTurns: [{ id: 'a' }, { id: 'b' }] })).toBe(2);

    expect(mergeQueuedTurns(
      [{ id: 'local1', preview: 'hello' }, { id: 'local1', preview: 'hello again' }],
      [{ id: 'server1', preview: 'world' }, { id: 'server1', preview: 'world again' }],
    )).toEqual([
      { id: 'local1', preview: 'hello' },
      { id: 'server1', preview: 'world' },
    ]);
  });
});
