// @vitest-environment jsdom
import React from 'react';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {act, cleanup, renderHook, waitFor} from '@testing-library/react';
import {useShellNavigation} from './useShellNavigation.js';
import {useConversationHeading} from './useConversationHeading.js';

const {getConversation} = vi.hoisted(() => ({getConversation: vi.fn()}));
vi.mock('../services/agentlyClient', () => ({client: {getConversation}}));

beforeEach(() => {window.history.replaceState(null, '', '/?retain=value'); window.localStorage.clear(); getConversation.mockReset();});
afterEach(cleanup);

it('keeps legacy path ownership and only intercepts while the section shell is enabled', () => {
  const restore = vi.fn();
  const {rerender, unmount} = renderHook(enabled => useShellNavigation({enabled, section: 'chat', conversationId: '', onRestore: restore}), {initialProps: false});
  const path = () => new CustomEvent('agently:conversation-path', {cancelable: true, detail: {pathname: '/conversation/example', conversationId: 'example'}});
  expect(window.dispatchEvent(path())).toBe(true);
  rerender(true);
  expect(window.dispatchEvent(path())).toBe(false);
  expect(window.location.pathname).toBe('/conversation/example');
  expect(window.location.search).toBe('?retain=value');
  expect(restore).toHaveBeenLastCalledWith(expect.objectContaining({section: 'chat', conversationId: 'example', selectionInProgress: true}));
  unmount();
  expect(window.dispatchEvent(path())).toBe(true);
});

it('restores section and conversation visits with native Back/Forward without adding a visit during restoration', async () => {
  const restored = vi.fn();
  const {result} = renderHook(() => {
    const [route, setRoute] = React.useState({section: 'chat', conversationId: ''});
    return useShellNavigation({...route, enabled: true, onRestore: entry => {
      setRoute(entry); restored(entry);
      if (!entry.selectionInProgress) window.dispatchEvent(new CustomEvent('agently:conversation-path', {cancelable: true, detail: {pathname: window.location.pathname, conversationId: entry.conversationId}}));
    }});
  });
  act(() => result.current.selectSection('workspaces'));
  expect(window.location.search).toContain('section=workspaces');
  const workspaces = window.history.state.agentlyShell;
  act(() => window.dispatchEvent(new CustomEvent('agently:conversation-path', {cancelable: true, detail: {pathname: '/conversation/second', conversationId: 'second'}})));
  expect(result.current.canGoBack).toBe(true);
  expect(result.current.canGoForward).toBe(false);
  act(() => result.current.back());
  await waitFor(() => expect(window.history.state.agentlyShell).toEqual(workspaces));
  expect(restored).toHaveBeenLastCalledWith(workspaces);
  expect(result.current.canGoForward).toBe(true);
  act(() => result.current.forward());
  await waitFor(() => expect(window.location.pathname).toBe('/conversation/second'));
  expect(window.history.state.agentlyShell.index).toBe(2);
  act(() => result.current.back());
  await waitFor(() => expect(result.current.canGoForward).toBe(true));
  act(() => result.current.selectSection('action:schedule'));
  expect(result.current.canGoForward).toBe(false);
  expect(window.history.state.agentlyShell.index).toBe(2);
});

it('loads only the conversation heading and ignores late or stale responses and another conversation’s updates', async () => {
  let resolveA, resolveB;
  getConversation.mockImplementation(id => new Promise(resolve => {if (id === 'a') resolveA = resolve; else resolveB = resolve;}));
  const {result, rerender} = renderHook(id => useConversationHeading(id), {initialProps: 'a'});
  expect(getConversation).toHaveBeenCalledWith('a', {includeTranscript: false});
  rerender('b');
  act(() => window.dispatchEvent(new CustomEvent('agently:conversation-meta-updated', {detail: {id: 'b', patch: {title: 'Updated B'}}})));
  await act(async () => {resolveA({id: 'a', title: 'Old A'}); resolveB({id: 'b', title: 'Old B'});});
  expect(result.current).toBe('Updated B');
  act(() => window.dispatchEvent(new CustomEvent('agently:conversation-title-seed', {detail: {id: 'a', title: 'Another A'}})));
  expect(result.current).toBe('Updated B');
  rerender('');
  expect(result.current).toBe('New Chat');
});

it('keeps a readable title when heading lookup fails', async () => {
  getConversation.mockRejectedValue(new Error('offline'));
  const {result} = renderHook(() => useConversationHeading('unavailable'));
  await act(async () => {});
  expect(result.current).toBe('Conversation');
});

it('restores catalog destination IDs from links and keeps each visit distinct', async () => {
  const first = JSON.stringify(['business', 'browse', 'accounts']);
  const second = JSON.stringify(['business', 'browse', 'campaigns']);
  window.history.replaceState(null, '', `/?retain=value&section=workspaces&destination=${encodeURIComponent(first)}`);
  const restore = vi.fn();
  const {result, unmount} = renderHook(() => useShellNavigation({enabled: true, section: 'workspaces', conversationId: 'chat-a', onRestore: restore}));
  expect(restore).toHaveBeenLastCalledWith(expect.objectContaining({section: 'workspaces', destination: first}));
  act(() => result.current.selectSection('workspaces', second));
  expect(new URL(window.location.href).searchParams.get('destination')).toBe(second);
  expect(window.location.search).toContain('retain=value');
  act(() => result.current.back());
  await waitFor(() => expect(new URL(window.location.href).searchParams.get('destination')).toBe(first));
  expect(restore).toHaveBeenLastCalledWith(expect.objectContaining({destination: first}));
  act(() => result.current.forward());
  await waitFor(() => expect(new URL(window.location.href).searchParams.get('destination')).toBe(second));
  act(() => result.current.selectSection('chat', second));
  expect(new URL(window.location.href).searchParams.has('destination')).toBe(false);
  expect(restore).toHaveBeenLastCalledWith(expect.objectContaining({destination: ''}));
  unmount();
});
