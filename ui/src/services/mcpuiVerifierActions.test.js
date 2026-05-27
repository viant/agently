import { describe, expect, it } from 'vitest';

import { Context } from 'forge/core';
import { injectActions } from 'forge/actions';
import { useControlEvents } from 'forge/hooks';

globalThis.window = globalThis.window || {};

function createMetadata() {
  return {
    namespace: 'MCPUIVerifier',
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

function createBridgeRecorder() {
  const messages = [];
  globalThis.window.__mcpuiForgeGuest = {
    message(content) {
      messages.push({ type: 'message', content });
    },
  };
  return messages;
}

describe('mcpui verifier action path', () => {
  it('resolves the workspace handler and updates windowForm + guest bridge directly', () => {
    const metadata = createMetadata();
    injectActions(metadata);
    const ctx = Context('W1', metadata, 'verifier', { __connectorRuntime: {} });
    ctx.init();
    const dsCtx = ctx.Context('verifier');
    const messages = createBridgeRecorder();
    const handler = dsCtx.lookupHandler('MCPUIVerifier.sendHostMessage');
    expect(typeof handler).toBe('function');
    expect(handler({ context: dsCtx })).toBe(true);
    expect(dsCtx.signals.windowForm.peek().statusText).toBe('sendHostMessage invoked');
    expect(dsCtx.signals.windowForm.peek().hostStatus).toBe('host message requested');
    expect(messages).toEqual([{ type: 'message', content: 'Interactive Forge verifier says hello from the workspace surface.' }]);
  });

  it('wires button onClick through useControlEvents into the same handler path', () => {
    const metadata = createMetadata();
    injectActions(metadata);
    const ctx = Context('W2', metadata, 'verifier', { __connectorRuntime: {} });
    ctx.init();
    const dsCtx = ctx.Context('verifier');
    const messages = createBridgeRecorder();
    const item = {
      id: 'sendHostMessage',
      type: 'button',
      on: [{ event: 'onClick', handler: 'MCPUIVerifier.sendHostMessage' }],
    };
    const handlers = useControlEvents(dsCtx, [item], undefined);
    expect(typeof handlers.sendHostMessage?.events?.onClick).toBe('function');
    handlers.sendHostMessage.events.onClick({ target: {} });
    expect(dsCtx.signals.windowForm.peek().statusText).toBe('sendHostMessage invoked');
    expect(dsCtx.signals.windowForm.peek().hostStatus).toBe('host message requested');
    expect(messages).toEqual([{ type: 'message', content: 'Interactive Forge verifier says hello from the workspace surface.' }]);
  });
});
