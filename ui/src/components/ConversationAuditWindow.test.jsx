// @vitest-environment jsdom
import React from 'react';
import {readFileSync} from 'node:fs';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {act, cleanup, configure, fireEvent, render, screen, waitFor, within} from '@testing-library/react';
import {SettingProvider, activeWindows, addWindow, selectedTabId, selectedWindowId} from 'forge/core';
import ConversationAuditWindow from './ConversationAuditWindow.jsx';
import AppSectionContent from './AppSectionContent.jsx';
import SectionNavigation from './SectionNavigation.jsx';
import {auditFilter, auditDuration} from '../services/conversationAudit.js';

// These integration tests render the real Forge table and its full set of cells.
vi.setConfig({testTimeout: 20000});
configure({asyncUtilTimeout: 5000});

const response = body => ({ok: true, json: async () => body});
const readHostSource = path => readFileSync(new URL(path, import.meta.url), 'utf8');
const capabilities = {uiEnabled: true, canAudit: true, currentUserId: 'alice', canAuditAllUsers: false};
const mount = () => render(<SettingProvider endpoints={{}} connectorConfig={{}} services={{}}><ConversationAuditWindow/></SettingProvider>);
const chooseUsers = async label => {
  fireEvent.click(screen.getByRole('button', {name: 'Users', exact: true}));
  fireEvent.click(await screen.findByText(label, {exact: true}));
};
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
  expect(url.searchParams.get('includeTotal')).toBe('true');
  expect(url.searchParams.get('limit')).toBe('10');
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
    const size = Number(query.get('limit'));
    return response({rows: Array.from({length: Math.min(size, 21 - (page - 1) * size)}, (_, i) => ({
      id: `turn-${page}-${i}`, conversationId: 'example', conversationTitle: `Page ${page} row ${i}`,
    })), ...((page * size < 21) ? {nextCursor: `cursor-${page + 1}`} : {}),
    ...(query.get('includeTotal') === 'true' ? {totalCount: 21} : {})});
  });
  mount();
  fireEvent.click(await screen.findByRole('button', {name: 'Search turns'}));
  await screen.findByText('Page 1 row 0');
  const bar = () => document.querySelector('.conversation-audit-table .toolbar-center');
  expect(within(bar()).getByText('Page 1 of 3')).toBeTruthy();
  expect(screen.getByRole('combobox', {name: 'Rows per page'}).value).toBe('10');
  const go = async (label, title) => {
    const button = screen.getByRole('button', {name: label, exact: true});
    await waitFor(() => expect(button.disabled).toBe(false));
    fireEvent.click(button);
    await screen.findByText(title, {exact: true});
  };
  await go('Next Page', 'Page 2 row 0');
  expect(within(bar()).getByText('Page 2 of 3')).toBeTruthy();
  await go('Next Page', 'Page 3 row 0');
  expect(screen.getByRole('button', {name: 'Next Page'}).disabled).toBe(true);
  await go('Previous Page', 'Page 2 row 0');
  await go('Next Page', 'Page 3 row 0');
  await go('First Page', 'Page 1 row 0');
  await go('Next Page', 'Page 2 row 0');
  await go('Refresh', 'Page 1 row 0');
  expect(screen.getByText(/10 turns on this page/)).toBeTruthy();
  expect(queries.map(query => query.get('includeTotal'))).toEqual(['true', null, null, null, null, null, null, 'true']);
  expect(queries.map(query => query.get('cursor'))).toEqual([
    null, 'cursor-2', 'cursor-3', 'cursor-2', 'cursor-3', null, 'cursor-2', null,
  ]);
  await go('Next Page', 'Page 2 row 0');
  fireEvent.change(screen.getByRole('combobox', {name: 'Rows per page'}), {target: {value: '20'}});
  await screen.findByText(/20 turns on this page/);
  expect(within(bar()).getByText('Page 1 of 2')).toBeTruthy();
  expect(queries.at(-1).has('includeTotal')).toBe(false);
  expect(queries.at(-1).has('cursor')).toBe(false);
  expect(queries.at(-1).get('limit')).toBe('20');
  await go('Next Page', 'Page 2 row 0');
  expect(queries.at(-1).get('limit')).toBe('20');
  expect(within(bar()).getByText('Page 2 of 2')).toBeTruthy();
  await chooseUsers('Exact user ID');
  fireEvent.change(screen.getByRole('textbox', {name: 'User ID', exact: true}), {target: {value: 'bob'}});
  await waitFor(() => expect(screen.getByRole('button', {name: 'Next Page'}).disabled).toBe(true));
  await go('Search turns', 'Page 1 row 0');
  await waitFor(() => expect(queries.at(-1).get('userId')).toBe('bob'));
  expect(queries.at(-1).has('cursor')).toBe(false);
  expect(queries.at(-1).get('includeTotal')).toBe('true');
}, 30000);

it('shows empty results as Page 1 of 1 inside the navigation bar', async () => {
  fetch.mockImplementation(async url => url.endsWith('/capabilities') ? response(capabilities) : response({rows: [], totalCount: 0}));
  mount();
  fireEvent.click(await screen.findByRole('button', {name: 'Search turns'}));
  await screen.findByText(/0 turns on this page/);
  expect(within(document.querySelector('.toolbar-center')).getByText('Page 1 of 1')).toBeTruthy();
  expect(screen.getByRole('button', {name: 'Next Page'}).disabled).toBe(true);
  expect(screen.getByRole('button', {name: 'Last Page'}).disabled).toBe(true);
});

it('preserves the last successful page and total after a failed page request', async () => {
  let fail = false;
  fetch.mockImplementation(async url => {
    if (url.endsWith('/capabilities')) return response(capabilities);
    if (fail) return {ok: false, json: async () => ({error: 'Synthetic read failure'})};
    return response({rows: [{id: 'saved', conversationTitle: 'Saved result'}], nextCursor: 'next', totalCount: 80});
  });
  mount();
  fireEvent.click(await screen.findByRole('button', {name: 'Search turns'}));
  await screen.findByText('Saved result');
  fail = true;
  await waitFor(() => expect(screen.getByRole('button', {name: 'Next Page'}).disabled).toBe(false));
  fireEvent.click(screen.getByRole('button', {name: 'Next Page'}));
  await screen.findByText('Synthetic read failure');
  expect(screen.getByText('Saved result')).toBeTruthy();
  expect(within(document.querySelector('.toolbar-center')).getByText('Page 1 of 8')).toBeTruthy();
  expect(screen.getByRole('button', {name: 'Previous Page'}).disabled).toBe(true);
});

it('exports JSON with applied filters and blocks both exports until completion', async () => {
  const blob = new Blob(['[]'], {type: 'application/json'});
  const create = vi.fn(() => 'blob:synthetic');
  vi.stubGlobal('URL', class extends URL {static createObjectURL = create; static revokeObjectURL = vi.fn();});
  const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
  let finish;
  fetch.mockImplementation(async (url, options) => {
    if (url.endsWith('/capabilities')) return response(capabilities);
    if (url.endsWith('/export/json')) return new Promise(resolve => {finish = () => resolve({ok: true, blob: async () => blob});});
    return response({rows: [], totalCount: 0});
  });
  try {
    mount();
    expect((await screen.findByRole('button', {name: 'Export JSON'})).disabled).toBe(true);
    fireEvent.click(screen.getByRole('button', {name: 'Search turns'}));
    await screen.findByText(/0 turns on this page/);
    const applied = new URL(fetch.mock.calls.at(-1)[0], 'https://example.invalid').searchParams;
    fireEvent.click(screen.getByRole('button', {name: 'Export JSON'}));
    await waitFor(() => expect(finish).toBeTypeOf('function'));
    expect(screen.getByRole('button', {name: 'Export CSV'}).disabled).toBe(true);
    expect(screen.getByRole('button', {name: 'Export JSON'}).disabled).toBe(true);
    const [url, options] = fetch.mock.calls.at(-1);
    expect(url).toMatch(/\/export\/json$/);
    expect(options.credentials).toBe('include');
    expect(JSON.parse(options.body)).toEqual({userId: 'alice', from: applied.get('from'), to: applied.get('to')});
    await act(async () => finish());
    await waitFor(() => expect(click).toHaveBeenCalledOnce());
    expect(click.mock.instances[0].download).toBe('conversation-audit.json');
    expect(create).toHaveBeenCalledWith(blob);
    await waitFor(() => expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:synthetic'), {timeout: 2000});
    fireEvent.change(screen.getByRole('textbox', {name: 'Conversation ID (optional)'}), {target: {value: 'changed'}});
    await waitFor(() => expect(screen.getByRole('button', {name: 'Export JSON'}).disabled).toBe(true));
  } finally {click.mockRestore();}
});

it('aborts pending requests on close and ignores a late response', async () => {
  let late, oldSignal, requests = 0;
  fetch.mockImplementation(async (url, options) => {
    if (url.endsWith('/capabilities')) return response(capabilities);
    requests++;
    if (requests === 1) return response({rows: [{id: 'saved', conversationTitle: 'Saved result'}], totalCount: 80});
    if (requests === 2) {
      oldSignal = options.signal;
      return new Promise(resolve => {late = () => resolve(response({rows: [], totalCount: 0}));});
    }
    throw new Error('Unexpected request');
  });
  const view = mount();
  fireEvent.click(await screen.findByRole('button', {name: 'Search turns'}));
  await screen.findByText('Saved result');
  fireEvent.click(screen.getByRole('button', {name: 'Refresh'}));
  await waitFor(() => expect(late).toBeTypeOf('function'));
  expect(screen.getByText('Saved result')).toBeTruthy();
  expect(within(document.querySelector('.toolbar-center')).getByText('Page 1 of 8')).toBeTruthy();
  view.unmount();
  expect(oldSignal.aborted).toBe(true);
  await act(async () => late());
  expect(screen.queryByText('Saved result')).toBeNull();
  expect(requests).toBe(2);
});

it('locks an ordinary owner and uses the server identity even if the form is tampered with', async () => {
  fetch.mockImplementation(async url => url.endsWith('/capabilities') ? response(capabilities) : response({rows: []}));
  mount();
  const owner = await screen.findByRole('textbox', {name: 'User ID', exact: true});
  expect(owner.readOnly).toBe(true);
  expect(screen.queryByRole('checkbox', {name: /All users/})).toBeNull();
  expect(screen.queryByRole('button', {name: 'Users', exact: true})).toBeNull();
  fireEvent.change(owner, {target: {value: 'bob'}});
  fireEvent.click(screen.getByRole('button', {name: 'Search turns'}));
  await screen.findByText(/0 turns on this page/);
  expect(new URL(fetch.mock.calls.at(-1)[0], 'https://example.invalid').searchParams.get('userId')).toBe('alice');
});

it('blocks restored windows without a grant before mounting any data controls', async () => {
  fetch.mockResolvedValue(response({...capabilities, canAudit: false}));
  mount();
  await screen.findByText(/You do not have conversation audit access/);
  expect(screen.queryByRole('textbox')).toBeNull();
  expect(screen.queryByRole('button', {name: 'Search turns'})).toBeNull();
  expect(fetch).toHaveBeenCalledTimes(1);
});

it('lets admins explicitly select their own, exact, fragment and all-user scopes', async () => {
  const queries = [];
  fetch.mockImplementation(async url => {
    if (url.endsWith('/capabilities')) return response({...capabilities, canAuditAllUsers: true});
    queries.push(new URL(url, 'https://example.invalid').searchParams);
    return response({rows: [], nextCursor: 'next', totalCount: 21});
  });
  mount();
  const owner = await screen.findByRole('textbox', {name: 'User ID', exact: true});
  expect(owner.readOnly).toBe(true);
  expect(owner.value).toBe('alice');
  expect(screen.queryByRole('checkbox')).toBeNull();
  expect(screen.queryByRole('button', {name: 'Owner matching'})).toBeNull();
  const search = async () => {
    const before = queries.length;
    fireEvent.click(screen.getByRole('button', {name: 'Search turns'}));
    await waitFor(() => expect(queries).toHaveLength(before + 1));
    await waitFor(() => expect(screen.getByRole('button', {name: 'Refresh'}).disabled).toBe(false));
    return queries.at(-1);
  };
  let query = await search();
  expect(query.get('userId')).toBe('alice');
  expect(query.has('userIdContains')).toBe(false);
  await chooseUsers('User ID contains');
  expect(owner.readOnly).toBe(false);
  fireEvent.change(owner, {target: {value: ' Team_%* '}});
  expect(queries).toHaveLength(1);
  for (const label of ['Export CSV', 'Export JSON', 'Refresh', 'Next Page']) {
    expect(screen.getByRole('button', {name: label, exact: true}).disabled).toBe(true);
  }
  query = await search();
  expect(query.get('userIdContains')).toBe('Team_%*');
  expect(query.has('userId')).toBe(false);
  await chooseUsers('Exact user ID');
  expect(owner.value).toBe(' Team_%* ');
  expect(screen.getByRole('button', {name: 'Export CSV'}).disabled).toBe(true);
  query = await search();
  expect(query.get('userId')).toBe('Team_%*');
  expect(query.has('userIdContains')).toBe(false);
  expect(query.has('cursor')).toBe(false);
  await chooseUsers('All users');
  expect(owner.value).toBe('');
  expect(owner.placeholder).toBe('All users');
  expect(owner.disabled).toBe(true);
  query = await search();
  expect(query.has('userId')).toBe(false);
  expect(query.has('userIdContains')).toBe(false);
  expect(query.get('from')).toBeTruthy();
  expect(query.get('to')).toBeTruthy();
  expect(query.get('includeTotal')).toBe('true');
  await chooseUsers('My conversations');
  expect(owner.disabled).toBe(false);
  expect(owner.readOnly).toBe(true);
  expect(owner.value).toBe('alice');
  fireEvent.change(owner, {target: {value: 'tampered'}});
  query = await search();
  expect(query.get('userId')).toBe('alice');
  expect(query.has('userIdContains')).toBe(false);
});

it('preserves the owner text, conversation and edited dates across scope switches without fetching', async () => {
  fetch.mockResolvedValue(response({...capabilities, canAuditAllUsers: true}));
  mount();
  const owner = await screen.findByRole('textbox', {name: 'User ID', exact: true});
  const conversation = screen.getByRole('textbox', {name: 'Conversation ID (optional)'});
  const from = screen.getByText('Conversation created from', {exact: true}).closest('.forge-control-wrapper').querySelector('input');
  const to = screen.getByText('Conversation created before', {exact: true}).closest('.forge-control-wrapper').querySelector('input');
  fireEvent.change(from, {target: {value: '2026-10-01 08:00'}});
  fireEvent.blur(from);
  fireEvent.change(to, {target: {value: '2026-10-02 08:00'}});
  fireEvent.blur(to);
  fireEvent.change(conversation, {target: {value: 'keep-conversation'}});
  await chooseUsers('Exact user ID');
  fireEvent.change(owner, {target: {value: ' keep_owner_%* '}});
  for (const scope of ['All users', 'My conversations', 'User ID contains', 'Exact user ID']) {
    await chooseUsers(scope);
    expect(conversation.value).toBe('keep-conversation');
    expect(from.value).toBe('2026-10-01 08:00');
    expect(to.value).toBe('2026-10-02 08:00');
    if (scope === 'Exact user ID' || scope === 'User ID contains') expect(owner.value).toBe(' keep_owner_%* ');
  }
  expect(fetch).toHaveBeenCalledTimes(1);
});

it('rejects blank exact IDs and fragments instead of silently searching all users', async () => {
  fetch.mockResolvedValue(response({...capabilities, canAuditAllUsers: true}));
  mount();
  const owner = await screen.findByRole('textbox', {name: 'User ID', exact: true});
  for (const scope of ['Exact user ID', 'User ID contains']) {
    await chooseUsers(scope);
    fireEvent.change(owner, {target: {value: '  '}});
    fireEvent.click(screen.getByRole('button', {name: 'Search turns'}));
    expect(await screen.findByRole('alert')).toHaveProperty('textContent', expect.stringContaining('select All users'));
    expect(screen.getByRole('button', {name: 'Export CSV'}).disabled).toBe(true);
    expect(screen.getByRole('button', {name: 'Export JSON'}).disabled).toBe(true);
  }
  expect(fetch).toHaveBeenCalledTimes(1);
});

it('rejects empty or ambiguous fragments and preserves literal characters', () => {
  const bounds = {from: '2026-11-01', to: '2026-11-02'};
  expect(() => auditFilter({...bounds, userIdContains: ' '})).toThrow(/fragment/);
  expect(() => auditFilter({...bounds, userId: 'alice', userIdContains: 'ali'})).toThrow();
  expect(auditFilter({...bounds, userIdContains: " %_*' "}).userIdContains).toBe("%_*'");
});
