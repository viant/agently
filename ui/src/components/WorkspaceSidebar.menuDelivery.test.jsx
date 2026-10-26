// @vitest-environment jsdom
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import {useState} from 'react';

const openWindow = vi.fn();
const getAuthMeSilently = vi.fn();
const sidebarRender = vi.fn();

vi.mock('../services/agentlyClient', () => ({ getAuthMeSilently }));
vi.mock('./menuBarHelpers', () => ({ openWindow }));
vi.mock('./Sidebar', () => ({ default: () => {sidebarRender(); return <div data-testid="conversation-sidebar" />;} }));
// Use real shared primitives without loading unrelated report renderers.
vi.mock('forge/components', async () => {
  const {default: Typography, typographyProps} = await import('../../../../forge/src/components/Typography.jsx');
  const {default: Icon} = await import('../../../../forge/src/components/Icon.jsx');
  return {Typography, typographyProps, Icon};
});
vi.mock('forge/core', async () => {
  const {signal} = await import('@preact/signals-react');
  return {activeWindows: signal([]), selectedWindowId: signal(null), selectedTabId: signal(null), getFormSignal: vi.fn(() => signal({}))};
});

let WorkspaceSidebar;
let windowSignals;

beforeAll(async () => {
  ({ default: WorkspaceSidebar } = await import('./WorkspaceSidebar.jsx'));
  windowSignals = await import('forge/core');
});

afterAll(() => {
  cleanup?.();
  window.close();
});

beforeEach(() => {
  cleanup?.();
  window.localStorage.clear();
  openWindow.mockReset();
  sidebarRender.mockReset();
  windowSignals.activeWindows.value = [];
  windowSignals.selectedWindowId.value = null;
  windowSignals.getFormSignal.mockReset().mockReturnValue({value: {}});
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

it('collapses nested menu groups independently and preserves leaf actions', async () => {
  fetch.mockResolvedValue(response(layoutPayload({applications: [{id: 'dsp', title: 'DSP', menus: [
    {id: 'management', title: 'Management', children: [
      {id: 'accounts', title: 'Accounts', children: [
        {id: 'advertisers', title: 'Advertisers', action: {type: 'window', windowKey: 'advertiserList'}},
      ]},
    ]},
  ]}]})));
  render(<WorkspaceSidebar />);
  const parent = await screen.findByRole('button', {name: 'Management'});
  const nested = screen.getByRole('button', {name: 'Accounts'});
  expect(parent.getAttribute('aria-expanded')).toBe('true');
  fireEvent.click(nested);
  expect(screen.queryByRole('button', {name: 'Advertisers'})).toBeNull();
  fireEvent.click(parent);
  expect(parent.getAttribute('aria-expanded')).toBe('false');
  expect(screen.queryByRole('button', {name: 'Accounts'})).toBeNull();
  fireEvent.click(parent);
  expect(screen.getByRole('button', {name: 'Accounts'}).getAttribute('aria-expanded')).toBe('false');
  fireEvent.click(screen.getByRole('button', {name: 'Accounts'}));
  fireEvent.click(screen.getByRole('button', {name: 'Advertisers'}));
  expect(openWindow).toHaveBeenCalledWith('advertiserList', 'Advertisers', [], expect.any(Object));
});

it('shows directional scroll controls only when application tabs overflow', async () => {
  fetch.mockResolvedValue(response(layoutPayload({applications: [
    {id: 'dsp', title: 'DSP', menus: []}, {id: 'studio', title: 'Studio', menus: []},
  ]})));
  render(<WorkspaceSidebar />);
  const tabs = await screen.findByRole('tablist', {name: 'Applications'});
  const scrollBy = vi.fn();
  Object.defineProperties(tabs, {clientWidth: {value: 200}, scrollWidth: {value: 400}, scrollLeft: {value: 0, writable: true}, scrollBy: {value: scrollBy}});
  fireEvent.scroll(tabs);
  expect(screen.getByRole('button', {name: 'Scroll applications left'}).disabled).toBe(true);
  fireEvent.click(screen.getByRole('button', {name: 'Scroll applications right'}));
  expect(scrollBy).toHaveBeenCalledWith({left: 150, behavior: 'smooth'});
  tabs.scrollLeft = 200;
  fireEvent.scroll(tabs);
  expect(screen.getByRole('button', {name: 'Scroll applications right'}).disabled).toBe(true);
  expect(screen.getByRole('button', {name: 'Scroll applications left'}).disabled).toBe(false);
});

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
  it('binds typed report inputs from the selected window and reacts to changes without mutating the catalog', async () => {
    const {signal} = await import('@preact/signals-react');
    const form = signal({AdvertiserId: 83611});
    const action = {type: 'window', resourceUri: 'window://ar/report', parameters: {ViewId: 1294}, parameterBindings: {
      AdvertiserIds: {from: ['AdvertiserId', 'advertiserId'], array: true},
      CampaignId: {from: 'CampaignId', optional: true},
    }};
    const original = JSON.stringify(action);
    windowSignals.activeWindows.value = [{windowId: 'advertiser', parameters: {AdvertiserId: 1}}];
    windowSignals.selectedWindowId.value = 'advertiser';
    windowSignals.getFormSignal.mockImplementation(id => id === 'advertiser:windowForm' ? form : {value: {}});
    fetch.mockResolvedValue(response(layoutPayload({applications: [{id: 'ar', title: 'Reports', menus: [{id: 'overview', title: 'Overview', action}]}]})));
    render(<WorkspaceSidebar conversationId="conversation" />);
    const report = await screen.findByRole('button', {name: 'Overview'});
    fireEvent.click(report);
    expect(openWindow).toHaveBeenLastCalledWith('window://ar/report', 'Overview', [], expect.objectContaining({parameters: {ViewId: 1294, AdvertiserIds: [83611]}}));
    act(() => { form.value = {AdvertiserId: 0}; });
    fireEvent.click(report);
    expect(openWindow).toHaveBeenLastCalledWith('window://ar/report', 'Overview', [], expect.objectContaining({parameters: {ViewId: 1294, AdvertiserIds: [0]}}));
    act(() => { windowSignals.selectedWindowId.value = 'chat'; });
    await waitFor(() => expect(report.disabled).toBe(true));
    openWindow.mockClear();
    fireEvent.click(report);
    expect(openWindow).not.toHaveBeenCalled();
    expect(JSON.stringify(action)).toBe(original);
  });

  it('reads local window form inputs without resolving an undeclared placeholder datasource', async () => {
    windowSignals.activeWindows.value=[{windowId:'local',parameters:{RecordId:7}}];
    windowSignals.selectedWindowId.value='local';
    const resolveDatasource=vi.fn(()=>{throw new Error('DataSource not found: EMPTY');});
    windowSignals.getFormSignal.mockImplementation(id => id === 'local:windowForm' ? {value:{Status:'active'}} : {value:{}});
    fetch.mockResolvedValue(response(layoutPayload({applications:[{id:'reports',title:'Reports',menus:[{id:'open',title:'Open',
      action:{type:'window',windowKey:'report',parameterBindings:{Status:{from:'Status'},RecordId:{from:'RecordId'}}}}]}]})));
    render(<WorkspaceSidebar />);
    fireEvent.click(await screen.findByRole('button',{name:'Open'}));
    expect(resolveDatasource).not.toHaveBeenCalled();
    expect(openWindow).toHaveBeenCalledWith('report','Open',[],expect.objectContaining({parameters:{Status:'active',RecordId:7}}));
  });

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

it('defers discovery until session confirmation and clears menus when the session ends', async () => {
  fetch.mockResolvedValue(response(layoutPayload({ applications: menuApplications })));
  const topbar = vi.fn();
  const mounted = render(<WorkspaceSidebar discoveryReady={false} onTopbarActionsChange={topbar} />);
  expect(fetch).not.toHaveBeenCalled();
  mounted.rerender(<WorkspaceSidebar discoveryReady={true} onTopbarActionsChange={topbar} />);
  await screen.findByRole('button', { name: 'Advertisers' });
  expect(fetch).toHaveBeenCalledTimes(1);
  mounted.rerender(<WorkspaceSidebar discoveryReady={false} onTopbarActionsChange={topbar} />);
  await waitFor(() => expect(screen.queryByRole('button', { name: 'Advertisers' })).toBeNull());
  expect(topbar).toHaveBeenLastCalledWith([]);
});

it('opens management URL menus in an isolated browser tab', async () => {
  fetch.mockResolvedValue(response(layoutPayload({applications:[{id:'management', title:'Management', menus:[{id:'models',title:'Models', action:{type:'url',url:'https://studio.example/?section=models'}}]}]})));
  const open = vi.spyOn(window, 'open').mockImplementation(() => null);
  render(<WorkspaceSidebar />);
  fireEvent.click(await screen.findByRole('button',{name:'Models'}));
  expect(open).toHaveBeenCalledWith('https://studio.example/?section=models','_blank','noopener,noreferrer');
  expect(openWindow).not.toHaveBeenCalled();
  open.mockRestore();
});

it('opens canonical remote window resources through the existing window host', async () => {
  fetch.mockResolvedValue(response(layoutPayload({applications:[{id:'studio', title:'Studio', menus:[{id:'models',title:'Models', action:{type:'window',resourceUri:'window://studio/Models',providerIdentity:'studio',parameters:{section:'active'}}}]}]})));
  render(<WorkspaceSidebar discoveryReady />);
  fireEvent.click(await screen.findByRole('button',{name:'Models'}));
  expect(openWindow).toHaveBeenCalledWith('window://studio/Models','Models',[],expect.objectContaining({parameters:{section:'active'},providerIdentity:'studio',resourceRef:{uri:'window://studio/Models'}}));
});

it('keeps unrelated window navigation enabled while an earlier open is pending', async () => {
  fetch.mockResolvedValue(response(layoutPayload({applications:[{id:'studio',title:'Studio',menus:[
    {id:'slow',title:'Slow window',action:{type:'window',resourceUri:'window://studio/slow',providerIdentity:'studio'}},
    {id:'connectors',title:'Connectors',action:{type:'window',resourceUri:'window://studio/connectors',providerIdentity:'studio'}},
  ]}]})));
  openWindow.mockImplementationOnce(()=>new Promise(()=>{}));
  render(<WorkspaceSidebar discoveryReady />);
  const slow=await screen.findByRole('button',{name:'Slow window'});
  const connectors=screen.getByRole('button',{name:'Connectors'});
  fireEvent.click(slow);
  expect(connectors.disabled).toBe(false);
  fireEvent.click(connectors);
  expect(openWindow).toHaveBeenCalledTimes(2);
  expect(openWindow.mock.calls[1][0]).toBe('window://studio/connectors');
});

it('keeps conversation history mounted while sections replace its visible menu', async () => {
  const payload = layoutPayload({applications: [{id: 'business', title: 'Business', menus: [
    {id: 'accounts', title: 'Accounts', action: {type: 'window', windowKey: 'accounts', parameters: {scope: 'allowed'}}},
  ]}], topbarActions: [{id: 'schedule', title: 'Scheduled tasks', icon: 'lucide:clock-3', action: {type: 'window', windowKey: 'schedule', refreshDataSources: ['schedules']}}]});
  payload.layout.left.navigation = {presentation: 'sections'};
  fetch.mockResolvedValue(response(payload));
  openWindow.mockReturnValue({windowId: 'section-window'});
  const onSectionWindow = vi.fn();
  const onOpenWorkspace = vi.fn();
  function Shell() {
    const [route, setRoute] = useState({section: 'chat', destination: ''});
    return <WorkspaceSidebar {...route} conversationId="chat-a" onSectionWindow={onSectionWindow} onOpenWorkspace={onOpenWorkspace}
      onSectionChange={(section, destination = '') => setRoute({section, destination})}/>;
  }
  render(<Shell/>);
  const rail = await screen.findByRole('navigation', {name: 'App sections'});
  expect(rail.querySelector('[aria-current="page"]').getAttribute('aria-label')).toBe('Chat');
  const history = screen.getByTestId('conversation-sidebar');
  fireEvent.click(screen.getByRole('button', {name: 'Workspaces', exact: true}));
  fireEvent.click(await screen.findByRole('button', {name: 'Accounts'}));
  expect(history.isConnected).toBe(true);
  expect(history.closest('[hidden]')).not.toBeNull();
  expect(openWindow).toHaveBeenLastCalledWith('accounts', 'Accounts', [], expect.objectContaining({conversationId: undefined, parentKey: null, region: 'app.section', parameters: {scope: 'allowed'}}));
  expect(onOpenWorkspace).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', {name: 'Scheduled tasks'}));
  expect(openWindow).toHaveBeenLastCalledWith('schedule', 'Scheduled tasks', ['schedules'], expect.objectContaining({conversationId: undefined, parentKey: null, region: 'app.section'}));
  expect(onSectionWindow).toHaveBeenLastCalledWith({windowId: 'section-window'}, 'Scheduled tasks', 'action:schedule', '', '');
});

it('restores only current catalog destinations and selects the owning application', async () => {
  const payload = layoutPayload({applications: [
    {id:'first',title:'First',menus:[{id:'same',title:'First records',action:{type:'window',windowKey:'firstRecords'}}]},
    {id:'second',title:'Second',menus:[{id:'group',title:'Browse',children:[{id:'same',title:'Second records',action:{type:'window',windowKey:'secondRecords'}}]}]},
  ]});
  payload.layout.left.navigation = {presentation:'sections'};
  fetch.mockResolvedValue(response(payload));
  openWindow.mockReturnValue({windowId:'restored'});
  const onSectionWindow=vi.fn();
  const destination=JSON.stringify(['second','group','same']);
  const mounted=render(<WorkspaceSidebar section="workspaces" destination={destination} onSectionWindow={onSectionWindow}/>);
  const selected=await screen.findByRole('button',{name:'Second records'});
  expect(selected.getAttribute('aria-current')).toBe('page');
  expect(selected.getAttribute('data-forge-type')).toBe('body');
  expect(openWindow).toHaveBeenLastCalledWith('secondRecords','Second records',[],expect.objectContaining({region:'app.section',conversationId:undefined}));
  expect(onSectionWindow).toHaveBeenLastCalledWith({windowId:'restored'},'Second records','workspaces',destination,'');
  mounted.rerender(<WorkspaceSidebar section="workspaces" destination='["missing","arbitrary-window"]' onSectionWindow={onSectionWindow}/>);
  await waitFor(()=>expect(onSectionWindow).toHaveBeenLastCalledWith(null,'','workspaces','["missing","arbitrary-window"]',expect.stringContaining('unavailable')));
  expect(openWindow).toHaveBeenCalledTimes(1);
});

it('does not reopen disabled destinations or routes without required context', async () => {
  const payload=layoutPayload({applications:[{id:'example',menus:[
    {id:'denied',disabled:true,action:{type:'window',windowKey:'denied'}},
    {id:'scoped',title:'Scoped',action:{type:'window',windowKey:'scoped',parameterBindings:{account:{from:['account']}}}},
  ]}]});
  payload.layout.left.navigation={presentation:'sections'};
  fetch.mockResolvedValue(response(payload));
  const onSectionWindow=vi.fn();
  const mounted=render(<WorkspaceSidebar section="workspaces" destination='["example","denied"]' onSectionWindow={onSectionWindow}/>);
  await waitFor(()=>expect(onSectionWindow).toHaveBeenLastCalledWith(null,'','workspaces','["example","denied"]',expect.stringContaining('unavailable')));
  mounted.rerender(<WorkspaceSidebar section="workspaces" destination='["example","scoped"]' onSectionWindow={onSectionWindow}/>);
  await waitFor(()=>expect(onSectionWindow).toHaveBeenLastCalledWith(null,'Scoped','workspaces','["example","scoped"]',expect.stringContaining('required context')));
  expect(openWindow).not.toHaveBeenCalled();
});

it('restores an existing action section from current metadata without a rail click', async () => {
  const payload=layoutPayload({topbarActions:[{id:'schedule',title:'Scheduled tasks',action:{type:'window',windowKey:'schedule',refreshDataSources:['schedules']}}]});
  payload.layout.left.navigation={presentation:'sections'};
  fetch.mockResolvedValue(response(payload));
  openWindow.mockReturnValue({windowId:'schedule-window'});
  render(<WorkspaceSidebar section="action:schedule"/>);
  await waitFor(()=>expect(openWindow).toHaveBeenCalledWith('schedule','Scheduled tasks',['schedules'],expect.objectContaining({region:'app.section'})));
});

it('honors disabled history and reveals section menus from the collapsed rail', async () => {
  const payload = layoutPayload({applications: [{id: 'example', title: 'Example', menus: []}]});
  payload.layout.left.navigation = {presentation: 'sections'};
  payload.layout.left.history = {enabled: false};
  fetch.mockResolvedValue(response(payload));
  const expand = vi.fn();
  const select = vi.fn();
  render(<WorkspaceSidebar collapsed onExpand={expand} onSectionChange={select}/>);
  const workspaces = await screen.findByRole('button', {name: 'Workspaces', exact: true});
  expect(screen.queryByTestId('conversation-sidebar')).toBeNull();
  expect(sidebarRender).not.toHaveBeenCalled();
  fireEvent.click(workspaces);
  expect(expand).toHaveBeenCalledOnce();
  expect(select).toHaveBeenLastCalledWith('workspaces');
});
