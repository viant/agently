// @vitest-environment jsdom
import React from 'react';
import {readFileSync} from 'node:fs';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {act, cleanup, fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {SettingProvider, activeWindows, addWindow, selectedTabId, selectedWindowId} from 'forge/core';
import ConversationAuditWindow from './ConversationAuditWindow.jsx';
import AppSectionContent from './AppSectionContent.jsx';
import SectionNavigation from './SectionNavigation.jsx';
import {auditFilter, auditDuration} from '../services/conversationAudit.js';

const response = body => ({ok: true, json: async () => body});
const readHostSource = path => readFileSync(new URL(path, import.meta.url), 'utf8');
const capabilities = {uiEnabled: true, currentUserId: 'alice', canAuditAllUsers: false};
const mount = () => render(<SettingProvider endpoints={{}} connectorConfig={{}} services={{}}><ConversationAuditWindow/></SettingProvider>);
beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn());
  // jsdom has no layout observer; the real shared table uses the browser API.
  vi.stubGlobal('ResizeObserver', class {observe() {} unobserve() {} disconnect() {}});
});
afterEach(() => {
  cleanup(); activeWindows.value = []; selectedTabId.value = null; selectedWindowId.value = null;
  vi.unstubAllGlobals();
});

it('renders host-declared audit navigation alongside Scheduled tasks', () => {
  // Read the host declaration so a copied fixture cannot hide an unsupported icon.
  const host = readHostSource('../../../conversation_audit.go');
  const icons = [...host.matchAll(/\bIcon:\s*"([^"]+)"/g)].map(match => match[1]);
  expect(icons).toHaveLength(1);
  const tasks = {id: 'schedule', title: 'Scheduled tasks', icon: 'lucide:clock-3'};
  const audit = {id: 'conversation-audit', title: 'Conversation audit', icon: icons[0]};
  const onSelect = vi.fn();
  const view = render(<SectionNavigation actions={[tasks, audit]} onSelect={onSelect}/>);
  const navigation = screen.getByRole('navigation', {name: 'App sections'});
  expect(within(navigation).getAllByRole('button').map(button => button.title)).toEqual([
    'Chat', 'Workspaces', 'Scheduled tasks', 'Conversation audit',
  ]);
  fireEvent.click(within(navigation).getByRole('button', {name: 'Conversation audit'}));
  expect(onSelect).toHaveBeenLastCalledWith('action:conversation-audit');
  fireEvent.click(within(navigation).getByRole('button', {name: 'Scheduled tasks'}));
  expect(onSelect).toHaveBeenLastCalledWith('action:schedule');
  view.rerender(<SectionNavigation actions={[tasks]} onSelect={onSelect}/>);
  expect(within(navigation).queryByRole('button', {name: 'Conversation audit'})).toBeNull();
  expect(fetch).not.toHaveBeenCalled();
});

it('opens audit in the section pane with the current header tabs and waits for search', async () => {
  fetch.mockResolvedValue(response(capabilities));
  const header = document.createElement('header');
  document.body.append(header);
  try {
    let entry;
    await act(async () => {
      entry = addWindow('Conversation audit', null, 'conversation-audit', '', true, {}, {
        region: 'app.section',
        inlineMetadata: {windowKey: 'conversation-audit', dataSource: {}, view: {content: {id: 'audit'}}},
      });
    });
    const view = render(<SettingProvider endpoints={{}} connectorConfig={{}} services={{}}>
      <AppSectionContent window={entry} title="Conversation audit" headerMount={header}/>
    </SettingProvider>);
    await screen.findByRole('button', {name: 'Search turns'}, {timeout: 15000});
    expect(within(header).getByRole('tab', {name: 'Conversation audit'})).toBeTruthy();
    expect(view.container.querySelector('[role=tablist]')).toBeNull();
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0][0]).toMatch(/\/capabilities$/);
  } finally {
    cleanup(); header.remove();
  }
});

it('does not mount the reader when the experimental UI is disabled', async () => {
  fetch.mockResolvedValue(response({...capabilities, uiEnabled: false}));
  mount();
  expect(await screen.findByText(/experimental audit window is disabled/i)).toBeTruthy();
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(screen.queryByRole('button', {name: 'Search turns'})).toBeNull();
});

it('uses real shared controls to search and refresh without opening turn details', async () => {
  const row = {id: 'turn-1', conversationId: 'conversation-1', conversationTitle: 'Example', userId: 'alice', status: 'completed', feedbackRating: 1};
  fetch.mockImplementation(async url => {
    if (url.endsWith('/capabilities')) return response(capabilities);
    if (url.includes('/turns?')) return response({rows: [row]});
    throw new Error(`Unexpected audit request: ${url}`);
  });
  mount();
  await screen.findByRole('button', {name: 'Search turns'});
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(screen.getByRole('button', {name: 'Export CSV'}).disabled).toBe(true);
  expect(screen.queryByRole('combobox', {name: 'Turn to inspect'})).toBeNull();
  expect(screen.queryByRole('button', {name: 'View turn'})).toBeNull();
  fireEvent.click(screen.getByRole('button', {name: 'Search turns'}));
  await screen.findByText(/1 turns on this page/);
  expect(fetch).toHaveBeenCalledTimes(2);
  const url = new URL(fetch.mock.calls[1][0], 'https://example.invalid');
  expect(url.searchParams.get('userId')).toBe('alice');
  expect(url.searchParams.get('from')).toBeTruthy();
  expect(url.searchParams.get('to')).toBeTruthy();
  expect(url.searchParams.get('includeContent')).toBe('true');
  expect(screen.getByRole('button', {name: 'Export CSV'}).disabled).toBe(false);
  const table = document.querySelector('.conversation-audit-table');
  await within(table).findByText('Example', {exact: true});
  await act(async () => fireEvent.click(within(table).getByText('Example', {exact: true})));
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(screen.queryByRole('combobox', {name: 'Turn to inspect'})).toBeNull();
  expect(screen.queryByRole('button', {name: 'View turn'})).toBeNull();
  expect(screen.queryByRole('region', {name: 'Saved turn details'})).toBeNull();
  fireEvent.click(screen.getByRole('button', {name: 'Refresh'}));
  await waitFor(() => expect(screen.getByRole('button', {name: 'Refresh'}).disabled).toBe(false));
  expect(fetch).toHaveBeenCalledTimes(3);
  expect(fetch.mock.calls[2][0]).toBe(fetch.mock.calls[1][0]);
  const inputs = document.querySelectorAll('input[type="text"]');
  expect(inputs.length).toBeGreaterThan(0);
  fireEvent.change(inputs[0], {target: {value: 'another-conversation'}});
  await waitFor(() => expect(screen.getByRole('button', {name: 'Export CSV'}).disabled).toBe(true));
});

it('converts bounded local dates and makes exact conversation selection independent of old dates', () => {
  expect(auditFilter({conversationId: ' c ', userId: 'alice', from: 'invalid'})).toEqual({conversationId: 'c', userId: 'alice'});
  expect(() => auditFilter({userId: 'alice'})).toThrow(/date range/);
  expect(() => auditFilter({from: '2026-01-01', to: '2026-04-01'})).toThrow(/31 days/);
  expect(auditFilter({from: '2026-10-24T12:00:00+02:00', to: '2026-10-26T12:00:00+01:00'})).toEqual({from: '2026-10-24T10:00:00.000Z', to: '2026-10-26T11:00:00.000Z'});
});

it('shows full turn content and saved metrics through the existing table', async () => {
  const row = {id: 'turn-1', conversationId: 'example-conversation-id', conversationTitle: 'Example',
    createdAt: '2026-10-29T10:00:00Z', conversationCreatedAt: '2026-10-28T09:00:00Z',
    agentIdUsed: 'example-agent', models: ['alpha/chat', 'zeta/reasoning'], inputTokens: 42, outputTokens: 0,
    executionSpanMs: 62000, input: `Long input, with Unicode: żółć\n${'input '.repeat(80)}INPUT END`,
    output: `${'output '.repeat(80)}OUTPUT END`, inputPreview: 'Obsolete input preview', outputPreview: 'Obsolete output preview'};
  fetch.mockImplementation(async url => url.endsWith('/capabilities') ? response(capabilities) : response({rows: [row]}));
  mount();
  fireEvent.click(await screen.findByRole('button', {name: 'Search turns'}));
  await screen.findByText(/1 turns on this page/);
  const table = document.querySelector('.conversation-audit-table');
  await within(table).findByText('Conversation ID', {exact: true});
  expect([...table.querySelectorAll('thead .forge-table-header-label')].map(header => header.textContent)).toEqual([
    'Turn time', 'Turn ID', 'Conversation ID', 'Conversation', 'Input', 'Output', 'User', 'Agent', 'Status', 'Models',
    'Input tokens', 'Output tokens', 'Turn duration', 'Feedback', 'Feedback comment',
    'Model calls', 'Tool calls', 'Conversation created at',
  ]);
  for (const label of ['Conversation ID', 'Agent', 'Models', 'Input tokens', 'Output tokens', 'Turn duration', 'Input', 'Output']) {
    expect(within(table).getByText(label, {exact: true})).toBeTruthy();
  }
  expect(within(table).getByText('example-conversation-id')).toBeTruthy();
  expect(within(table).getByText('turn-1')).toBeTruthy();
  expect(within(table).getByText('example-agent')).toBeTruthy();
  expect(within(table).getByText('alpha/chat, zeta/reasoning')).toBeTruthy();
  expect(within(table).getByText('1 min 2 s')).toBeTruthy();
  expect(within(table).getByText('42', {exact: true})).toBeTruthy();
  expect(within(table).getByText('0', {exact: true})).toBeTruthy();
  const contents = [...table.querySelectorAll('.cell-content')].map(cell => cell.textContent);
  expect(contents).toContain(row.input);
  expect(contents).toContain(row.output);
  expect(within(table).queryByText(/Obsolete/)).toBeNull();
  expect(fetch).toHaveBeenCalledTimes(2, 'no per-row detail requests');
});

it('preserves zero durations and distinguishes missing measurements', () => {
  expect(auditDuration(0)).toBe('0 ms');
  expect(auditDuration(999)).toBe('999 ms');
  expect(auditDuration(1001)).toBe('1.001 s');
  expect(auditDuration(3600000)).toBe('1 h');
  for (const value of [undefined, null, -1, NaN]) expect(auditDuration(value)).toBe('unknown');
});

it('connects the shared table pager to bounded API cursors without inventing a last page', async () => {
  fetch.mockImplementation(async url => {
    if (url.endsWith('/capabilities')) return response(capabilities);
    const next = new URL(url, 'https://example.invalid').searchParams.has('cursor');
    return response({rows: [{id: next ? 'turn-2' : 'turn-1', conversationId: 'example', conversationTitle: next ? 'Second page' : 'First page'}], ...(next ? {} : {nextCursor: 'saved-cursor'})});
  });
  mount();
  fireEvent.click(await screen.findByRole('button', {name: 'Search turns'}));
  await screen.findByText('First page');
  expect(screen.getByRole('button', {name: 'Last Page'}).disabled).toBe(true);
  await waitFor(() => expect(screen.getByRole('button', {name: 'Next Page'}).disabled).toBe(false));
  fireEvent.click(screen.getByRole('button', {name: 'Next Page'}));
  await screen.findByText('Second page');
  expect(new URL(fetch.mock.calls[2][0], 'https://example.invalid').searchParams.get('cursor')).toBe('saved-cursor');
  expect(screen.getByRole('button', {name: 'Next Page'}).disabled).toBe(true);
  fireEvent.click(screen.getByRole('button', {name: 'Previous Page'}));
  await screen.findByText('First page');
  expect(new URL(fetch.mock.calls[3][0], 'https://example.invalid').searchParams.has('cursor')).toBe(false);
});

it('keeps cursor history across three full pages, refresh and changed filters', async () => {
  const queries = [];
  fetch.mockImplementation(async url => {
    if (url.endsWith('/capabilities')) return response({...capabilities, canAuditAllUsers: true});
    const query = new URL(url, 'https://example.invalid').searchParams;
    queries.push(query);
    const cursor = query.get('cursor');
    const page = cursor === 'cursor-3' ? 3 : cursor === 'cursor-2' ? 2 : 1;
    return response({rows: Array.from({length: page === 3 ? 1 : Number(query.get('limit'))}, (_, i) => ({
      id: `turn-${page}-${i}`, conversationId: 'example', conversationTitle: `Page ${page} row ${i}`,
    })), ...(page < 3 ? {nextCursor: `cursor-${page + 1}`} : {})});
  });
  mount();
  fireEvent.click(await screen.findByRole('button', {name: 'Search turns'}));
  await screen.findByText('Page 1 row 0');
  const go = async (label, title) => {
    const button = screen.getByRole('button', {name: label, exact: true});
    await waitFor(() => expect(button.disabled).toBe(false));
    fireEvent.click(button);
    await screen.findByText(title, {exact: true});
  };
  await go('Next Page', 'Page 2 row 0');
  await go('Next Page', 'Page 3 row 0');
  expect(screen.getByRole('button', {name: 'Next Page'}).disabled).toBe(true);
  await go('Previous Page', 'Page 2 row 0');
  await go('Next Page', 'Page 3 row 0');
  await go('First Page', 'Page 1 row 0');
  await go('Next Page', 'Page 2 row 0');
  await go('Refresh', 'Page 1 row 0');
  expect(screen.getByText(/Page 1\. 50 turns on this page/)).toBeTruthy();
  expect(queries.map(query => query.get('cursor'))).toEqual([
    null, 'cursor-2', 'cursor-3', 'cursor-2', 'cursor-3', null, 'cursor-2', null,
  ]);
  await go('Next Page', 'Page 2 row 0');
  fireEvent.change(screen.getByRole('combobox', {name: 'Rows per page'}), {target: {value: '20'}});
  await screen.findByText(/Page 1\. 20 turns on this page/);
  expect(queries.at(-1).has('cursor')).toBe(false);
  expect(queries.at(-1).get('limit')).toBe('20');
  await go('Next Page', 'Page 2 row 0');
  expect(queries.at(-1).get('limit')).toBe('20');
  fireEvent.change(screen.getByRole('textbox', {name: 'Conversation owner ID'}), {target: {value: 'bob'}});
  await waitFor(() => expect(screen.getByRole('button', {name: 'Next Page'}).disabled).toBe(true));
  await go('Search turns', 'Page 1 row 0');
  await waitFor(() => expect(queries.at(-1).get('userId')).toBe('bob'));
  expect(queries.at(-1).has('cursor')).toBe(false);
}, 30000);

it('allows editing owner ID but handles a server denial without showing another owner’s data', async () => {
  fetch.mockImplementation(async url => {
    if (url.endsWith('/capabilities')) return response(capabilities);
    const owner = new URL(url, 'https://example.invalid').searchParams.get('userId');
    return owner === 'bob' ? {ok: false, status: 403, json: async () => ({error: 'audit access denied'})}
      : response({rows: [{id: 'own-turn', conversationTitle: 'Own conversation'}]});
  });
  mount();
  const owner = await screen.findByRole('textbox', {name: 'Conversation owner ID'});
  expect(owner.readOnly).toBe(false);
  expect(screen.queryByRole('checkbox', {name: /All users/})).toBeNull();
  expect(screen.getByText(/Other owners require additional audit access/)).toBeTruthy();
  fireEvent.click(screen.getByRole('button', {name: 'Search turns'}));
  await screen.findByText('Own conversation');
  fireEvent.change(owner, {target: {value: 'bob'}});
  fireEvent.click(screen.getByRole('button', {name: 'Search turns'}));
  await screen.findByRole('alert');
  expect(screen.getByRole('alert').textContent).toContain('audit access denied');
  expect(screen.getByRole('button', {name: 'Export CSV'}).disabled).toBe(true);
  expect(new URL(fetch.mock.calls.at(-1)[0], 'https://example.invalid').searchParams.get('userId')).toBe('bob');
});
