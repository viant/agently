import { describe, expect, it, vi } from 'vitest';
import {
  MCPUI_APPROVAL_OUTCOME_EVENT,
  MCPUI_APPROVAL_REQUEST_EVENT,
  dispatchMCPUIApprovalOutcome,
  dispatchMCPUIApprovalRequest,
  normalizeMCPUIApprovalOutcome,
  normalizeMCPUIApprovalRequest,
} from './approvalEvents.js';

describe('mcpApps/approvalEvents', () => {
  it('normalizes valid approval requests exactly', () => {
    expect(normalizeMCPUIApprovalRequest({
      approvalId: ' approval-1 ',
      resourceUri: ' ui://mcpuiverify/demo/verify_widget ',
      toolName: ' system/os:getEnv ',
      title: ' Remote verifier ',
    })).toEqual({
      approvalId: 'approval-1',
      resourceUri: 'ui://mcpuiverify/demo/verify_widget',
      toolName: 'system/os:getEnv',
      title: 'Remote verifier',
    });
  });

  it('rejects approval requests without an explicit id', () => {
    expect(normalizeMCPUIApprovalRequest({ title: 'x' })).toBe(null);
  });

  it('dispatches a browser event for valid approval requests', () => {
    const dispatchEvent = vi.fn();
    const originalWindow = globalThis.window;
    globalThis.window = { dispatchEvent };
    try {
      expect(dispatchMCPUIApprovalRequest({ approvalId: 'approval-1' })).toBe(true);
      expect(dispatchEvent).toHaveBeenCalledTimes(1);
      expect(dispatchEvent.mock.calls[0][0].type).toBe(MCPUI_APPROVAL_REQUEST_EVENT);
    } finally {
      globalThis.window = originalWindow;
    }
  });

  it('normalizes and dispatches approval outcomes exactly', () => {
    expect(normalizeMCPUIApprovalOutcome({
      approvalId: ' approval-1 ',
      action: ' approve ',
      status: ' executed ',
      result: ' ok ',
    })).toEqual({
      approvalId: 'approval-1',
      action: 'approve',
      status: 'executed',
      decision: '',
      conversationId: '',
      turnId: '',
      messageId: '',
      toolName: '',
      result: 'ok',
      errorMessage: '',
    });

    const dispatchEvent = vi.fn();
    const originalWindow = globalThis.window;
    globalThis.window = { dispatchEvent };
    try {
      expect(dispatchMCPUIApprovalOutcome({ approvalId: 'approval-1', status: 'executed' })).toBe(true);
      expect(dispatchEvent).toHaveBeenCalledTimes(1);
      expect(dispatchEvent.mock.calls[0][0].type).toBe(MCPUI_APPROVAL_OUTCOME_EVENT);
    } finally {
      globalThis.window = originalWindow;
    }
  });
});
