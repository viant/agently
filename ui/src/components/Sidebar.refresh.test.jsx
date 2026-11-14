// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {fireEvent} from '@testing-library/react';
import Sidebar from './Sidebar';
import { client } from '../services/agentlyClient';
import { openConversationInMainWindow } from '../services/conversationWindow';
const appearanceState = vi.hoisted(() => ({themeId: 'baseline'}));
vi.mock('./WorkspaceStyleProvider', () => ({useWorkspaceStyle: () => ({state: appearanceState})}));
vi.mock('../services/agentlyClient', () => ({ client: { listConversations: vi.fn() } }));
vi.mock('../services/conversationWindow', () => ({
  MAIN_CHAT_WINDOW_ID: 'chat/new',
  getScopedConversationSelection: () => 'selected',
  getWindowById: vi.fn(),
  isLinkedChildWindow: () => false,
  detachWorkspaceWindowsFromConversation: vi.fn(),
  openConversationInMainWindow: vi.fn(),
  requestNewConversationInMainWindow: vi.fn()
}));

describe('mounted history refresh', () => {
  let root;
  let container;
  const page = { data: [{ Id: 'selected', Title: 'Saved conversation', Status: 'succeeded' }], page: {} };
  const button = (label) => [...container.querySelectorAll('button')].find((element) => (element.getAttribute('aria-label') || element.textContent) === label);
  const click = async (label) => act(async () => button(label).click());
  async function mount(appearance = 'document', initialPage = page, themeId = 'baseline', searchMount) {
    appearanceState.themeId = themeId;
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    vi.useFakeTimers();
    client.listConversations.mockReset().mockResolvedValue(initialPage);
    container = document.createElement('div');
    document.body.append(container);
    root = createRoot(container);
    await act(async () => root.render(React.createElement(Sidebar, { appearance, searchMount, history: { list: { virtualized: false } } })));
    await act(async () => vi.advanceTimersByTimeAsync(250));
  }
  afterEach(async () => {
    if (root) await act(async () => root.unmount());
    container?.remove();
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it.each(['document', ''])('shows failed refresh and retry while retaining rows and selection (%s)', async (appearance) => {
    await mount(appearance);
    client.listConversations.mockRejectedValueOnce(new TypeError('Failed to fetch'));
    await click(appearance ? 'Refresh conversations' : 'Refresh');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Could not refresh conversations');
    expect(container.querySelector('[role="alert"] p')?.getAttribute('data-forge-type')).toBe('small');
    expect(button('Retry').getAttribute('data-forge-variant')).toBe('secondary');
    expect(container.querySelector('.is-selected')?.textContent).toContain('Saved conversation');
    client.listConversations.mockResolvedValueOnce({ ...page, data: [{ ...page.data[0], Title: 'Updated conversation' }] });
    await click('Retry');
    expect(container.querySelector('[role="alert"]')).toBeNull();
    expect(container.querySelector('.is-selected')?.textContent).toContain('Updated conversation');
    expect(openConversationInMainWindow).not.toHaveBeenCalled();
  });

  it('shows a server refresh error even when history is already populated', async () => {
    await mount();
    client.listConversations.mockRejectedValueOnce(new Error('Service unavailable'));
    await click('Refresh conversations');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Could not refresh conversations');
    expect(container.querySelector('.is-selected')).not.toBeNull();
  });

  it('shows retry instead of an empty-history message when refreshing an empty list fails', async () => {
    await mount('document', { data: [], page: {} });
    client.listConversations.mockRejectedValueOnce(new TypeError('Failed to fetch'));
    await click('Refresh conversations');
    expect(container.querySelector('[role="alert"]')?.textContent).toContain('Check your connection and retry');
    expect(container.querySelector('.app-sidebar-empty')).toBeNull();
    client.listConversations.mockResolvedValueOnce(page);
    await click('Retry');
    expect(container.querySelector('.is-selected')?.textContent).toContain('Saved conversation');
  });

  it('fetches latest history, ends unchanged loading, and preserves selection without duplicates', async () => {
    await mount();
    let resolve;
    client.listConversations.mockImplementationOnce(() => new Promise((done) => { resolve = done; }));
    await click('Refresh conversations');
    expect(button('Refresh conversations').disabled).toBe(true);
    expect(container.querySelector('.is-selected')).not.toBeNull();
    await act(async () => resolve(page));
    expect(button('Refresh conversations').disabled).toBe(false);
    await click('Refresh conversations');
    expect(container.querySelectorAll('.app-conversation-row')).toHaveLength(1);
    expect(container.querySelector('.is-selected')).not.toBeNull();
    expect(client.listConversations).toHaveBeenLastCalledWith({ excludeScheduled: true, query: undefined, page: { limit: 12, direction: 'latest', cursor: undefined } });
  });

  it('retries the failed older page and appends without duplicating existing rows', async () => {
    await mount('document', { ...page, page: { hasOlder: true, cursor: 'selected' } });
    client.listConversations.mockRejectedValueOnce(new Error('Service unavailable'));
    await click('Load older conversations');
    expect(container.querySelector('[role="alert"]')).not.toBeNull();
    expect(container.querySelector('.is-selected')).not.toBeNull();
    client.listConversations.mockResolvedValueOnce({ data: [...page.data, { Id: 'older', Title: 'Older conversation' }], page: {} });
    await click('Retry');
    expect(client.listConversations).toHaveBeenLastCalledWith({ excludeScheduled: true, query: undefined, page: { limit: 12, direction: 'before', cursor: 'selected' } });
    expect(container.querySelectorAll('.app-conversation-row')).toHaveLength(2);
    expect(container.querySelector('.is-selected')?.textContent).toContain('Saved conversation');
  });

  it('uses existing activity and metadata events to update rows without a full list read', async () => {
    await mount();
    await act(async () => {
      window.dispatchEvent(new CustomEvent('agently:conversation-activity', { detail: { id: 'selected', type: 'turn_failed' } }));
      window.dispatchEvent(new CustomEvent('agently:conversation-meta-updated', { detail: { id: 'selected', patch: { title: 'Renamed conversation' } } }));
    });
    expect(container.querySelector('.is-selected')?.textContent).toContain('Renamed conversation');
    expect(client.listConversations).toHaveBeenCalledTimes(1);
  });

  it('uses the existing new-conversation event to read current backend history', async () => {
    await mount();
    client.listConversations.mockResolvedValueOnce({ data: [{ Id: 'new', Title: 'New conversation' }, ...page.data], page: {} });
    await act(async () => {
      window.dispatchEvent(new CustomEvent('agently:conversation-new', { detail: { id: 'new' } }));
      await vi.advanceTimersByTimeAsync(300);
    });
    expect(client.listConversations).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('New conversation');
  });

  it('expands Search Chat below New Chat and reuses history search only in Design System 2.0', async () => {
    const masthead = document.createElement('div');
    document.body.append(masthead);
    try {
      await mount('document', page, 'design-system-2', masthead);
      expect(masthead.childElementCount).toBe(0);
      expect(container.querySelector('.app-sidebar-new').nextElementSibling.className).toBe('app-sidebar-search');
      expect(button('Search Chat')).toBeTruthy();
      expect(container.querySelector('.app-sidebar-section > span').textContent).toBe('Chats');
      expect(container.querySelector('input')).toBeNull();
      await click('Search Chat');
      const input = container.querySelector('input');
      expect(input.getAttribute('aria-label')).toBe('Search chats');
      expect(input.placeholder).toBe('Search chat...');
      expect(document.activeElement).toBe(input);
      client.listConversations.mockResolvedValueOnce({data: [{Id: 'matching', Title: 'Matching report'}], page: {}});
      await act(async () => fireEvent.change(input, {target: {value: 'report'}}));
      await act(async () => vi.advanceTimersByTimeAsync(250));
      expect(client.listConversations).toHaveBeenLastCalledWith({excludeScheduled: true, query: 'report', page: {limit: 12, direction: 'latest', cursor: undefined}});
      expect(container.textContent).toContain('Matching report');
      expect(openConversationInMainWindow).not.toHaveBeenCalled();
      await act(async () => fireEvent.keyDown(input, {key: 'Escape'}));
      expect(container.querySelector('input')).toBeNull();
      expect(document.activeElement).toBe(button('Search Chat'));
      await act(async () => vi.advanceTimersByTimeAsync(250));
      expect(client.listConversations).toHaveBeenLastCalledWith({excludeScheduled: true, query: undefined, page: {limit: 12, direction: 'latest', cursor: undefined}});
      await click('Search Chat');
      await click('Close conversation search');
      expect(document.activeElement).toBe(button('Search Chat'));
    } finally {
      masthead.remove();
    }
  });

  it('collapses an empty inline search after focus leaves the group without stealing the next control focus', async () => {
    const outside = document.createElement('button');
    document.body.append(outside);
    try {
      await mount('document', page, 'design-system-2');
      await click('Search Chat');
      await act(async () => button('Close conversation search').focus());
      expect(container.querySelector('input')).not.toBeNull();
      await act(async () => outside.focus());
      expect(container.querySelector('input')).toBeNull();
      expect(button('Search Chat')).toBeTruthy();
      expect(document.activeElement).toBe(outside);
    } finally {outside.remove();}
  });

  it('retains a nonempty search on blur and clears whitespace when an empty search collapses', async () => {
    const outside = document.createElement('button');
    document.body.append(outside);
    try {
      await mount('document', page, 'design-system-2');
      await click('Search Chat');
      const input = container.querySelector('input');
      await act(async () => fireEvent.change(input, {target: {value: 'report'}}));
      await act(async () => outside.focus());
      expect(container.querySelector('input')).toBe(input);
      expect(input.value).toBe('report');
      expect(document.activeElement).toBe(outside);
      await act(async () => input.focus());
      await act(async () => fireEvent.change(input, {target: {value: '   '}}));
      await act(async () => outside.focus());
      expect(container.querySelector('input')).toBeNull();
      expect(document.activeElement).toBe(outside);
      await act(async () => vi.advanceTimersByTimeAsync(250));
      expect(client.listConversations).toHaveBeenLastCalledWith({excludeScheduled: true, query: undefined, page: {limit: 12, direction: 'latest', cursor: undefined}});
    } finally {outside.remove();}
  });

  it('keeps the incumbent conversation search open on blur outside Design System 2.0', async () => {
    await mount('document', page, 'baseline');
    await click('Search conversations');
    const input = container.querySelector('input');
    await act(async () => button('Refresh conversations').focus());
    expect(container.querySelector('input')).toBe(input);
  });

  it('restores the native masthead search when switching away from Design System 2.0', async () => {
    const masthead = document.createElement('div');
    document.body.append(masthead);
    try {
      await mount('document', page, 'design-system-2', masthead);
      for (const themeId of ['baseline', 'steward-neutral-preview', 'steward-verification-preview', 'viant-2026-preview']) {
        appearanceState.themeId = themeId;
        await act(async () => root.render(<Sidebar appearance="document" searchMount={masthead} history={{list: {virtualized: false}}}/>));
        expect(container.querySelector('.app-sidebar-search')).toBeNull();
        expect(button('Search Chat')).toBeUndefined();
        expect(container.querySelector('.app-sidebar-section > span').textContent).toBe('Conversations');
        expect(masthead.querySelector('button').getAttribute('aria-label')).toBe('Search conversations');
      }
      appearanceState.themeId = 'design-system-2';
      await act(async () => root.render(<Sidebar appearance="document" searchMount={masthead} history={{list: {virtualized: false}}}/>));
      expect(masthead.childElementCount).toBe(0);
      expect(button('Search Chat')).toBeTruthy();
    } finally {
      masthead.remove();
    }
  });
});
