import { describe, it, expect, vi } from 'vitest';

import { connectForgeUIActionsToChat, dispatchForgeUIAction } from './forgeUIActions';

describe('forgeUIActions', () => {
  it('bridges forge-ui-action into submitMessage with a runtime-safe summary', async () => {
    const previousWindow = globalThis.window;
    const eventTarget = new EventTarget();
    globalThis.window = eventTarget;
    globalThis.window.addEventListener = eventTarget.addEventListener.bind(eventTarget);
    globalThis.window.removeEventListener = eventTarget.removeEventListener.bind(eventTarget);
    globalThis.window.dispatchEvent = eventTarget.dispatchEvent.bind(eventTarget);

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
    globalThis.window = previousWindow;
  });
});
