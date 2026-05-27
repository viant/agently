import { describe, expect, it } from 'vitest';
import fs from 'node:fs';
import path from 'node:path';

const HANDLERS_PATH = path.resolve(
  __dirname,
  '../../../../.agently/extension/forge/windows/mcpuiVerifierInteractive.js',
);

function createWindowFormSignal(initial = {}) {
  let snapshot = { ...initial };
  return {
    peek() {
      return snapshot;
    },
    get value() {
      return snapshot;
    },
    set value(next) {
      snapshot = next;
    },
  };
}

function createContext() {
  return {
    signals: { windowForm: createWindowFormSignal() },
  };
}

function loadHandlers(target) {
  const source = fs.readFileSync(HANDLERS_PATH, 'utf8');
  const factory = new Function('window', `return ${source};`);
  return factory(target);
}

describe('mcpuiVerifierInteractive workspace handlers', () => {
  it('sets local statusText and clickCount on every invocation', () => {
    const handlers = loadHandlers({});
    const context = createContext();

    handlers.sendHostMessage({ context });
    handlers.requestToolCall({ context });

    const snapshot = context.signals.windowForm.peek();
    expect(snapshot.statusText).toMatch(/^requestToolCall invoked at /);
    expect(snapshot.clickCount).toBe(2);
  });

  it('marks hostStatus as unavailable when the guest bridge is missing', () => {
    const handlers = loadHandlers({});
    const context = createContext();

    handlers.requestOpenLink({ context });

    const snapshot = context.signals.windowForm.peek();
    expect(snapshot.statusText).toMatch(/^requestOpenLink invoked at /);
    expect(snapshot.clickCount).toBe(1);
    expect(snapshot.hostStatus).toBe('guest bridge unavailable');
  });

  it('records a guest action attempt and posts through the bridge when available', () => {
    const posts = [];
    const target = {
      __mcpuiForgeGuest: {
        message(content) {
          posts.push({ kind: 'message', content });
        },
        toolCall(name, args, assistantText) {
          posts.push({ kind: 'toolCall', name, args, assistantText });
        },
        openLink(url) {
          posts.push({ kind: 'openLink', url });
        },
      },
    };
    const handlers = loadHandlers(target);
    const context = createContext();

    handlers.sendHostMessage({ context });
    handlers.requestToolCall({ context });
    handlers.requestOpenLink({ context });

    expect(posts).toEqual([
      { kind: 'message', content: 'Interactive Forge verifier says hello from the workspace surface.' },
      { kind: 'toolCall', name: 'system/os:getEnv', args: { names: ['HOME'] }, assistantText: 'Read HOME from the interactive Forge verifier workspace.' },
      { kind: 'openLink', url: 'https://example.com/' },
    ]);
    const snapshot = context.signals.windowForm.peek();
    expect(snapshot.clickCount).toBe(3);
    expect(snapshot.hostStatus).toBe('guest action requested');
  });

  it('captures guest action errors in hostStatus without losing the local invocation record', () => {
    const target = {
      __mcpuiForgeGuest: {
        message() {
          throw new Error('forced failure');
        },
        toolCall() {},
        openLink() {},
      },
    };
    const handlers = loadHandlers(target);
    const context = createContext();

    handlers.sendHostMessage({ context });

    const snapshot = context.signals.windowForm.peek();
    expect(snapshot.statusText).toMatch(/^sendHostMessage invoked at /);
    expect(snapshot.clickCount).toBe(1);
    expect(snapshot.hostStatus).toBe('guest action failed: forced failure');
  });
});

