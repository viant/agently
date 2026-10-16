// @vitest-environment jsdom
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';

const openWindow = vi.fn();
const getAuthMeSilently = vi.fn();

vi.mock('../services/agentlyClient', () => ({ getAuthMeSilently }));
vi.mock('./menuBarHelpers', () => ({ openWindow }));
vi.mock('./Sidebar', () => ({ default: () => <div data-testid="conversation-sidebar" /> }));

let WorkspaceSidebar;

beforeAll(async () => {
  ({ default: WorkspaceSidebar } = await import('./WorkspaceSidebar.jsx'));
});

afterAll(() => {
  cleanup?.();
  window.close();
});

beforeEach(() => {
  cleanup?.();
  window.localStorage.clear();
  openWindow.mockReset();
  getAuthMeSilently.mockReset().mockResolvedValue({ subject: 'menu-test-user' });
  vi.stubGlobal('fetch', vi.fn());
});

function response(payload, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => payload,
  };
}

function layoutPayload({ applications, topbarActions = [], layoutId = 'test-layout' }) {
  return {
    workspaceId: 'menu-test-workspace',
    layout: {
      id: layoutId,
      left: {
        width: { default: 320, min: 240, max: 500 },
        split: { initial: 0.5, minSectionHeight: 160 },
        navigation: { enabled: true },
        history: { enabled: true },
      },
      topbar: { actions: topbarActions },
      applications,
    },
  };
}

const menuApplications = [
  {
    id: 'advertising', title: 'Advertising', icon: 'briefcase',
    menus: [
      { id: 'advertisers', title: 'Advertisers', icon: 'people', action: { type: 'window', windowKey: 'advertiserList', parameters: { source: 'workspace-menu' } } },
      { id: 'campaigns', title: 'Campaigns', icon: 'chart', action: { type: 'window', windowKey: 'campaignList', parameters: { status: 'active' }, refreshDataSources: ['campaigns'] } },
    ],
  },
];

describe('WorkspaceSidebar menu delivery from the workspace layout', () => {
  it('renders Advertisers and Campaigns from layout.applications and preserves their window actions', async () => {
    const topbarActions = [{ id: 'help', title: 'Help' }];
    fetch.mockResolvedValueOnce(response(layoutPayload({ applications: menuApplications, topbarActions })));
    const onTopbarActionsChange = vi.fn();
    const onOpenWorkspace = vi.fn();
    const onNavigate = vi.fn();

    render(<WorkspaceSidebar conversationId="conv-42" onTopbarActionsChange={onTopbarActionsChange} onOpenWorkspace={onOpenWorkspace} onNavigate={onNavigate} />);

    const advertisers = await screen.findByRole('button', { name: 'Advertisers' });
    const campaigns = screen.getByRole('button', { name: 'Campaigns' });
    expect(advertisers.querySelector('.app-layout-menu-icon')?.classList.contains('bp6-icon-people')).toBe(true);
    expect(campaigns.querySelector('.app-layout-menu-icon')?.classList.contains('bp6-icon-chart')).toBe(true);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0][0]).toMatch(/\/workspace\/layout$/);
    expect(fetch.mock.calls[0][1]).toMatchObject({ credentials: 'include', headers: { Accept: 'application/json' } });
    expect(onTopbarActionsChange).toHaveBeenCalledWith(topbarActions);

    fireEvent.click(advertisers);
    expect(openWindow).toHaveBeenLastCalledWith('advertiserList', 'Advertisers', [], {
      parameters: { source: 'workspace-menu' }, conversationId: 'conv-42', parentKey: 'chat/new', presentation: 'hosted', region: 'chat.top',
    });
    fireEvent.click(campaigns);
    expect(openWindow).toHaveBeenLastCalledWith('campaignList', 'Campaigns', ['campaigns'], {
      parameters: { status: 'active' }, conversationId: 'conv-42', parentKey: 'chat/new', presentation: 'hosted', region: 'chat.top',
    });
    expect(onOpenWorkspace).toHaveBeenCalledTimes(2);
    expect(onNavigate).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls[0][0]).not.toMatch(/menu/i);
  });

  it('clears previously loaded menus and topbar actions after reload returns 403', async () => {
    fetch.mockResolvedValueOnce(response(layoutPayload({ applications: menuApplications, topbarActions: [{ id: 'old' }] })))
      .mockResolvedValueOnce(response({}, 403));
    const onTopbarActionsChange = vi.fn();
    render(<WorkspaceSidebar onTopbarActionsChange={onTopbarActionsChange} />);
    await screen.findByRole('button', { name: 'Campaigns' });

    fireEvent.click(screen.getByRole('button', { name: 'Reload navigation' }));

    await screen.findByRole('alert');
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole('button', { name: 'Advertisers' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Campaigns' })).toBeNull();
    expect(screen.queryByText(/advertiserList|campaignList|fallback/i)).toBeNull();
    expect(onTopbarActionsChange).toHaveBeenLastCalledWith([]);
  });

  it('removes old menu items when a successful reload contains an empty applications array', async () => {
    fetch.mockResolvedValueOnce(response(layoutPayload({ applications: menuApplications })))
      .mockResolvedValueOnce(response(layoutPayload({ applications: [], layoutId: 'empty-menu' })));
    render(<WorkspaceSidebar />);
    await screen.findByRole('button', { name: 'Campaigns' });

    fireEvent.click(screen.getByRole('button', { name: 'Reload navigation' }));

    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole('button', { name: 'Advertisers' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Campaigns' })).toBeNull();
  });

  it('continues to render ordinary legacy applications and their menus', async () => {
    fetch.mockResolvedValueOnce(response(layoutPayload({ applications: [
      { id: 'legacy', title: 'Legacy tools', menus: [{ id: 'legacy-window', title: 'Legacy report', icon: 'document', action: { type: 'window', windowKey: 'legacyReport', parameters: { legacy: true } } }] },
    ] })));
    render(<WorkspaceSidebar />);

    const legacyMenu = await screen.findByRole('button', { name: 'Legacy report' });
    expect(legacyMenu.querySelector('.app-layout-menu-icon')).toBeTruthy();
    fireEvent.click(legacyMenu);
    expect(openWindow).toHaveBeenCalledWith('legacyReport', 'Legacy report', [], {
      parameters: { legacy: true }, conversationId: undefined, parentKey: 'chat/new', presentation: 'hosted', region: 'chat.top',
    });
  });
});
