import { describe, expect, it } from 'vitest';
import { delegatedAgentId, delegatedAgentLabel, displayStepTitle, isAgentRunTool } from './toolPresentation';

describe('toolPresentation', () => {
  it('keeps delegated agent ids canonical for llm/agents:run steps', () => {
    const step = {
      toolName: 'llm/agents:run',
      requestPayload: JSON.stringify({ agentId: 'agent_selector' }),
    };

    expect(isAgentRunTool(step)).toBe(true);
    expect(delegatedAgentId(step)).toBe('agent_selector');
    expect(delegatedAgentLabel(step)).toBe('agent_selector');
    expect(displayStepTitle(step)).toBe('agent_selector');
  });

  it('normalizes model ids into readable titles', () => {
    expect(displayStepTitle({ kind: 'model', provider: 'openai', model: 'gpt-5_4' })).toBe('openai/gpt-5.4');
    expect(displayStepTitle({ kind: 'model', model: 'openai_gpt-5_4' })).toBe('gpt-5.4');
  });

  it('derives model titles from request payloads when direct fields are absent', () => {
    const step = {
      kind: 'model',
      requestPayload: JSON.stringify({ provider: 'openai', model: 'gpt-5_4' }),
    };
    expect(displayStepTitle(step)).toBe('openai/gpt-5.4');
  });
});
