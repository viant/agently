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
});
