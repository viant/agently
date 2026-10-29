// @vitest-environment jsdom
import React, {useState} from 'react';
import {afterEach, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen} from '@testing-library/react';
const {openWindow, services} = vi.hoisted(() => ({openWindow: vi.fn(), services: {reportStore: {getReport: vi.fn()}}}));
vi.mock('./menuBarHelpers', () => ({openWindow}));
vi.mock('../services/agentlyClient', () => ({getAuthMeSilently: vi.fn(async () => ({subject: 'example-reader'}))}));
vi.mock('./Sidebar', () => ({default: () => <p>Conversation history</p>}));
vi.mock('forge/core', async () => {
  const {signal} = await import('@preact/signals-react');
  return {activeWindows: signal([]), selectedWindowId: signal(null), selectedTabId: signal(null), getFormSignal: () => signal({}), useSetting: () => ({services})};
});
vi.mock('forge/components', async () => {
  const {default: Typography, typographyProps} = await import('forge/components/Typography.jsx');
  const {default: Icon} = await import('forge/components/Icon.jsx');
  const {default: ReportFilterBar} = await import('forge/components/dashboard/ReportFilterBar.jsx');
  const {activeWindows} = await import('forge/core');
  return {Typography, typographyProps, Icon, ReportFilterBar,
    WindowManager: ({windowFilter, renderWindowContent}) => <>{activeWindows.value.filter(windowFilter).map(entry => <React.Fragment key={entry.windowId}>{renderWindowContent({window: entry, defaultContent: <div>Window: {entry.windowTitle}</div>})}</React.Fragment>)}</>,
    WindowContent: ({window: entry}) => <div>Window: {entry.windowTitle}</div>,
    ReportResourceRuntime: () => <article aria-label="Example report output"><p>Rendered report</p><button>Export PDF</button></article>};
});
import WorkspaceSidebar from './WorkspaceSidebar.jsx';
import AppSectionContent from './AppSectionContent.jsx';
import {activeWindows} from 'forge/core';
afterEach(() => {cleanup(); vi.unstubAllGlobals(); vi.clearAllMocks(); activeWindows.value = [];});

it('opens a menu report in the visible Workspaces pane while retaining its section and chat lane', async () => {
  const action = {type: 'report', resourceUri: 'report://example/overview', parameters: {from: '2026-01-01', to: '2026-01-02'}};
  vi.stubGlobal('fetch', vi.fn(async () => ({ok: true, json: async () => ({workspaceId: 'example', layout: {
    left: {navigation: {presentation: 'sections'}, history: {enabled: false}},
    applications: [{id: 'examples', title: 'Examples', menus: [{id: 'overview', title: 'Example report', action}]}]}})})));
  openWindow.mockImplementation((windowKey, windowTitle, _refresh, options) => {
    const entry = {windowId: 'example-report-window', windowKey, windowTitle, ...options};
    activeWindows.value = [entry]; return entry;
  });
  services.reportStore.getReport.mockResolvedValue({resource: {uri: action.resourceUri, providerIdentity: 'example-publisher', kind: 'published', contentFingerprint: 'closed-example'},
    resourceDefinition: {builderDefinition: {reportBuilder: {predicates: [{id: 'dates', label: 'Date range', kind: 'dateRange', startParamPath: 'from', endParamPath: 'to'}]}}}});
  const onOpenWorkspace = vi.fn();
  function Shell() {
    const [section, setSection] = useState('workspaces'), [destination, setDestination] = useState(''), [entry, setEntry] = useState(null);
    return <div data-primary-section={section}>
      <WorkspaceSidebar section={section} destination={destination} onSectionChange={(next, target = '') => {setSection(next); setDestination(target);}}
        onSectionWindow={setEntry} onOpenWorkspace={onOpenWorkspace}/>
      <main><AppSectionContent window={entry} title="Workspaces"/>
        <div data-testid="retained-chat-lane" style={{display: section === 'chat' ? 'flex' : 'none'}}>Retained conversation</div>
      </main></div>;
  }
  const {container} = render(<Shell/>);
  expect(screen.getByText('Choose a destination from the menu.')).not.toBeNull();
  fireEvent.click(await screen.findByRole('button', {name: 'Example report'}));
  const output = await screen.findByRole('article', {name: 'Example report output'});
  expect(screen.getByLabelText('Date range from').value).toBe('2026-01-01');
  expect(screen.getByRole('button', {name: 'Apply filters'})).not.toBeNull();
  expect(screen.getByRole('button', {name: 'Export PDF'})).not.toBeNull();
  expect(container.firstElementChild.dataset.primarySection).toBe('workspaces');
  expect(screen.getByRole('button', {name: 'Example report'}).getAttribute('aria-current')).toBe('page');
  expect(screen.queryByText('Choose a destination from the menu.')).toBeNull();
  expect(output.closest('.app-section-page')).not.toBeNull();
  expect(output.closest('[data-testid="retained-chat-lane"]')).toBeNull();
  for (let node = output; node; node = node.parentElement) {
    expect(node.hidden).toBe(false); expect(node.getAttribute('aria-hidden')).not.toBe('true'); expect(node.style.display).not.toBe('none');
  }
  expect(openWindow).toHaveBeenCalledWith(action.resourceUri, 'Example report', [], expect.objectContaining({region: 'app.section', parentKey: null, conversationId: undefined, parameters: action.parameters}));
  expect(onOpenWorkspace).not.toHaveBeenCalled();
  expect(services.reportStore.getReport).toHaveBeenCalledWith({resource: {uri: action.resourceUri}});
});
