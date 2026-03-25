import { beforeEach, describe, expect, it, vi } from 'vitest';

const activeWindows = {
  value: [],
  peek() {
    return this.value;
  }
};

const selectedTabId = { value: null };
const selectedWindowId = { value: null };
const addWindow = vi.fn();
const getWindowContext = vi.fn();

vi.mock('forge/core', () => ({
  activeWindows,
  addWindow,
  getWindowContext,
  selectedTabId,
  selectedWindowId
}));

vi.mock('@blueprintjs/core', () => ({
  Button: () => null,
  Dialog: () => null
}));

vi.mock('../services/agentlyClient', () => ({
  client: {
    getAuthMe: vi.fn().mockResolvedValue(null),
    logout: vi.fn().mockResolvedValue(undefined)
  }
}));

vi.mock('../viant-logo.png', () => ({
  default: 'logo.png'
}));

describe('MenuBar window reuse', () => {
  beforeEach(() => {
    activeWindows.value = [];
    selectedTabId.value = null;
    selectedWindowId.value = null;
    addWindow.mockReset();
    getWindowContext.mockReset();
  });

  it('refreshes the existing runs data source when reopening the top-level runs tab', async () => {
    const fetchCollection = vi.fn();
    activeWindows.value = [{ windowId: 'runs-window', windowKey: 'schedule/history' }];
    getWindowContext.mockReturnValue({
      Context(dataSourceRef) {
        expect(dataSourceRef).toBe('runs');
        return {
          handlers: {
            dataSource: {
              fetchCollection
            }
          }
        };
      }
    });

    const { openWindow } = await import('./MenuBar.jsx');
    openWindow('schedule/history', 'Runs', ['runs']);

    expect(selectedTabId.value).toBe('runs-window');
    expect(selectedWindowId.value).toBe('runs-window');
    expect(fetchCollection).toHaveBeenCalledTimes(1);
    expect(addWindow).not.toHaveBeenCalled();
  });
});

describe('MenuBar auth startup selection', () => {
  it('prefers oauth when oauth provider is present', async () => {
    const { resolveStartupAuthAction } = await import('./MenuBar.jsx');
    const action = resolveStartupAuthAction([
      { type: 'local', defaultUsername: 'devuser' },
      { type: 'bff', name: 'oauth' }
    ]);
    expect(action).toEqual({ type: 'oauth' });
  });

  it('falls back to local only when oauth is absent', async () => {
    const { resolveStartupAuthAction } = await import('./MenuBar.jsx');
    const action = resolveStartupAuthAction([
      { type: 'local', defaultUsername: 'devuser' }
    ]);
    expect(action).toEqual({ type: 'local', username: 'devuser' });
  });
});
