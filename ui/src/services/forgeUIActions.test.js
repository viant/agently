import { beforeEach, describe, it, expect, vi } from 'vitest';

import {
  connectForgeUIActionsToChat,
  connectForgeUIActionsToCallbacksOrChat,
  dispatchForgeUIAction,
} from './forgeUIActions';

async function withFakeWindow(fn) {
  const previousWindow = globalThis.window;
  const eventTarget = new EventTarget();
  globalThis.window = eventTarget;
  globalThis.window.addEventListener = eventTarget.addEventListener.bind(eventTarget);
  globalThis.window.removeEventListener = eventTarget.removeEventListener.bind(eventTarget);
  globalThis.window.dispatchEvent = eventTarget.dispatchEvent.bind(eventTarget);
  try {
    await fn();
  } finally {
    globalThis.window = previousWindow;
  }
}

describe('forgeUIActions', () => {
  it('bridges forge-ui-action into submitMessage with a runtime-safe summary', async () => {
    await withFakeWindow(async () => {
      const submitMessage = vi.fn(async () => {});
      const context = { id: 'ctx-1' };
      const disconnect = connectForgeUIActionsToChat(submitMessage, () => context);

      dispatchForgeUIAction({
        eventName: 'planner_table_submit',
        tableId: 'site-review',
        dataSourceRef: 'recommended_sites',
        selectedRows: [{ site_id: 101 }],
        unselectedRows: [{ site_id: 202 }],
        changedRows: [{ site_id: 202 }],
        finalDataSourceSnapshot: [{ site_id: 101 }, { site_id: 202 }],
        callback: {
          type: 'custom_callback',
          eventName: 'planner_table_submit',
          target: 'foreground',
        },
      });

      await Promise.resolve();
      await Promise.resolve();

      expect(submitMessage).toHaveBeenCalledTimes(1);
      const call = submitMessage.mock.calls[0][0];
      expect(call.context).toBe(context);
      expect(String(call.message || '')).toContain('Forge UI callback: planner_table_submit');
      expect(String(call.message || '')).toContain('selected=1');
      expect(String(call.message || '')).toContain('unselected=1');
      expect(String(call.message || '')).toContain('changed=1');
      expect(String(call.message || '')).toContain('"finalDataSourceSnapshot"');

      disconnect();
    });
  });
});

describe('forgeUIActions.connectForgeUIActionsToCallbacksOrChat', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('routes to the workspace callback dispatcher on success and posts a confirmation', async () => {
    await withFakeWindow(async () => {
      const submitMessage = vi.fn(async () => {});
      const dispatchCallback = vi.fn(async () => ({ ok: true, tool: 'steward-SaveRecommendation' }));
      const context = {
        conversationId: 'conv-42',
        agencyId: 5337,
        adOrderId: 987654,
      };
      const disconnect = connectForgeUIActionsToCallbacksOrChat(submitMessage, () => context, dispatchCallback);

      dispatchForgeUIAction({
        eventName: 'spo_planner_submit',
        selectedRows: [{ site_id: 101, action: 'CUT' }],
        callback: { type: 'custom_callback', eventName: 'spo_planner_submit', target: 'foreground' },
      });

      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();

      expect(dispatchCallback).toHaveBeenCalledTimes(1);
      const input = dispatchCallback.mock.calls[0][0];
      expect(input.eventName).toBe('spo_planner_submit');
      expect(input.conversationId).toBe('conv-42');
      expect(input.payload.selectedRows).toEqual([{ site_id: 101, action: 'CUT' }]);
      expect(input.context).toEqual({ agencyId: 5337, adOrderId: 987654 });

      expect(submitMessage).toHaveBeenCalledTimes(1);
      const msg = String(submitMessage.mock.calls[0][0].message || '');
      expect(msg).toContain('Forge UI callback dispatched: spo_planner_submit');
      expect(msg).toContain('steward-SaveRecommendation');

      disconnect();
    });
  });

  it('falls back to the legacy chat summary when no callback is registered (notFound)', async () => {
    await withFakeWindow(async () => {
      const submitMessage = vi.fn(async () => {});
      const dispatchCallback = vi.fn(async () => ({ ok: false, notFound: true, status: 404 }));
      const context = { conversationId: 'conv-1' };
      const disconnect = connectForgeUIActionsToCallbacksOrChat(submitMessage, () => context, dispatchCallback);

      dispatchForgeUIAction({
        eventName: 'planner_table_submit',
        selectedRows: [{ site_id: 1 }],
        callback: { type: 'custom_callback', eventName: 'planner_table_submit', target: 'foreground' },
      });

      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();

      expect(dispatchCallback).toHaveBeenCalledTimes(1);
      expect(submitMessage).toHaveBeenCalledTimes(1);
      const msg = String(submitMessage.mock.calls[0][0].message || '');
      // Legacy fallback uses the summariser — NOT the "dispatched:" confirmation.
      expect(msg).toContain('Forge UI callback: planner_table_submit');
      expect(msg).toContain('selected=1');

      disconnect();
    });
  });

  it('falls back to chat when dispatch fails with a non-notFound error', async () => {
    await withFakeWindow(async () => {
      const submitMessage = vi.fn(async () => {});
      const dispatchCallback = vi.fn(async () => ({ ok: false, notFound: false, error: 'template broken' }));
      const context = { conversationId: 'conv-1' };
      vi.spyOn(console, 'error').mockImplementation(() => {});
      const disconnect = connectForgeUIActionsToCallbacksOrChat(submitMessage, () => context, dispatchCallback);

      dispatchForgeUIAction({
        eventName: 'some_event',
        selectedRows: [],
        callback: { type: 'custom_callback', eventName: 'some_event', target: 'foreground' },
      });

      await Promise.resolve();
      await Promise.resolve();

      expect(submitMessage).toHaveBeenCalledTimes(1);
      expect(console.error).toHaveBeenCalled();

      disconnect();
    });
  });

  it('extracts conversationId via context.Context("conversations").handlers.dataSource.peekFormData().id', async () => {
    await withFakeWindow(async () => {
      const submitMessage = vi.fn(async () => {});
      const dispatchCallback = vi.fn(async () => ({ ok: true, tool: 'x' }));
      const context = {
        Context: (key) => key === 'conversations' ? {
          handlers: { dataSource: { peekFormData: () => ({ id: 'conv-from-form' }) } },
        } : null,
      };
      const disconnect = connectForgeUIActionsToCallbacksOrChat(submitMessage, () => context, dispatchCallback);

      dispatchForgeUIAction({ eventName: 'spo_planner_submit', callback: { eventName: 'spo_planner_submit' } });

      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();

      expect(dispatchCallback).toHaveBeenCalledTimes(1);
      expect(dispatchCallback.mock.calls[0][0].conversationId).toBe('conv-from-form');

      disconnect();
    });
  });
});
