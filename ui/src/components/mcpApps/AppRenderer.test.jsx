import { describe, expect, it } from 'vitest';
import { acceptGuestEnvelopeEvent, buildApprovalOutcomeToolResultEnvelope, reduceApprovalOutcomeState, reduceToolResultState } from './AppRenderer.jsx';

describe('AppRenderer', () => {
  it('marks queued tool results as pending approval when an approval id is present', () => {
    const next = reduceToolResultState(
      { conversationId: 'conv-1' },
      {
        params: {
          content: [{ type: 'text', text: 'queued for approval' }],
          structuredContent: {
            conversationId: 'conv-2',
            status: 'queued',
            approval: { id: 'approval-42' },
          },
        },
      },
      'Remote verifier',
    );
    expect(next.conversationId).toBe('conv-2');
    expect(next.toolCallStatus).toBe('queued');
    expect(next.approvalId).toBe('approval-42');
    expect(next.pendingApproval).toBe(true);
    expect(next.pendingApprovalTitle).toBe('Remote verifier');
  });

  it('applies approval outcomes from backend truth and clears pending state', () => {
    const next = reduceApprovalOutcomeState(
      {
        conversationId: 'conv-queued',
        toolCallStatus: 'queued',
        approvalId: 'approval-42',
        pendingApproval: true,
        pendingApprovalTitle: 'Remote verifier',
      },
      {
        approvalId: 'approval-42',
        status: 'executed',
        conversationId: 'conv-final',
        result: '{"values":{"HOME":"/tmp"}}',
      },
    );
    expect(next.conversationId).toBe('conv-final');
    expect(next.toolCallStatus).toBe('executed');
    expect(next.pendingApproval).toBe(false);
    expect(next.pendingApprovalTitle).toBe('');
    expect(next.toolResult).toBe('{"values":{"HOME":"/tmp"}}');
  });

  it('builds a canonical tool-result envelope from an approval outcome', () => {
    expect(buildApprovalOutcomeToolResultEnvelope({
      windowId: 'w1',
      resourceUri: 'ui://mcpuiverify/demo/verify_widget',
      outcome: {
        approvalId: 'approval-42',
        action: 'approve',
        decision: 'approve',
        status: 'executed',
        conversationId: 'conv-1',
        turnId: 'turn-1',
        messageId: 'msg-1',
        toolName: 'system/os/getEnv',
        result: '{"values":{"HOME":"/tmp"}}',
      },
    })).toEqual({
      version: '1.0.0',
      method: 'mcpui:tool-result',
      params: {
        windowId: 'w1',
        resourceUri: 'ui://mcpuiverify/demo/verify_widget',
        toolName: 'system/os/getEnv',
        content: [{ type: 'text', text: '{"values":{"HOME":"/tmp"}}' }],
        structuredContent: {
          approval: { id: 'approval-42' },
          action: 'approve',
          decision: 'approve',
          status: 'executed',
          conversationId: 'conv-1',
          turnId: 'turn-1',
          messageId: 'msg-1',
          toolName: 'system/os/getEnv',
          result: '{"values":{"HOME":"/tmp"}}',
          errorMessage: '',
        },
        _meta: {},
        protocolVersion: '1.0.0',
      },
    });
  });

  it('accepts opaque-origin srcdoc guest messages only from the bound source and exact binding ids', () => {
    const boundSource = {};
    expect(acceptGuestEnvelopeEvent({
      source: boundSource,
      origin: 'null',
      data: {
        version: '1.0.0',
        method: 'mcpui:message',
        params: {
          windowId: 'w1',
          resourceUri: 'ui://demo/widget',
          content: 'hello',
        },
      },
    }, {
      targetWindow: boundSource,
      windowId: 'w1',
      resourceUri: 'ui://demo/widget',
      sandbox: 'allow-scripts',
      pageOrigin: 'http://127.0.0.1:10071',
    })).toMatchObject({ ok: true });

    expect(acceptGuestEnvelopeEvent({
      source: {},
      origin: 'null',
      data: {
        version: '1.0.0',
        method: 'mcpui:message',
        params: {
          windowId: 'w1',
          resourceUri: 'ui://demo/widget',
          content: 'hello',
        },
      },
    }, {
      targetWindow: boundSource,
      windowId: 'w1',
      resourceUri: 'ui://demo/widget',
      sandbox: 'allow-scripts',
      pageOrigin: 'http://127.0.0.1:10071',
    })).toEqual({ ok: false, error: 'source mismatch' });
  });

  it('requires exact same-origin event.origin for same-origin route frames', () => {
    const boundSource = {};
    expect(acceptGuestEnvelopeEvent({
      source: boundSource,
      origin: 'http://127.0.0.1:10071',
      data: {
        version: '1.0.0',
        method: 'mcpui:message',
        params: {
          windowId: 'w2',
          resourceUri: 'ui://demo/route',
          content: 'ok',
        },
      },
    }, {
      targetWindow: boundSource,
      windowId: 'w2',
      resourceUri: 'ui://demo/route',
      sandbox: 'allow-scripts allow-same-origin',
      pageOrigin: 'http://127.0.0.1:10071',
    })).toMatchObject({ ok: true });

    expect(acceptGuestEnvelopeEvent({
      source: boundSource,
      origin: 'http://evil.example',
      data: {
        version: '1.0.0',
        method: 'mcpui:message',
        params: {
          windowId: 'w2',
          resourceUri: 'ui://demo/route',
          content: 'nope',
        },
      },
    }, {
      targetWindow: boundSource,
      windowId: 'w2',
      resourceUri: 'ui://demo/route',
      sandbox: 'allow-scripts allow-same-origin',
      pageOrigin: 'http://127.0.0.1:10071',
    })).toEqual({ ok: false, error: 'origin mismatch' });
  });

  it('rejects mismatched windowId and resourceUri echoes even from the bound source', () => {
    const boundSource = {};
    expect(acceptGuestEnvelopeEvent({
      source: boundSource,
      origin: 'null',
      data: {
        version: '1.0.0',
        method: 'mcpui:message',
        params: {
          windowId: 'wrong-window',
          resourceUri: 'ui://demo/widget',
          content: 'bad',
        },
      },
    }, {
      targetWindow: boundSource,
      windowId: 'w3',
      resourceUri: 'ui://demo/widget',
      sandbox: 'allow-scripts',
      pageOrigin: 'http://127.0.0.1:10071',
    })).toEqual({ ok: false, error: 'windowId mismatch' });

    expect(acceptGuestEnvelopeEvent({
      source: boundSource,
      origin: 'null',
      data: {
        version: '1.0.0',
        method: 'mcpui:message',
        params: {
          windowId: 'w3',
          resourceUri: 'ui://demo/other',
          content: 'bad',
        },
      },
    }, {
      targetWindow: boundSource,
      windowId: 'w3',
      resourceUri: 'ui://demo/widget',
      sandbox: 'allow-scripts',
      pageOrigin: 'http://127.0.0.1:10071',
    })).toEqual({ ok: false, error: 'resourceUri mismatch' });
  });
});
