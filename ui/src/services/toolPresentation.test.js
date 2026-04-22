import { describe, expect, it } from 'vitest';
import { delegatedAgentId, delegatedAgentLabel, displayStepTitle, executionRoleLabel, isAgentRunTool } from './toolPresentation';

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

  it('treats llm/agents:start as delegated-agent execution and preserves the agent label', () => {
    const step = {
      toolName: 'llm/agents:start',
      requestPayload: JSON.stringify({ agentId: 'guardian' }),
    };

    expect(isAgentRunTool(step)).toBe(true);
    expect(delegatedAgentId(step)).toBe('guardian');
    expect(displayStepTitle(step)).toBe('guardian');
  });

  it('does not treat llm/agents:status as an open-thread/delegated-run tool', () => {
    const step = {
      toolName: 'llm/agents:status',
      requestPayload: JSON.stringify({ conversationId: 'child-1' }),
    };

    expect(isAgentRunTool(step)).toBe(false);
    expect(delegatedAgentId(step)).toBe('');
    expect(displayStepTitle(step)).toBe('llm/agents:status');
  });

  it('normalizes model ids into readable titles', () => {
    expect(displayStepTitle({ kind: 'model', provider: 'openai', model: 'gpt-5_4' })).toBe('openai/gpt-5.4');
    expect(displayStepTitle({ kind: 'model', model: 'openai_gpt-5_4' })).toBe('gpt-5.4');
  });

  it('uses explicit execution roles for synthetic narrator/intake model rows', () => {
    expect(displayStepTitle({ kind: 'model', executionRole: 'narrator' })).toBe('Narrator');
    expect(displayStepTitle({ kind: 'model', executionRole: 'intake' })).toBe('Intake');
  });

  it('derives model titles from request payloads when direct fields are absent', () => {
    const step = {
      kind: 'model',
      requestPayload: JSON.stringify({ provider: 'openai', model: 'gpt-5_4' }),
    };
    expect(displayStepTitle(step)).toBe('openai/gpt-5.4');
  });

  it('classifies semantic execution roles for visible model/tool rows', () => {
    expect(executionRoleLabel({ executionRole: 'react' })).toBe('⌬');
    expect(executionRoleLabel({ executionRole: 'worker' })).toBe('⚙');
    expect(executionRoleLabel({ executionRole: 'intake' })).toBe('⇢');
    expect(executionRoleLabel({ executionRole: 'router' })).toBe('🧭');
    expect(executionRoleLabel({ executionRole: 'narrator' })).toBe('✍');
    expect(executionRoleLabel({ executionRole: 'summary' })).toBe('≡');
    expect(executionRoleLabel({ kind: 'model', phase: 'intake' })).toBe('');
    expect(executionRoleLabel({ kind: 'tool', toolName: 'llm/agents:start', requestPayload: JSON.stringify({ agentId: 'coder' }) })).toBe('');
  });
});
