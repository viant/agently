import { describe, expect, it } from 'vitest';

import { Context } from 'forge/core';
import { injectActions } from 'forge/actions';
import { runWindowLifecycleHandlers } from 'forge/components/windowLifecycle.js';

globalThis.window = globalThis.window || {};

function createMetadata() {
  return {
    namespace: 'MCPUIVerifier',
    on: [
      { event: 'onInit', handler: 'MCPUIVerifier.sendHostMessage' },
    ],
    actions: {
      code: `(() => {
        function setWindowStatus(context, key, value) {
          const current = context?.signals?.windowForm?.peek?.() || {};
          if (context?.signals?.windowForm) {
            context.signals.windowForm.value = {
              ...current,
              [key]: value,
            };
          }
        }
        function guest() {
          const bridge = window.__mcpuiForgeGuest;
          if (!bridge) {
            throw new Error("MCP UI forge guest bridge is not ready");
          }
          return bridge;
        }
        function sendHostMessage(props = {}) {
          const { context } = props;
          setWindowStatus(context, "statusText", "sendHostMessage invoked");
          guest().message("Interactive Forge verifier says hello from the workspace surface.");
          setWindowStatus(context, "hostStatus", "host message requested");
          return true;
        }
        return { sendHostMessage };
      })()`,
    },
    dataSource: {
      verifier: {
        selectionMode: 'single',
        parameters: [],
      },
    },
  };
}

describe('window lifecycle handlers', () => {
  it('runs top-level onInit handlers and mutates windowForm state', () => {
    const sent = [];
    globalThis.window.__mcpuiForgeGuest = {
      message(content) {
        sent.push(content);
      },
    };
    const metadata = createMetadata();
    injectActions(metadata);
    const root = Context('W-verify', metadata, 'verifier', { __connectorRuntime: {} });
    root.init();
    const dsCtx = root.Context('verifier');
    runWindowLifecycleHandlers({
      eventName: 'onInit',
      metadata,
      context: root,
      defaultDataSourceRef: 'verifier',
      windowFormSignal: dsCtx.signals.windowForm,
      windowId: 'W-verify',
      windowKey: 'mcpuiVerifierInteractive',
    });
    expect(dsCtx.signals.windowForm.peek().statusText).toBe('sendHostMessage invoked');
    expect(dsCtx.signals.windowForm.peek().hostStatus).toBe('host message requested');
    expect(sent).toEqual(['Interactive Forge verifier says hello from the workspace surface.']);
  });
});
