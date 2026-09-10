import { describe, expect, it } from 'vitest';

import { buildWebClientContext, buildWebQueryContext, detectWebFormFactor, subscribeWebFormFactor } from './clientContext';

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

  it('publishes form-factor changes when the browser crosses a responsive breakpoint', () => {
    const previousWindow = global.window;
    let resize;
    const values = [];
    global.window = {
      innerWidth: 1365,
      addEventListener: (name, handler) => { if (name === 'resize') resize = handler; },
      removeEventListener: () => {},
    };
    try {
      const unsubscribe = subscribeWebFormFactor((value) => values.push(value));
      global.window.innerWidth = 1024;
      resize();
      global.window.innerWidth = 390;
      resize();
      global.window.innerWidth = 487;
      resize();
      global.window.innerWidth = 1440;
      resize();
      unsubscribe();
      expect(values).toEqual(['desktop', 'tablet', 'phone', 'desktop']);
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
