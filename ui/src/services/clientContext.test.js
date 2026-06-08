import { describe, expect, it } from 'vitest';

import { buildWebClientContext, buildWebQueryContext, detectWebFormFactor } from './clientContext';

describe('clientContext', () => {
  it('detects phone, tablet, and desktop form factors from window width', () => {
    const previousWindow = global.window;

    global.window = { innerWidth: 640 };
    expect(detectWebFormFactor()).toBe('phone');

    global.window = { innerWidth: 900 };
    expect(detectWebFormFactor()).toBe('tablet');

    global.window = { innerWidth: 1365 };
    expect(detectWebFormFactor()).toBe('desktop');

    global.window = previousWindow;
  });

  it('builds web client context with target-identifying metadata', () => {
    const previousWindow = global.window;
    global.window = { innerWidth: 900 };

    try {
      expect(buildWebClientContext()).toEqual({
        kind: 'web',
        platform: 'web',
        formFactor: 'tablet',
        surface: 'browser',
        capabilities: ['markdown', 'chart', 'upload', 'code', 'diff'],
      });
    } finally {
      global.window = previousWindow;
    }
  });

  it('nests client target metadata inside query context', () => {
    const previousWindow = global.window;
    global.window = { innerWidth: 1365, sessionStorage: { getItem: () => null, setItem: () => {} } };

    try {
      const result = buildWebQueryContext();
      expect(result.client).toEqual({
        kind: 'web',
        platform: 'web',
        formFactor: 'desktop',
        surface: 'browser',
        capabilities: ['markdown', 'chart', 'upload', 'code', 'diff'],
      });
      expect(typeof result.uiClientId).toBe('string');
      expect(result.uiClientId.length).toBeGreaterThan(0);
    } finally {
      global.window = previousWindow;
    }
  });
});
