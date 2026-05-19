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
      };
      const disconnect = connectForgeUIActionsToCallbacksOrChat(submitMessage, () => context, dispatchCallback);

      dispatchForgeUIAction({
        eventName: 'spo_planner_submit',
        selectedRows: [{ site_id: 101, action: 'CUT' }],
        callbackContext: {
          agencyId: 5337,
          adOrderId: 987654,
          stageLifecycle: {
            currentStage: 'validate',
            validationStage: 'validate',
            successStage: 'execute',
            followUpStage: 'follow_up',
          },
        },
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
      expect(input.payload.plannerSubmit).toBeUndefined();
      expect(input.context).toEqual({
        agencyId: 5337,
        adOrderId: 987654,
        stageLifecycle: {
          currentStage: 'validate',
          validationStage: 'validate',
          successStage: 'execute',
          followUpStage: 'follow_up',
        },
      });

      expect(submitMessage).toHaveBeenCalledTimes(1);
      const msg = String(submitMessage.mock.calls[0][0].message || '');
      expect(msg).toContain('Forge UI callback dispatched: spo_planner_submit');
      expect(msg).toContain('steward-SaveRecommendation');
      expect(msg).toContain('Recommendation lifecycle advanced: Validate -> Execute.');
      expect(msg).toContain('Next stage: Follow Up.');

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

  it('routes llm_event planner submits directly back to chat with masked display text and structured payload', async () => {
    await withFakeWindow(async () => {
      const submitMessage = vi.fn(async () => {});
      const dispatchCallback = vi.fn(async () => ({ ok: true, tool: 'steward-RecommendationPatch' }));
      const context = { conversationId: 'conv-site' };
      const disconnect = connectForgeUIActionsToCallbacksOrChat(submitMessage, () => context, dispatchCallback);

      dispatchForgeUIAction({
        eventName: 'site_list_planner_submit',
        tableId: 'site-review',
        selectionField: 'selected',
        columns: [
          { key: 'site_id', label: 'Site ID' },
          { key: 'recommendation', label: 'Recommendation' },
        ],
        selectedRows: [{ publisher_id: 37, site_id: 3945613211, audience_id: 7301206, relationship: 'target', recommendation: 'ADD' }],
        plannerSubmit: {
          domain: 'site_list',
          submitIntent: 'submit_selected',
          selectedKeys: ['publisher_id', 'site_id', 'audience_id', 'relationship', 'recommendation'],
          toolGuidance: {
            tool: 'steward-RecommendationPatch',
            toolBundle: 'analyst-sitelist-tools',
            useSelectedRowsOnly: true,
          },
        },
        callbackContext: {
          displayQuery: 'Submit selected site recommendations.',
        },
        callback: { type: 'llm_event', eventName: 'site_list_planner_submit', target: 'foreground' },
      });

      await Promise.resolve();
      await Promise.resolve();

      expect(dispatchCallback).not.toHaveBeenCalled();
      expect(submitMessage).toHaveBeenCalledTimes(1);
      const call = submitMessage.mock.calls[0][0];
      expect(call.context).toBe(context);
      expect(call.message).toEqual({
        content: 'Execute the planner submit event using the structured plannerSubmitEvent context. If plannerSubmitEvent.plannerSubmit.toolGuidance.tool is present, attempt that guided tool or its review flow before answering. Do not summarize selected rows in prose unless execution is blocked after attempting the guided path.',
        displayQuery: 'Submit selected site recommendations.',
        tools: ['steward-RecommendationPatch'],
        toolBundles: ['analyst-sitelist-tools'],
        context: {
          plannerSubmitEvent: {
            eventName: 'site_list_planner_submit',
            tableId: 'site-review',
            selectionField: 'selected',
            columns: [
              { key: 'site_id', label: 'Site ID' },
              { key: 'recommendation', label: 'Recommendation' },
            ],
            plannerSubmit: {
              domain: 'site_list',
              submitIntent: 'submit_selected',
              selectedKeys: ['publisher_id', 'site_id', 'audience_id', 'relationship', 'recommendation'],
              toolGuidance: {
                tool: 'steward-RecommendationPatch',
                toolBundle: 'analyst-sitelist-tools',
                useSelectedRowsOnly: true,
              },
            },
            selectedRows: [{ publisher_id: 37, site_id: 3945613211, audience_id: 7301206, relationship: 'target', recommendation: 'ADD' }],
          },
        },
      });

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

  it('posts a blocked confirmation instead of falling back to legacy chat summary', async () => {
    await withFakeWindow(async () => {
      const submitMessage = vi.fn(async () => {});
      const dispatchCallback = vi.fn(async () => ({ ok: false, blocked: true, error: 'blocked by evaluator verdict' }));
      const context = { conversationId: 'conv-1' };
      const disconnect = connectForgeUIActionsToCallbacksOrChat(submitMessage, () => context, dispatchCallback);

      dispatchForgeUIAction({
        eventName: 'freq_cap_planner_submit',
        selectedRows: [{ ad_order_id: 2661447, action: 'KEEP' }],
        callbackContext: {
          stageLifecycle: {
            currentStage: 'validate',
            validationStage: 'validate',
            successStage: 'execute',
            followUpStage: 'follow_up',
          },
        },
        callback: { type: 'custom_callback', eventName: 'freq_cap_planner_submit', target: 'foreground' },
      });

      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();

      expect(dispatchCallback).toHaveBeenCalledTimes(1);
      expect(submitMessage).toHaveBeenCalledTimes(1);
      const msg = String(submitMessage.mock.calls[0][0].message || '');
      expect(msg).toContain('Forge UI callback blocked');
      expect(msg).toContain('blocked by evaluator verdict');
      expect(msg).toContain('Recommendation lifecycle blocked at Validate');

      disconnect();
    });
  });

  it('preserves alternate preview actions and callback event names', async () => {
    await withFakeWindow(async () => {
      const submitMessage = vi.fn(async () => {});
      const dispatchCallback = vi.fn(async () => ({ ok: true, tool: 'preview-only' }));
      const context = { conversationId: 'conv-preview' };
      const disconnect = connectForgeUIActionsToCallbacksOrChat(submitMessage, () => context, dispatchCallback);

      dispatchForgeUIAction({
        eventName: 'freq_cap_planner_preview',
        actionId: 'preview-frequency-cap',
        selectedRows: [{ ad_order_id: 2661447, action: 'KEEP' }],
        callbackContext: {
          stageLifecycle: {
            currentStage: 'review',
            validationStage: 'validate',
            successStage: 'execute',
            followUpStage: 'follow_up',
          },
        },
        callback: { type: 'custom_callback', eventName: 'freq_cap_planner_preview', target: 'foreground' },
      });

      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();

      expect(dispatchCallback).toHaveBeenCalledTimes(1);
      expect(dispatchCallback.mock.calls[0][0].eventName).toBe('freq_cap_planner_preview');
      const msg = String(submitMessage.mock.calls[0][0].message || '');
      expect(msg).toContain('Forge UI callback dispatched: freq_cap_planner_preview');
      disconnect();
    });
  });

  it('does not invent callback context from ambient forge state when none is provided', async () => {
    await withFakeWindow(async () => {
      const submitMessage = vi.fn(async () => {});
      const dispatchCallback = vi.fn(async () => ({ ok: true, tool: 'x' }));
      const context = {
        conversationId: 'conv-42',
        agencyId: 5337,
        adOrderId: 987654,
      };
      const disconnect = connectForgeUIActionsToCallbacksOrChat(submitMessage, () => context, dispatchCallback);

      dispatchForgeUIAction({
        eventName: 'spo_planner_submit',
        selectedRows: [{ site_id: 101 }],
        callback: { type: 'custom_callback', eventName: 'spo_planner_submit', target: 'foreground' },
      });

      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();

      expect(dispatchCallback).toHaveBeenCalledTimes(1);
      expect(dispatchCallback.mock.calls[0][0].context).toBeUndefined();

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
