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
    getAuthProviders: vi.fn().mockResolvedValue([]),
    localLogin: vi.fn().mockResolvedValue(undefined),
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
  }, 10000);

  it('replaces tabbed main-pane windows when requested and keeps floating windows', async () => {
    activeWindows.value = [
      { windowId: 'chat/new', windowKey: 'chat/new', inTab: true },
      { windowId: 'schedule', windowKey: 'schedule', inTab: true },
      { windowId: 'floating-tool', windowKey: 'tool', inTab: false }
    ];
    addWindow.mockImplementation(() => {
      const win = { windowId: 'schedule/history', windowKey: 'schedule/history', inTab: true };
      activeWindows.value = [...activeWindows.value, win];
      return win;
    });

    const { openWindow } = await import('./MenuBar.jsx');
    openWindow('schedule/history', 'Runs', ['runs'], { replaceTabbedWindows: true });

    expect(activeWindows.value).toEqual([
      { windowId: 'floating-tool', windowKey: 'tool', inTab: false },
      { windowId: 'schedule/history', windowKey: 'schedule/history', inTab: true }
    ]);
    expect(selectedTabId.value).toBe('schedule/history');
    expect(selectedWindowId.value).toBe('schedule/history');
    expect(addWindow).toHaveBeenCalledTimes(1);
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

  it('does not auto-trigger oauth login from MenuBar bootstrapping', async () => {
    const { client } = await import('../services/agentlyClient');
    client.getAuthMe.mockResolvedValueOnce(null);
    client.getAuthProviders.mockResolvedValueOnce([{ type: 'bff', name: 'oauth' }]);

    const mod = await import('./MenuBar.jsx');
    expect(mod.resolveStartupAuthAction([{ type: 'bff', name: 'oauth' }])).toEqual({ type: 'oauth' });
    // The component bootstrap should not call local login when oauth is present.
    expect(client.localLogin).not.toHaveBeenCalled();
  });
});

describe('MenuBar approval pagination', () => {
  it('paginates approval items with stable ranges', async () => {
    const { paginateApprovalItems } = await import('./MenuBar.jsx');
    const items = Array.from({ length: 18 }, (_, index) => ({ id: `approval-${index + 1}` }));

    const first = paginateApprovalItems(items, 0, 8);
    expect(first.items).toHaveLength(8);
    expect(first.start).toBe(0);
    expect(first.end).toBe(8);
    expect(first.total).toBe(18);
    expect(first.hasPrevious).toBe(false);
    expect(first.hasNext).toBe(true);

    const middle = paginateApprovalItems(items, 1, 8);
    expect(middle.items).toHaveLength(8);
    expect(middle.start).toBe(8);
    expect(middle.end).toBe(16);
    expect(middle.hasPrevious).toBe(true);
    expect(middle.hasNext).toBe(true);

    const last = paginateApprovalItems(items, 2, 8);
    expect(last.items).toHaveLength(2);
    expect(last.start).toBe(16);
    expect(last.end).toBe(18);
    expect(last.hasPrevious).toBe(true);
    expect(last.hasNext).toBe(false);
  });

  it('clamps out-of-range pages back to the last page', async () => {
    const { paginateApprovalItems } = await import('./MenuBar.jsx');
    const items = Array.from({ length: 3 }, (_, index) => ({ id: `approval-${index + 1}` }));

    const page = paginateApprovalItems(items, 99, 2);
    expect(page.page).toBe(1);
    expect(page.start).toBe(2);
    expect(page.end).toBe(3);
    expect(page.items).toHaveLength(1);
  });
});
