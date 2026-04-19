import { describe, expect, it, vi } from 'vitest';

import {
  buildUsageSummary,
  buildUsageTooltip,
  buildToolsLabel,
  clearCommandCenterChip,
} from '../../../../forge/src/components/chatCommandCenterViewState.js';

describe('chatCommandCenterViewState', () => {
  it('builds compact usage summary and tooltip text', () => {
    expect(buildUsageSummary({
      cost: 12.34,
      totalTokens: 15300,
    })).toBe('Cost $12.34 • Tokens 15.3k');

    expect(buildUsageTooltip({
      costText: '$12.34',
      tokensWithCacheText: '15.3k (18.1k incl. cache)',
    })).toBe('Cost: $12.34\nTokens: 15.3k (18.1k incl. cache)');
  });

  it('builds a short tools label', () => {
    expect(buildToolsLabel([])).toBe('');
    expect(buildToolsLabel(['prompt/list'])).toBe('prompt/list');
    expect(buildToolsLabel(['a', 'b', 'c'])).toBe('3 tools');
  });

  it('clears model/tools/reasoning chips through the provided handlers', () => {
    const handleAgentChange = vi.fn();
    const handleModelChange = vi.fn();
    const handleToolsChange = vi.fn();
    const handleReasoningChange = vi.fn();

    const base = {
      commandCenterDefaults: { agent: 'steward', model: 'gpt-5-mini' },
      currentAgent: 'steward',
      currentModel: 'gpt-5.4',
      metaSnapshot: {
        agentOptions: [{ value: 'steward', modelRef: 'gpt-5.4', tools: ['prompt/list', 'llm/agents/list'] }],
      },
      events: null,
      metaCtx: null,
      context: {},
      handleAgentChange,
      handleModelChange,
      handleToolsChange,
      handleReasoningChange,
    };

    expect(clearCommandCenterChip({ ...base, chip: { id: 'model' } })).toBe(true);
    expect(handleModelChange).toHaveBeenCalledWith('gpt-5.4');

    expect(clearCommandCenterChip({ ...base, chip: { id: 'tools' } })).toBe(true);
    expect(handleToolsChange).toHaveBeenCalledWith(['prompt/list', 'llm/agents/list']);

    expect(clearCommandCenterChip({ ...base, chip: { id: 'reasoningEffort' } })).toBe(true);
    expect(handleReasoningChange).toHaveBeenCalledWith('');
  });

  it('defers to onClearOverride when defined', () => {
    const execute = vi.fn();
    const events = {
      onClearOverride: {
        isDefined: () => true,
        execute,
      },
    };

    expect(clearCommandCenterChip({
      chip: { id: 'agent' },
      commandCenterDefaults: {},
      currentAgent: 'steward',
      currentModel: 'gpt-5.4',
      metaSnapshot: {},
      events,
      metaCtx: { id: 'meta' },
      context: { id: 'root' },
      handleAgentChange: vi.fn(),
      handleModelChange: vi.fn(),
      handleToolsChange: vi.fn(),
      handleReasoningChange: vi.fn(),
    })).toBe(true);

    expect(execute).toHaveBeenCalled();
  });
});
