import { describe, expect, it, vi, beforeEach } from 'vitest';

const selectedWindowId = { value: 'window-1' };
const getWindowContext = vi.fn();

vi.mock('forge/core', () => ({
  getWindowContext,
  selectedWindowId
}));

describe('approvalCallbacks', () => {
  beforeEach(() => {
    getWindowContext.mockReset();
  });

  it('executes matching callbacks and merges edited fields', async () => {
    const handler = vi.fn().mockResolvedValue({
      editedFields: { names: ['HOME', 'PATH'] }
    });
    getWindowContext.mockReturnValue({
      lookupHandler(name) {
        if (name === 'approval.filter') return handler;
        return null;
      }
    });

    const { executeApprovalCallbacks } = await import('./approvalCallbacks.js');
    const result = await executeApprovalCallbacks({
      meta: {
        forge: {
          callbacks: [
            { event: 'approve', handler: 'approval.filter' },
            { event: 'reject', handler: 'approval.reject' }
          ]
        }
      },
      event: 'approve',
      payload: {
        approval: { toolName: 'system/os/getEnv' },
        editedFields: { names: ['HOME', 'SHELL', 'PATH'] },
        originalArgs: { names: ['HOME', 'SHELL', 'PATH'] }
      }
    });

    expect(handler).toHaveBeenCalledTimes(1);
    expect(result.editedFields).toEqual({ names: ['HOME', 'PATH'] });
  });
});
