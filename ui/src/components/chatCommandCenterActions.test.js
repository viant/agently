import { describe, expect, it, vi } from 'vitest';

import {
  applyAgentSelection,
  applyModelSelection,
  applyReasoningSelection,
  applyToolsSelection,
  applyAutoSelectToolsSelection,
} from '../../../../forge/src/components/chatCommandCenterActions.js';

function makeDataSource(initial = {}) {
  const state = { ...initial };
  return {
    peekFormData: vi.fn(() => state),
    setFormField: vi.fn(({ item, value }) => {
      state[item.id] = value;
    }),
    setFormData: vi.fn(({ values }) => {
      Object.assign(state, values);
    }),
    state,
  };
}

function makeContext(convDS) {
  return {
    resources: {},
    Context: vi.fn((name) => {
      if (name === 'conversations') {
        return { handlers: { dataSource: convDS } };
      }
      return null;
    }),
  };
}

describe('chatCommandCenterActions', () => {
  it('applies agent selection and mirrors key fields to conversations', () => {
    const metaDS = makeDataSource({
      defaults: { model: 'gpt-5-mini' },
      agentOptions: [{ value: 'steward', label: 'Steward', modelRef: 'gpt-5.4', name: 'Steward', tools: ['planner'] }],
      agentInfo: { steward: { tools: ['planner', 'search'] } },
    });
    const convDS = makeDataSource();

    applyAgentSelection({
      agentID: 'steward',
      metaDS,
      metaSnapshot: metaDS.state,
      context: makeContext(convDS),
    });

    expect(metaDS.state.agent).toBe('steward');
    expect(metaDS.state.model).toBe('gpt-5.4');
    expect(metaDS.state.tool).toBeUndefined();
    expect(convDS.state.agent).toBe('steward');
    expect(convDS.state.agentName).toBe('Steward');
    expect(convDS.state.model).toBe('gpt-5.4');
  });

  it('clears agent selection back to defaults', () => {
    const metaDS = makeDataSource({
      defaults: { model: 'gpt-5-mini' },
      agent: 'steward',
      model: 'gpt-5.4',
    });
    const convDS = makeDataSource();

    applyAgentSelection({
      agentID: '',
      metaDS,
      metaSnapshot: metaDS.state,
      context: makeContext(convDS),
    });

    expect(metaDS.state.agent).toBe('');
    expect(metaDS.state.model).toBe('gpt-5-mini');
    expect(convDS.state.agent).toBe('');
    expect(convDS.state.model).toBe('gpt-5-mini');
  });

  it('applies model, reasoning, tools, and auto-select settings', () => {
    const metaDS = makeDataSource();
    const convDS = makeDataSource();
    const context = makeContext(convDS);

    applyModelSelection({ modelID: 'gpt-5.4', metaDS, context });
    applyReasoningSelection({ effort: 'high', metaDS });
    applyToolsSelection({ toolNames: ['prompt/list', 'llm/agents/list'], metaDS });
    applyAutoSelectToolsSelection({ enabled: true, metaDS, context });

    expect(metaDS.state.model).toBe('gpt-5.4');
    expect(convDS.state.model).toBe('gpt-5.4');
    expect(metaDS.state.reasoningEffort).toBe('high');
    expect(metaDS.state.tool).toEqual(['prompt/list', 'llm/agents/list']);
    expect(metaDS.state.autoSelectTools).toBe(true);
    expect(convDS.state.autoSelectTools).toBe(true);
    expect(context.resources.autoSelectToolsTouched).toBe(true);
  });
});
