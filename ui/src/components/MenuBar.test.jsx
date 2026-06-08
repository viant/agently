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
const removeWindow = vi.fn((windowId) => {
  activeWindows.value = activeWindows.value.filter((entry) => entry.windowId !== windowId);
});
const getWindowContext = vi.fn();

vi.mock('forge/core', () => ({
  activeWindows,
  addWindow,
  removeWindow,
  getWindowContext,
  selectedTabId,
  selectedWindowId
}));

vi.mock('@blueprintjs/core', () => ({
  Button: () => null,
  Dialog: () => null,
  Spinner: () => null,
  Switch: () => null
}));

vi.mock('forge/widgets/SchemaBasedForm.jsx', () => ({
  default: () => null
}));

vi.mock('../services/agentlyClient', () => ({
  client: {
    getAuthMe: vi.fn().mockResolvedValue(null),
    getAuthProviders: vi.fn().mockResolvedValue([]),
    localLogin: vi.fn().mockResolvedValue(undefined),
    logout: vi.fn().mockResolvedValue(undefined)
  }
}));

vi.mock('../services/workspaceMetadata', () => ({
  getWorkspaceMetadataSnapshot: vi.fn().mockReturnValue(null),
  resolveWorkspaceBranding: vi.fn((payload, {
    fallbackName = 'Agently',
    fallbackIconRef = 'builtin:viant',
  } = {}) => ({
    appName: String(payload?.appName || payload?.defaults?.appName || '').trim() || fallbackName,
    appIconRef: String(payload?.appIconRef || payload?.defaults?.appIconRef || '').trim() || fallbackIconRef,
  })),
  subscribeWorkspaceMetadata: vi.fn(() => () => {}),
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
    removeWindow.mockClear();
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

  it('removes the chat subtree before opening a top-level replacement window', async () => {
    activeWindows.value = [
      { windowId: 'chat/new', windowKey: 'chat/new', inTab: true },
      { windowId: 'order_1', windowKey: 'order', inTab: true, parentKey: 'chat/new', presentation: 'hosted', region: 'chat.top' },
      { windowId: 'schedule', windowKey: 'schedule', inTab: true, parentKey: 'chat/new', presentation: 'hosted', region: 'chat.bottom' }
    ];
    addWindow.mockImplementation(() => {
      const win = { windowId: 'schedule/history', windowKey: 'schedule/history', inTab: true };
      activeWindows.value = [...activeWindows.value, win];
      return win;
    });

    const { openWindow } = await import('./MenuBar.jsx');
    openWindow('schedule/history', 'Runs', ['runs'], { replaceTabbedWindows: true, replaceMainChatTree: true });

    expect(activeWindows.value).toEqual([
      { windowId: 'schedule/history', windowKey: 'schedule/history', inTab: true }
    ]);
    expect(selectedTabId.value).toBe('schedule/history');
    expect(selectedWindowId.value).toBe('schedule/history');
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

describe('MenuBar branding', () => {
  it('keeps the Viant logo only for the explicit builtin viant icon', async () => {
    const { resolveBrandLogoSrc } = await import('./MenuBar.jsx');
    expect(resolveBrandLogoSrc('builtin:viant')).toBe('logo.png');
    expect(resolveBrandLogoSrc('')).toBe('logo.png');
    expect(resolveBrandLogoSrc('builtin:steward')).toBe('');
  });

  it('derives a monogram for workspaces without an image-backed icon ref', async () => {
    const { resolveBrandMonogram } = await import('./MenuBar.jsx');
    expect(resolveBrandMonogram('Steward')).toBe('S');
    expect(resolveBrandMonogram('Acme Workspace')).toBe('AW');
    expect(resolveBrandMonogram('')).toBe('A');
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

describe('MenuBar queue approval dialog', () => {
  it('normalizes queue approvals into renderable review schema seeded from queued arguments', async () => {
    const { normalizeQueueApprovalDialog } = await import('./MenuBar.jsx');
    const item = {
      toolName: 'steward/RecommendationPatch',
      arguments: {
        Recommendation: { id: 7288305001 },
        intent: 'target_add_sites',
        rows: [
          { publisher_id: 157, site_id: 3927679773, recommendation: 'ADD', selected: true },
          { publisher_id: 157, site_id: 3932225519, recommendation: 'ADD', selected: true }
        ]
      },
      metadata: {
        approval: { toolName: 'steward/RecommendationPatch', title: 'Recommendation review' },
        review: {
          requestedSchema: {
            type: 'object',
            properties: {
              intent: { type: 'string', readOnly: true },
              rows: {
                type: 'array',
                title: 'Selected recommendations',
                'x-ui-widget': 'planner.table',
                items: {
                  type: 'object',
                  properties: {
                    publisher_id: { type: 'integer' },
                    site_id: { type: 'integer' },
                    recommendation: { type: 'string' },
                    selected: { type: 'boolean', default: true }
                  }
                }
              }
            },
            required: ['rows']
          }
        }
      }
    };

    const dialog = normalizeQueueApprovalDialog(item);
    expect(dialog.approval.toolName).toBe('steward/RecommendationPatch');
    expect(dialog.argumentsPayload.intent).toBe('target_add_sites');
    expect(dialog.preparedSchema.properties.intent.default).toBe('target_add_sites');
    expect(dialog.preparedSchema.properties.rows.default).toEqual(item.arguments.rows);
    expect(dialog.plannerMeta?.defaultRows).toEqual(item.arguments.rows);
    expect(dialog.preparedSchema.properties.rows['x-ui-widget']).toBe('planner.table');
  });

  it('parses stringified queue metadata and arguments', async () => {
    const { normalizeQueueApprovalDialog } = await import('./MenuBar.jsx');
    const metadata = {
      approval: { toolName: 'steward/RecommendationPatch', title: 'Recommendation review' },
      review: {
        requestedSchema: {
          type: 'object',
          properties: {
            intent: { type: 'string', readOnly: true },
            rows: {
              type: 'array',
              title: 'Selected recommendations',
              xUiWidget: 'planner.table',
              items: {
                type: 'object',
                properties: {
                  site_id: { type: 'integer' },
                  selected: { type: 'boolean', default: true }
                }
              }
            }
          }
        }
      }
    };
    const argumentsPayload = {
      intent: 'target_add_sites',
      rows: [
        { site_id: 101, selected: true },
        { site_id: 202, selected: true }
      ]
    };
    const dialog = normalizeQueueApprovalDialog({
      metadata: JSON.stringify(metadata),
      arguments: JSON.stringify(argumentsPayload),
    });
    expect(dialog.argumentsPayload).toEqual(argumentsPayload);
    expect(dialog.preparedSchema.properties.rows.default).toEqual(argumentsPayload.rows);
    expect(dialog.plannerMeta?.defaultRows).toEqual(argumentsPayload.rows);
    expect(dialog.plannerMeta?.field).toBe('rows');
  });
});

describe('MenuBar MCP UI approval routing', () => {
  it('matches pending approvals by exact queue row id', async () => {
    const { findApprovalItemById } = await import('./MenuBar.jsx');
    const items = [
      { id: 'approval-1', toolName: 'system/os:getEnv' },
      { id: 'approval-2', toolName: 'system/os:getEnv' },
    ];
    expect(findApprovalItemById(items, 'approval-2')).toEqual(items[1]);
    expect(findApprovalItemById(items, ' approval-2 ')).toEqual(items[1]);
    expect(findApprovalItemById(items, 'approval-x')).toBe(null);
  });

  it('captures and restores a focusable return target exactly', async () => {
    const { captureReturnFocusTarget, restoreReturnFocusTarget } = await import('./MenuBar.jsx');
    const focus = vi.fn();
    const target = { focus };
    expect(captureReturnFocusTarget({ activeElement: target })).toBe(target);
    expect(restoreReturnFocusTarget(target)).toBe(true);
    expect(focus).toHaveBeenCalledTimes(1);
  });

  it('ignores non-focusable active elements when capturing return focus', async () => {
    const { captureReturnFocusTarget, restoreReturnFocusTarget } = await import('./MenuBar.jsx');
    expect(captureReturnFocusTarget({ activeElement: {} })).toBe(null);
    expect(restoreReturnFocusTarget(null)).toBe(false);
  });

  it('preserves an existing widget return-focus target instead of overwriting it with a later host-side control', async () => {
    const { resolveReturnFocusTarget } = await import('./MenuBar.jsx');
    const iframeTarget = { focus: vi.fn() };
    const hostButton = { focus: vi.fn() };
    expect(resolveReturnFocusTarget(iframeTarget, { activeElement: hostButton })).toBe(iframeTarget);
    expect(resolveReturnFocusTarget(null, { activeElement: hostButton })).toBe(hostButton);
  });

  it('restores focus only on the approval-dialog open->close transition with a captured target', async () => {
    const { nextApprovalDialogFocusReturn } = await import('./MenuBar.jsx');
    const target = { focus: vi.fn() };

    // closed -> closed: nothing to restore, was-open flag stays false.
    expect(nextApprovalDialogFocusReturn({ wasOpen: false, isOpen: false, target }))
      .toEqual({ wasOpen: false, restoreTarget: null });

    // closed -> open: mark dialog as having been open; no restore yet.
    const opening = nextApprovalDialogFocusReturn({ wasOpen: false, isOpen: true, target });
    expect(opening).toEqual({ wasOpen: true, restoreTarget: null });

    // open -> open: still open, no restore.
    expect(nextApprovalDialogFocusReturn({ wasOpen: true, isOpen: true, target }))
      .toEqual({ wasOpen: true, restoreTarget: null });

    // open -> close: emit the captured restore target exactly once.
    expect(nextApprovalDialogFocusReturn({ wasOpen: true, isOpen: false, target }))
      .toEqual({ wasOpen: false, restoreTarget: target });

    // open -> close without a captured target: do not invent one.
    expect(nextApprovalDialogFocusReturn({ wasOpen: true, isOpen: false, target: null }))
      .toEqual({ wasOpen: false, restoreTarget: null });
  });
});
