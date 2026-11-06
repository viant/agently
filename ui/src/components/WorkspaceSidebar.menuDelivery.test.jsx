// @vitest-environment jsdom
import { afterAll, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import {useState} from 'react';

const openWindow = vi.fn();
const getAuthMeSilently = vi.fn();
const sidebarRender = vi.fn();

vi.mock('../services/agentlyClient', () => ({ getAuthMeSilently }));
vi.mock('./menuBarHelpers', () => ({ openWindow }));
vi.mock('./Sidebar', () => ({ default: props => {sidebarRender(props); return <div data-testid="conversation-sidebar" hidden={props.hidden}/>;} }));
// Use real shared primitives without loading unrelated report renderers.
vi.mock('forge/components', async () => {
  const {default: Typography, typographyProps} = await import('forge/components/Typography.jsx');
  const {default: Icon} = await import('forge/components/Icon.jsx');
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
        {id: 'records', title: 'Records', action: {type: 'window', windowKey: 'recordList'}},
      ]},
    ]},
  ]}]})));
  render(<WorkspaceSidebar />);
  const parent = await screen.findByRole('button', {name: 'Management'});
  const nested = screen.getByRole('button', {name: 'Accounts'});
  expect(parent.getAttribute('aria-expanded')).toBe('true');
  fireEvent.click(nested);
  expect(screen.queryByRole('button', {name: 'Records'})).toBeNull();
  fireEvent.click(parent);
  expect(parent.getAttribute('aria-expanded')).toBe('false');
  expect(screen.queryByRole('button', {name: 'Accounts'})).toBeNull();
  fireEvent.click(parent);
  expect(screen.getByRole('button', {name: 'Accounts'}).getAttribute('aria-expanded')).toBe('false');
  fireEvent.click(screen.getByRole('button', {name: 'Accounts'}));
  fireEvent.click(screen.getByRole('button', {name: 'Records'}));
  expect(openWindow).toHaveBeenCalledWith('recordList', 'Records', [], expect.any(Object));
});

it.each(['split', 'sections'])('shows directional scroll controls when %s application tabs overflow', async presentation => {
  const payload = layoutPayload({applications: [
    {id: 'dsp', title: 'DSP', menus: []}, {id: 'studio', title: 'Studio', menus: []},
  ]});
  payload.layout.left.navigation = {presentation};
  fetch.mockResolvedValue(response(payload));
  render(<WorkspaceSidebar section="workspaces" />);
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
      { id: 'records', title: 'Records', icon: 'people', action: { type: 'window', windowKey: 'recordList', parameters: { source: 'workspace-menu' } } },
      { id: 'projects', title: 'Projects', icon: 'chart', action: { type: 'window', windowKey: 'projectList', parameters: { status: 'active' }, refreshDataSources: ['projects'] } },
    ],
  },
];

describe('WorkspaceSidebar menu delivery from the workspace layout', () => {
  it('binds typed report inputs from the selected window and reacts to changes without mutating the catalog', async () => {
    const {signal} = await import('@preact/signals-react');
    const form = signal({RecordId: 83611});
    const action = {type: 'window', resourceUri: 'window://ar/report', parameters: {ViewId: 1294}, parameterBindings: {
      RecordIds: {from: ['RecordId', 'recordId'], array: true},
      ProjectId: {from: 'ProjectId', optional: true},
    }};
    const original = JSON.stringify(action);
    windowSignals.activeWindows.value = [{windowId: 'record', parameters: {RecordId: 1}}];
    windowSignals.selectedWindowId.value = 'record';
    windowSignals.getFormSignal.mockImplementation(id => id === 'record:windowForm' ? form : {value: {}});
    fetch.mockResolvedValue(response(layoutPayload({applications: [{id: 'ar', title: 'Reports', menus: [{id: 'overview', title: 'Overview', action}]}]})));
    render(<WorkspaceSidebar conversationId="conversation" />);
    const report = await screen.findByRole('button', {name: 'Overview'});
    fireEvent.click(report);
    expect(openWindow).toHaveBeenLastCalledWith('window://ar/report', 'Overview', [], expect.objectContaining({parameters: {ViewId: 1294, RecordIds: [83611]}}));
    act(() => { form.value = {RecordId: 0}; });
    fireEvent.click(report);
    expect(openWindow).toHaveBeenLastCalledWith('window://ar/report', 'Overview', [], expect.objectContaining({parameters: {ViewId: 1294, RecordIds: [0]}}));
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

  it('renders Records and Projects from layout.applications and preserves their window actions', async () => {
    const topbarActions = [{ id: 'help', title: 'Help' }];
    fetch.mockResolvedValueOnce(response(layoutPayload({ applications: menuApplications, topbarActions })));
    const onTopbarActionsChange = vi.fn();
    const onOpenWorkspace = vi.fn();
    const onNavigate = vi.fn();

    render(<WorkspaceSidebar conversationId="conv-42" onTopbarActionsChange={onTopbarActionsChange} onOpenWorkspace={onOpenWorkspace} onNavigate={onNavigate} />);

    const records = await screen.findByRole('button', { name: 'Records' });
    const projects = screen.getByRole('button', { name: 'Projects' });
    expect(records.querySelector('.app-layout-menu-icon')?.classList.contains('bp6-icon-people')).toBe(true);
    expect(projects.querySelector('.app-layout-menu-icon')?.classList.contains('bp6-icon-chart')).toBe(true);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0][0]).toMatch(/\/workspace\/layout$/);
    expect(fetch.mock.calls[0][1]).toMatchObject({ credentials: 'include', headers: { Accept: 'application/json' } });
    expect(onTopbarActionsChange).toHaveBeenCalledWith(topbarActions);

    fireEvent.click(records);
    expect(openWindow).toHaveBeenLastCalledWith('recordList', 'Records', [], {
      parameters: { source: 'workspace-menu' }, conversationId: 'conv-42', parentKey: 'chat/new', presentation: 'hosted', region: 'chat.top',
    });
    fireEvent.click(projects);
    expect(openWindow).toHaveBeenLastCalledWith('projectList', 'Projects', ['projects'], {
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
    await screen.findByRole('button', { name: 'Projects' });

    fireEvent.click(screen.getByRole('button', { name: 'Reload navigation' }));

    await screen.findByRole('alert');
    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole('button', { name: 'Records' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Projects' })).toBeNull();
    expect(screen.queryByText(/recordList|projectList|fallback/i)).toBeNull();
    expect(onTopbarActionsChange).toHaveBeenLastCalledWith([]);
  });

  it('removes old menu items when a successful reload contains an empty applications array', async () => {
    fetch.mockResolvedValueOnce(response(layoutPayload({ applications: menuApplications })))
      .mockResolvedValueOnce(response(layoutPayload({ applications: [], layoutId: 'empty-menu' })));
    render(<WorkspaceSidebar />);
    await screen.findByRole('button', { name: 'Projects' });

    fireEvent.click(screen.getByRole('button', { name: 'Reload navigation' }));

    await waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole('button', { name: 'Records' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Projects' })).toBeNull();
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
  await screen.findByRole('button', { name: 'Records' });
  expect(fetch).toHaveBeenCalledTimes(1);
  mounted.rerender(<WorkspaceSidebar discoveryReady={false} onTopbarActionsChange={topbar} />);
  await waitFor(() => expect(screen.queryByRole('button', { name: 'Records' })).toBeNull());
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
  expect(history.hidden).toBe(false);
  fireEvent.click(screen.getByRole('button', {name: 'Workspaces', exact: true}));
  fireEvent.click(await screen.findByRole('button', {name: 'Accounts'}));
  expect(history.isConnected).toBe(true);
  expect(history.hidden).toBe(true);
  expect(history.closest('[hidden]')).not.toBeNull();
  expect(openWindow).toHaveBeenLastCalledWith('accounts', 'Accounts', [], expect.objectContaining({conversationId: undefined, parentKey: null, region: 'app.section', parameters: {scope: 'allowed'}}));
  expect(onOpenWorkspace).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button', {name: 'Scheduled tasks'}));
  expect(openWindow).toHaveBeenLastCalledWith('schedule', 'Scheduled tasks', ['schedules'], expect.objectContaining({conversationId: undefined, parentKey: null, region: 'app.section'}));
  expect(onSectionWindow).toHaveBeenLastCalledWith({windowId: 'section-window'}, 'Scheduled tasks', 'action:schedule', '', '');
});

it('updates retained conversation history when the navigation drawer is hidden or shown', async () => {
  const payload = layoutPayload({applications: []});
  payload.layout.left.navigation = {presentation: 'sections'};
  fetch.mockResolvedValue(response(payload));
  const mounted = render(<WorkspaceSidebar drawerMode drawerHidden/>);
  const history = await screen.findByTestId('conversation-sidebar');
  expect(history.hidden).toBe(true);
  mounted.rerender(<WorkspaceSidebar drawerMode drawerHidden={false}/>);
  expect(history.hidden).toBe(false);
  mounted.rerender(<WorkspaceSidebar drawerMode drawerHidden/>);
  expect(history.hidden).toBe(true);
  expect(history.isConnected).toBe(true);
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

it('reselects the current destination after a linked child was opened', async () => {
  const destination=JSON.stringify(['example','records']);
  const payload=layoutPayload({applications:[{id:'example',title:'Example',menus:[{id:'records',title:'Records',action:{type:'window',windowKey:'records'}}]}]});
  payload.layout.left.navigation.presentation='sections';
  fetch.mockResolvedValue(response(payload));
  openWindow.mockReturnValue({windowId:'root'});
  render(<WorkspaceSidebar section="workspaces" destination={destination}/>);
  await waitFor(()=>expect(openWindow).toHaveBeenCalledTimes(1));
  const before=fetch.mock.calls.length;
  fireEvent.click(screen.getByRole('button',{name:'Records',exact:true}));
  await waitFor(()=>expect(openWindow).toHaveBeenCalledTimes(2));
  expect(fetch.mock.calls.length).toBe(before);
  expect(openWindow).toHaveBeenLastCalledWith('records','Records',[],expect.objectContaining({region:'app.section',parentKey:null}));
});

it('switches application menus exclusively and opens the selected report', async () => {
  const payload = layoutPayload({applications:[
    {id:'records',title:'Records',menus:[{id:'list',title:'Record list',action:{type:'window',windowKey:'records'}}]},
    {id:'analytics',title:'Analytics',menus:[{id:'summary',title:'Summary report',action:{type:'report',resourceUri:'report://example/summary',providerIdentity:'example'}}]},
  ]});
  payload.layout.left.navigation.presentation = 'sections';
  fetch.mockResolvedValue(response(payload));
  const onSectionChange = vi.fn();
  render(<WorkspaceSidebar section="workspaces" onSectionChange={onSectionChange}/>);
  await screen.findByRole('button', {name:'Record list'});
  fireEvent.click(screen.getByRole('tab', {name:'Analytics'}));
  expect(screen.queryByRole('button', {name:'Record list'})).toBeNull();
  expect(screen.getByRole('tab', {name:'Analytics'}).getAttribute('aria-selected')).toBe('true');
  fireEvent.click(screen.getByRole('button', {name:'Summary report'}));
  expect(onSectionChange).toHaveBeenCalledWith('workspaces', JSON.stringify(['analytics','summary']));
  fireEvent.keyDown(screen.getByRole('tab', {name:'Analytics'}), {key:'ArrowLeft'});
  expect(screen.getByRole('button', {name:'Record list'})).toBeTruthy();
  expect(screen.queryByRole('button', {name:'Summary report'})).toBeNull();
});

it('makes an empty application menu explicit and allows reloading it', async () => {
  const payload = layoutPayload({applications:[{id:'example',title:'Example',menus:[]}]});
  payload.layout.left.navigation.presentation = 'sections';
  fetch.mockResolvedValue(response(payload));
  render(<WorkspaceSidebar section="workspaces"/>);
  expect((await screen.findByRole('status')).textContent).toContain('No navigation is available for Example');
  const calls = fetch.mock.calls.length;
  fireEvent.click(screen.getByRole('button', {name:'Reload menu'}));
  await waitFor(() => expect(fetch.mock.calls.length).toBeGreaterThan(calls));
});

it('uses application scope for later menu opens and labels instead of another focused window', async () => {
  const {applicationScope} = await import('../services/applicationScope');
  applicationScope.clear();
  const payload = layoutPayload({applications:[
    {id:'analytics',providerIdentity:'producer',title:'Analytics',scope:{keys:['entityId','entityName']},menus:[
      {id:'select',title:'Entity',scopeLabelKey:'entityName',action:{type:'window',windowKey:'selector'}},
      {id:'summary',title:'Summary',action:{type:'window',windowKey:'summary',parameterBindings:{Entity:{from:'entityId'}}}},
    ]},
    {id:'operations',title:'Operations',menus:[]},
  ]});
  fetch.mockResolvedValue(response(payload));
  openWindow.mockImplementation((key,title,refs,options)=>{
    const entry={windowId:`opened-${key}`,windowKey:key,parameters:options.parameters};
    windowSignals.activeWindows.value=[...windowSignals.activeWindows.value,entry];return entry;
  });
  render(<WorkspaceSidebar/>);
  await screen.findByRole('button',{name:'Entity'});
  expect(screen.getByRole('button',{name:'Summary'}).disabled).toBe(true);
  fireEvent.click(screen.getByRole('button',{name:'Entity'}));
  await act(async()=>applicationScope.set({identity:{windowId:'opened-selector'}},{entityId:12,entityName:'Selected entity'}));
  expect(screen.getByRole('button',{name:'Entity: Selected entity'})).toBeTruthy();
  await act(async()=>{
    windowSignals.activeWindows.value=[...windowSignals.activeWindows.value,{windowId:'other-app',parameters:{entityId:99}}];
    windowSignals.selectedWindowId.value='other-app';
  });
  fireEvent.click(screen.getByRole('button',{name:'Summary'}));
  expect(openWindow).toHaveBeenLastCalledWith('summary','Summary',[],expect.objectContaining({parameters:{Entity:12}}));
  applicationScope.clear();
});

it('refreshes published menus on focus without reopening the current window or switching apps', async () => {
  const apps = [
    {id:'first',title:'First',menus:[]},
    {id:'second',title:'Second',menus:[{id:'records',title:'Records',action:{type:'window',windowKey:'records'}}]},
  ];
  const initial = layoutPayload({applications:apps});
  initial.layoutRevision = 'one';
  initial.layout.left.navigation.presentation = 'sections';
  initial.layout.left.width.preferenceScope = 'global';
  const next = structuredClone(initial);
  next.layoutRevision = 'two';
  next.layout.applications[1].menus.push({id:'new',title:'New report',action:{type:'report',resourceUri:'report://example/new'}});
  fetch.mockResolvedValueOnce(response(initial)).mockResolvedValue(response(next));
  openWindow.mockReturnValue({windowId:'retained'});
  render(<WorkspaceSidebar section="workspaces" destination='["second","records"]'/>);
  await screen.findByRole('button',{name:'Records',exact:true});
  expect(openWindow).toHaveBeenCalledTimes(1);
  await act(async () => { window.dispatchEvent(new Event('focus')); });
  await screen.findByRole('button',{name:'New report',exact:true});
  expect(screen.getByRole('tab',{name:'Second',exact:true}).getAttribute('aria-selected')).toBe('true');
  expect(openWindow).toHaveBeenCalledTimes(1);
  const authCalls = getAuthMeSilently.mock.calls.length;
  await act(async () => { window.dispatchEvent(new Event('focus')); });
  expect(getAuthMeSilently).toHaveBeenCalledTimes(authCalls);
  expect(openWindow).toHaveBeenCalledTimes(1);
});

it('removes revoked applications on refresh without breaking conversation history', async () => {
  const initial = layoutPayload({applications:[{id:'restricted',title:'Restricted',menus:[{id:'records',title:'Restricted records',action:{type:'window',windowKey:'records'}}]}]});
  initial.layoutRevision='one';
  const next=layoutPayload({applications:[]});
  next.layoutRevision='two';
  fetch.mockResolvedValueOnce(response(initial)).mockResolvedValue(response(next));
  render(<WorkspaceSidebar/>);
  await screen.findByRole('button',{name:'Restricted records'});
  const history=screen.getByTestId('conversation-sidebar');
  await act(async () => { window.dispatchEvent(new Event('focus')); });
  await waitFor(()=>expect(screen.queryByRole('button',{name:'Restricted records'})).toBeNull());
  expect(history.isConnected).toBe(true);
  expect(screen.queryByText(/cohort denied/i)).toBeNull();
});

it('refreshes on a bounded timer, skips hidden pages, and never overlaps requests', async () => {
  let tick;
  const interval=vi.spyOn(window,'setInterval').mockImplementation(callback=>{tick=callback;return 98765;});
  const clear=vi.spyOn(window,'clearInterval');
  const hidden=vi.spyOn(document,'hidden','get').mockReturnValue(false);
  let finish;
  const payload=layoutPayload({applications:[{id:'app',title:'App',menus:[{id:'records',title:'Records',action:{type:'window',windowKey:'records'}}]}]});
  payload.layoutRevision='one';
  fetch.mockResolvedValueOnce(response(payload)).mockImplementation(()=>new Promise(resolve=>{finish=resolve;}));
  const mounted=render(<WorkspaceSidebar/>);
  try {
    await screen.findByRole('button',{name:'Records'});
    expect(interval).toHaveBeenCalledWith(expect.any(Function),15000);
    hidden.mockReturnValue(true);
    await act(async()=>{tick();});
    expect(fetch).toHaveBeenCalledTimes(1);
    hidden.mockReturnValue(false);
    await act(async()=>{tick();tick();window.dispatchEvent(new Event('focus'));});
    expect(fetch).toHaveBeenCalledTimes(2);
    await act(async()=>{finish(response(payload));});
    mounted.unmount();
    expect(clear).toHaveBeenCalledWith(98765);
    window.dispatchEvent(new Event('focus'));
    expect(fetch).toHaveBeenCalledTimes(2);
  } finally {
    mounted.unmount(); interval.mockRestore(); clear.mockRestore(); hidden.mockRestore();
  }
});


it.each([401,403])('clears previously admitted navigation after background HTTP %s',async status=>{
  const payload=layoutPayload({applications:[{id:'example',title:'Example',menus:[{id:'records',title:'Records',action:{type:'window',windowKey:'records'}}]}]});
  payload.layoutRevision='one';
  fetch.mockResolvedValueOnce(response(payload)).mockResolvedValue(response({},status));
  const topbar=vi.fn();
  render(<WorkspaceSidebar onTopbarActionsChange={topbar}/>);
  await screen.findByRole('button',{name:'Records',exact:true});
  await act(async()=>window.dispatchEvent(new Event('focus')));
  expect(screen.queryByRole('button',{name:'Records',exact:true})).toBeNull();
  expect(topbar).toHaveBeenLastCalledWith([]);
});

it.each(['agently:session-reset','agently:logout','agently:authorized'])('discards retained identity and late reads on %s',async event=>{
  const initial=layoutPayload({applications:[{id:'restricted',title:'Restricted',menus:[{id:'records',title:'Restricted records',action:{type:'window',windowKey:'records'}}]}]});
  initial.layoutRevision='first-owner';
  let finishPrevious;
  fetch.mockResolvedValue(response({},503)).mockResolvedValueOnce(response(initial)).mockImplementationOnce(()=>new Promise(resolve=>{finishPrevious=resolve;}));
  const topbar=vi.fn();
  render(<WorkspaceSidebar onTopbarActionsChange={topbar}/>);
  await screen.findByRole('button',{name:'Restricted records',exact:true});
  await act(async()=>window.dispatchEvent(new Event('focus')));
  expect(fetch).toHaveBeenCalledTimes(2);
  await act(async()=>window.dispatchEvent(new Event(event)));
  expect(screen.queryByRole('button',{name:'Restricted records',exact:true})).toBeNull();
  expect(topbar).toHaveBeenLastCalledWith([]);
  getAuthMeSilently.mockResolvedValue({subject:'another-example-user'});
  await act(async()=>finishPrevious(response(initial)));
  expect(screen.queryByRole('button',{name:'Restricted records',exact:true})).toBeNull();
  fetch.mockResolvedValue(response({},503));
  await act(async()=>window.dispatchEvent(new Event('agently:authorized')));
  expect(screen.queryByRole('button',{name:'Restricted records',exact:true})).toBeNull();
  await act(async()=>window.dispatchEvent(new Event('focus')));
  expect(screen.queryByRole('button',{name:'Restricted records',exact:true})).toBeNull();
  const next=layoutPayload({applications:[{id:'public',title:'Public',menus:[{id:'summary',title:'Public summary',action:{type:'window',windowKey:'summary'}}]}]});
  next.layoutRevision='next-owner';
  fetch.mockResolvedValue(response(next));
  await act(async()=>window.dispatchEvent(new Event('focus')));
  expect(screen.getByRole('button',{name:'Public summary',exact:true})).toBeTruthy();
  expect(screen.queryByRole('button',{name:'Restricted records',exact:true})).toBeNull();
});

it.each(['split', 'sections'])('renders portable group and leaf drawings while preserving %s navigation actions', async presentation => {
  // Independently authored shapes exercise filled outlines and stroke geometry.
  const groupIcon = {viewBox: [0, 0, 20, 20], paths: ['M2 3L17 3L9 18Z'], fill: 'currentColor', stroke: 'none'};
  const leafIcon = {viewBox: [0, 0, 24, 24], rects: [{x: 3, y: 4, width: 17, height: 15, rx: 2}]};
  const payload = layoutPayload({applications: [{id: 'tools', title: 'Tools', menus: [{id: 'group', title: 'Shapes', iconSvg: groupIcon, children: [
    {id: 'leaf', title: 'Inspect shape', iconSvg: leafIcon, action: {type: 'window', windowKey: 'shapeInspector', parameters: {shape: 7}}},
  ]}]}]});
  payload.layout.left.navigation.presentation = presentation;
  fetch.mockResolvedValue(response(payload));
  const onSectionChange = vi.fn();
  render(<WorkspaceSidebar section="workspaces" onSectionChange={onSectionChange}/>);
  const group = await screen.findByRole('button', {name: 'Shapes'});
  expect(group.querySelector('svg[viewBox="0 0 20 20"]')?.getAttribute('fill')).toBe('currentColor');
  expect(group.querySelector('svg[viewBox="0 0 20 20"]')?.getAttribute('stroke')).toBe('none');
  const leaf = screen.getByRole('button', {name: 'Inspect shape'});
  expect(leaf.querySelector('svg[viewBox="0 0 24 24"] rect')?.getAttribute('width')).toBe('17');
  fireEvent.click(group);
  expect(screen.queryByRole('button', {name: 'Inspect shape'})).toBeNull();
  fireEvent.click(group);
  fireEvent.click(screen.getByRole('button', {name: 'Inspect shape'}));
  if (presentation === 'sections') expect(onSectionChange).toHaveBeenCalledWith('workspaces', JSON.stringify(['tools', 'group', 'leaf']));
  else expect(openWindow).toHaveBeenCalledWith('shapeInspector', 'Inspect shape', [], expect.objectContaining({parameters: {shape: 7}}));
});

it('refreshes query navigation with only the selected application context, preserving false and zero', async () => {
  const {applicationScope} = await import('../services/applicationScope');
  applicationScope.clear();
  const apps = [
    {id:'selected',providerIdentity:'source-one',title:'Selected app',scope:{keys:['entityId','enabled','labels']},menus:[{id:'choose',title:'Choose context',action:{type:'window',windowKey:'selector'}}]},
    {id:'peer',providerIdentity:'source-two',title:'Peer app',scope:{keys:['entityId','privateValue']},menus:[]},
  ];
  const payload=layoutPayload({applications:apps});payload.layoutRevision='same-source-revision';
  fetch.mockImplementation(async url=>{
    const selected=new URL(url,window.location.origin).searchParams;
    if(!selected.has('parameters'))return response(payload);
    const next=structuredClone(payload);const context=JSON.parse(selected.get('parameters'));
    if(Object.hasOwn(context,'entityId'))next.layout.applications[0].menus.push({id:'scoped',title:`Rows for ${context.entityId}`,action:{type:'window',windowKey:'rows'}});
    return response(next);
  });
  openWindow.mockImplementation(()=>{const entry={windowId:'selector-root',parameters:{}};windowSignals.activeWindows.value=[entry];return entry;});
  render(<WorkspaceSidebar/>);fireEvent.click(await screen.findByRole('button',{name:'Choose context'}));
  const initialCalls=fetch.mock.calls.length;
  await act(async()=>{
    const peer={windowId:'peer-root',parameters:{}};windowSignals.activeWindows.value=[...windowSignals.activeWindows.value,peer];
    applicationScope.register(peer,apps[1],{});applicationScope.set({identity:{windowId:peer.windowId}},{entityId:99,privateValue:'peer-only'});
    windowSignals.selectedWindowId.value=peer.windowId;windowSignals.getFormSignal.mockReturnValue({value:{entityId:88,focusedOnly:'omit'}});
  });
  expect(fetch).toHaveBeenCalledTimes(initialCalls);
  await act(async()=>applicationScope.set({identity:{windowId:'selector-root'}},{entityId:0,enabled:false,labels:['A']}));
  await screen.findByRole('button',{name:'Rows for 0'});
  const query=new URL(fetch.mock.calls.at(-1)[0],window.location.origin).searchParams;
  expect(query.get('applicationId')).toBe('selected');expect(query.get('providerIdentity')).toBe('source-one');
  expect(JSON.parse(query.get('parameters'))).toEqual({entityId:0,enabled:false,labels:['A']});
  expect(screen.getByRole('tab',{name:'Selected app'}).getAttribute('aria-selected')).toBe('true');
  expect(applicationScope.inputs(apps[0])).toEqual({entityId:0,enabled:false,labels:['A']});
  applicationScope.clear();
});

it('supersedes pending context pulls and ignores late responses without losing selected application scope',async()=>{
  const {applicationScope}=await import('../services/applicationScope');applicationScope.clear();
  const app={id:'query',providerIdentity:'owner',title:'Query app',scope:{keys:['entityId']},menus:[{id:'choose',title:'Choose context',action:{type:'window',windowKey:'selector'}}]};
  const initial=layoutPayload({applications:[app]});initial.layoutRevision='constant';
  const pending=new Map();
  fetch.mockImplementation(url=>{
    const query=new URL(url,window.location.origin).searchParams;
    if(!query.has('parameters'))return Promise.resolve(response(initial));
    const context=JSON.parse(query.get('parameters'));
    return new Promise(resolve=>pending.set(context.entityId,resolve));
  });
  openWindow.mockImplementation(()=>{const entry={windowId:'query-selector',parameters:{}};windowSignals.activeWindows.value=[entry];return entry;});
  render(<WorkspaceSidebar/>);fireEvent.click(await screen.findByRole('button',{name:'Choose context'}));
  await act(async()=>applicationScope.set({identity:{windowId:'query-selector'}},{entityId:1}));await waitFor(()=>expect(pending.has(1)).toBe(true));
  await act(async()=>applicationScope.set({identity:{windowId:'query-selector'}},{entityId:2}));await waitFor(()=>expect(pending.has(2)).toBe(true));
  const result=id=>{const payload=structuredClone(initial);payload.layout.applications[0].menus.push({id:'rows',title:`Rows ${id}`,action:{type:'window',windowKey:'rows'}});return response(payload);};
  await act(async()=>pending.get(2)(result(2)));await screen.findByRole('button',{name:'Rows 2'});
  await act(async()=>pending.get(1)(result(1)));expect(screen.queryByRole('button',{name:'Rows 1'})).toBeNull();
  expect(screen.getByText('Query app',{exact:true})).toBeTruthy();expect(applicationScope.inputs(app)).toEqual({entityId:2});
  applicationScope.clear();
});

it('clears selected context before an authorized session reload and ignores the old actor response',async()=>{
  const {applicationScope}=await import('../services/applicationScope');applicationScope.clear();
  const app={id:'scoped',providerIdentity:'owner',title:'Scoped app',scope:{keys:['entityId']},menus:[{id:'choose',title:'Choose context',action:{type:'window',windowKey:'selector'}}]};
  const initial=layoutPayload({applications:[app]});let finishOld;
  fetch.mockImplementation(url=>new URL(url,window.location.origin).searchParams.has('parameters')?new Promise(resolve=>finishOld=resolve):Promise.resolve(response(initial)));
  openWindow.mockImplementation(()=>{const entry={windowId:'actor-selector',parameters:{}};windowSignals.activeWindows.value=[entry];return entry;});
  render(<WorkspaceSidebar/>);fireEvent.click(await screen.findByRole('button',{name:'Choose context'}));
  await act(async()=>applicationScope.set({identity:{windowId:'actor-selector'}},{entityId:7}));await waitFor(()=>expect(finishOld).toBeTypeOf('function'));
  getAuthMeSilently.mockResolvedValue({subject:'new-actor'});
  await act(async()=>window.dispatchEvent(new Event('agently:authorized')));await screen.findByRole('button',{name:'Choose context'});
  const last=new URL(fetch.mock.calls.at(-1)[0],window.location.origin);expect(last.searchParams.has('parameters')).toBe(false);expect(applicationScope.inputs(app)).toEqual({});
  const stale=structuredClone(initial);stale.layout.applications[0].menus.push({id:'stale',title:'Previous actor rows',action:{type:'window',windowKey:'stale'}});
  await act(async()=>finishOld(response(stale)));expect(screen.queryByRole('button',{name:'Previous actor rows'})).toBeNull();applicationScope.clear();
});
