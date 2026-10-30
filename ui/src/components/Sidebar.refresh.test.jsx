// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Sidebar from './Sidebar';
import { client } from '../services/agentlyClient';
import { openConversationInMainWindow } from '../services/conversationWindow';
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
  async function mount(appearance = 'document', initialPage = page) {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    vi.useFakeTimers();
    client.listConversations.mockReset().mockResolvedValue(initialPage);
    container = document.createElement('div');
    document.body.append(container);
    root = createRoot(container);
    await act(async () => root.render(React.createElement(Sidebar, { appearance, history: { list: { virtualized: false } } })));
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
});
