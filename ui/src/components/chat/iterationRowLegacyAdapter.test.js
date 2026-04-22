import { describe, expect, it } from 'vitest';

import { rowToLegacyIterationMessage } from './iterationRowLegacyAdapter.js';

function makeRow(partial = {}) {
  return {
    renderKey: 'rk_iter_1',
    turnId: 'turn_1',
    lifecycle: 'running',
    createdAt: '2026-04-18T10:00:00Z',
    rounds: [],
    linkedConversations: [],
    elicitation: null,
    isStreaming: true,
    ...partial,
  };
}

describe('iterationRowLegacyAdapter', () => {
  it('maps rounds into legacy execution groups and assistant content', () => {
    const message = rowToLegacyIterationMessage(makeRow({
      lifecycle: 'completed',
      isStreaming: false,
      rounds: [{
        renderKey: 'rk_round_1',
        phase: 'sidecar',
        preamble: 'Calling updatePlan.',
        content: 'Done planning.',
        finalResponse: true,
        modelSteps: [{
          renderKey: 'rk_model_1',
          provider: 'openai',
          model: 'gpt-5.4',
          status: 'success',
          startedAt: '2026-04-18T10:00:01Z',
          completedAt: '2026-04-18T10:00:02Z',
        }],
        toolCalls: [{
          renderKey: 'rk_tool_1',
          toolCallId: 'tool_1',
          toolName: 'updatePlan',
          status: 'completed',
        }],
        lifecycleEntries: [],
      }],
    }));

    expect(message.role).toBe('assistant');
    expect(message.status).toBe('completed');
    expect(message.content).toBe('Done planning.');
    expect(message._iterationData.preamble).toEqual({ content: 'Calling updatePlan.' });
    expect(message._iterationData.response.content).toBe('Done planning.');
    expect(message._iterationData.executionGroups).toHaveLength(1);
    expect(message._iterationData.executionGroups[0].phase).toBe('sidecar');
    expect(message._iterationData.executionGroups[0].modelSteps[0].status).toBe('succeeded');
    expect(message._iterationData.executionGroups[0].toolSteps[0].toolName).toBe('updatePlan');
    expect(message._iterationData.isLatestIteration).toBe(false);
  });

  it('preserves execution roles on legacy model and group mappings', () => {
    const message = rowToLegacyIterationMessage(makeRow({
      lifecycle: 'completed',
      isStreaming: false,
      rounds: [{
        renderKey: 'rk_round_intake',
        phase: 'intake',
        executionRole: 'intake',
        preamble: 'Classifying request.',
        content: '',
        finalResponse: false,
        modelSteps: [{
          renderKey: 'rk_model_intake',
          executionRole: 'intake',
          provider: 'openai',
          model: 'gpt-5-mini',
          status: 'completed',
        }],
        toolCalls: [],
        lifecycleEntries: [],
      }],
    }));

    expect(message._iterationData.executionGroups[0].executionRole).toBe('intake');
    expect(message._iterationData.executionGroups[0].modelSteps[0].executionRole).toBe('intake');
  });

  it('falls back to preamble or tool title for visible content while streaming', () => {
    const preambleMessage = rowToLegacyIterationMessage(makeRow({
      rounds: [{
        renderKey: 'rk_round_1',
        phase: 'intake',
        preamble: 'Thinking through it.',
        content: '',
        finalResponse: false,
        modelSteps: [],
        toolCalls: [],
        lifecycleEntries: [],
      }],
    }));
    expect(preambleMessage.content).toBe('Thinking through it.');
    expect(preambleMessage._iterationData.streamContent).toBe('Thinking through it.');

    const toolMessage = rowToLegacyIterationMessage(makeRow({
      rounds: [{
        renderKey: 'rk_round_2',
        phase: 'sidecar',
        preamble: '',
        content: '',
        finalResponse: false,
        modelSteps: [],
        toolCalls: [{
          renderKey: 'rk_tool_2',
          toolCallId: 'tool_2',
          toolName: 'llm/agents/list',
          status: 'running',
        }],
        lifecycleEntries: [],
      }],
    }));
    expect(toolMessage.content).toBe('Calling llm/agents/list.');
    expect(toolMessage._iterationData.streamContent).toBe('Calling llm/agents/list.');
  });

  it('maps linked conversations and elicitation into legacy shape without synthetic lifecycle rows', () => {
    const message = rowToLegacyIterationMessage(makeRow({
      linkedConversations: [{
        conversationId: 'conv_child',
        title: 'Child run',
        status: 'completed',
        response: 'ok',
        updatedAt: '2026-04-18T10:00:04Z',
      }],
      elicitation: {
        elicitationId: 'el_1',
        message: 'Need approval',
        requestedSchema: { type: 'object' },
        status: 'pending',
      },
      rounds: [{
        renderKey: 'rk_round_3',
        phase: 'main',
        preamble: '',
        content: '',
        finalResponse: false,
        modelSteps: [],
        toolCalls: [],
        lifecycleEntries: [],
      }],
    }));

    expect(message.elicitationId).toBe('el_1');
    expect(message.elicitation.message).toBe('Need approval');
    expect(message._iterationData.linkedConversations).toEqual([{
      conversationId: 'conv_child',
      title: 'Child run',
      status: 'completed',
      response: 'ok',
      updatedAt: '2026-04-18T10:00:04Z',
    }]);
    expect(message._iterationData.executionGroups[0].toolSteps).toEqual([]);
  });
});
