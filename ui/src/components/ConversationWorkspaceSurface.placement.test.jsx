// @vitest-environment jsdom
import React, {useEffect, useState} from 'react';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {act, cleanup, fireEvent, render, screen} from '@testing-library/react';

const theme = vi.hoisted(() => ({id: 'design-system-2'}));
vi.mock('./WorkspaceStyleProvider', () => ({useWorkspaceStyle: () => ({state: {themeId: theme.id}})}));
vi.mock('./WorkspaceWindowContent.jsx', () => ({default: () => null}));
vi.mock('./InlineReportWorkspace.jsx', () => ({default: () => null}));
vi.mock('./ToolFeedDetail.jsx', () => ({default: () => null}));
vi.mock('./mcpApps/AppRenderer.jsx', () => ({default: () => null}));

import ConversationWorkspaceSurface from './ConversationWorkspaceSurface.jsx';

beforeEach(() => {
  theme.id = 'design-system-2';
  window.matchMedia = vi.fn(() => ({matches: false, addEventListener() {}, removeEventListener() {}}));
});
afterEach(() => {cleanup();vi.restoreAllMocks();vi.unstubAllGlobals();});

const report = {windowId: 'sample-report', windowTitle: 'Example report'};
function Review({onMount = () => {}, entry = report}) {
  const [mode, setMode] = useState('split');
  const [header, setHeader] = useState(null);
  const [chatHeader, setChatHeader] = useState(null);
  return <><header><div data-testid="chat-header"><h1>Chat</h1><div ref={setChatHeader}/></div><div ref={setHeader}/></header><ConversationWorkspaceSurface presentation="document" activeSurface="workspace"
    headerMount={header} chatHeaderMount={chatHeader} workspaceMode={mode} onChangeWorkspaceMode={setMode} workspaceWindow={entry}
    renderConversation={() => <Draft onMount={onMount}/>} renderWorkspace={() => <input aria-label="Report note" defaultValue="Keep report edit"/>}/></>;
}
function Draft({onMount}) {
  useEffect(onMount, []);
  return <textarea aria-label="Message" defaultValue="Keep chat draft"/>;
}
function choose(name) {
  fireEvent.click(screen.getByRole('button', {name: 'Chat options'}));
  fireEvent.click(screen.getByRole('menuitem', {name}));
}

it('owns the split menu in the shell header and the overlay menu in its floating panel', () => {
  const view = render(<Review/>);
  const header = screen.getByTestId('chat-header');
  expect(header.contains(screen.getByRole('button', {name: 'Chat options'}))).toBe(true);
  expect(view.container.querySelector('.app-report-chat-controls')).toBeNull();
  choose('Split on right');
  expect(header.contains(screen.getByRole('button', {name: 'Chat options'}))).toBe(true);
  choose('Overlay');
  expect(header.querySelector('button')).toBeNull();
  expect(view.container.querySelector('.app-report-chat-controls').contains(screen.getByRole('button', {name: 'Chat options'}))).toBe(true);
  choose('Split on left');
  expect(header.contains(screen.getByRole('button', {name: 'Chat options'}))).toBe(true);
  expect(view.container.querySelector('.app-report-chat-controls')).toBeNull();
});

it('uses the existing menu to switch left, right and overlay without remounting the draft or report', () => {
  const mounted = vi.fn();
  const view = render(<Review onMount={mounted}/>);
  const surface = view.container.querySelector('.app-summary-surface-shell');
  const draft = screen.getByLabelText('Message'), note = screen.getByLabelText('Report note');
  expect(surface.dataset.chatPlacement).toBe('left');
  choose('Split on right');
  expect(surface.dataset.chatPlacement).toBe('right');
  fireEvent.click(screen.getByRole('button', {name: 'Chat options'}));
  expect(screen.getByRole('menuitem', {name: 'Split on right'}).getAttribute('aria-current')).toBe('true');
  fireEvent.click(screen.getByRole('menuitem', {name: 'Overlay'}));
  expect(surface.dataset.chatPlacement).toBe('overlay');
  expect(surface.classList.contains('is-split')).toBe(false);
  expect(draft.closest('.app-summary-conversation-content').hidden).toBe(false);
  expect(screen.queryByRole('button', {name: 'Show composer'})).toBeNull();
  choose('Split on left');
  expect(surface.dataset.chatPlacement).toBe('left');
  expect(screen.getByLabelText('Message')).toBe(draft);
  expect(screen.getByLabelText('Report note')).toBe(note);
  expect(draft.value).toBe('Keep chat draft');
  expect(note.value).toBe('Keep report edit');
  expect(mounted).toHaveBeenCalledOnce();
});

it('keeps split choices unavailable when the existing report capability prohibits splitting', () => {
  const view = render(<Review entry={{...report, workspaceObject: {capabilities: {split: false}}}}/>);
  expect(view.container.querySelector('.app-summary-surface-shell').dataset.chatPlacement).toBe('overlay');
  fireEvent.click(screen.getByRole('button', {name: 'Chat options'}));
  expect(screen.getByRole('menuitem', {name: 'Split on left'}).getAttribute('aria-disabled')).toBe('true');
  expect(screen.getByRole('menuitem', {name: 'Split on right'}).getAttribute('aria-disabled')).toBe('true');
  expect(screen.getByRole('menuitem', {name: 'Overlay'}).getAttribute('aria-current')).toBe('true');
});

it('defaults an unspecified DS2 report layout to split on the left', () => {
  const view=render(<ConversationWorkspaceSurface presentation="document" activeSurface="workspace" workspaceWindow={report}
    renderConversation={()=><textarea aria-label="Message"/>} renderWorkspace={()=><span>Report</span>}/>);
  expect(view.container.querySelector('.app-summary-surface-shell').dataset.chatPlacement).toBe('left');
});

it.each(['baseline', 'legacy'])('preserves the existing report controls outside DS2 (%s)', id => {
  theme.id = id;
  const view = render(<Review/>);
  expect(view.container.querySelector('.app-summary-surface-shell').hasAttribute('data-chat-placement')).toBe(false);
  expect(screen.queryByRole('button', {name: 'Chat options'})).toBeNull();
  expect(screen.getByRole('button', {name: 'Show chat as overlay'})).toBeTruthy();
});

it('restores the existing controls and keeps the draft when switching away from DS2', () => {
  const view = render(<Review/>);
  const draft = screen.getByLabelText('Message');
  choose('Overlay');
  theme.id = 'baseline';
  view.rerender(<Review/>);
  expect(screen.queryByRole('button', {name: 'Chat options'})).toBeNull();
  expect(screen.getByRole('button', {name: 'Show chat on the left'})).toBeTruthy();
  expect(screen.getByLabelText('Message')).toBe(draft);
});

it('uses the existing container-width rule for overlay and restores the chosen split side after resizing', async () => {
  let width = 1000, notify;
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockImplementation(() => width);
  vi.stubGlobal('ResizeObserver', class {constructor(callback) {this.callback=callback;} observe(element) {if(element.classList.contains('app-summary-surface-shell')) notify=()=>this.callback();} disconnect() {}});
  const view = render(<Review/>);
  const surface = view.container.querySelector('.app-summary-surface-shell');
  const draft = screen.getByLabelText('Message');
  choose('Split on right');
  width=390;await act(async () => {notify();});
  expect(surface.dataset.chatPlacement).toBe('overlay');
  fireEvent.click(screen.getByRole('button', {name:'Chat options'}));
  expect(screen.getByRole('menuitem', {name:'Split on left'}).getAttribute('aria-disabled')).toBe('true');
  fireEvent.click(screen.getByRole('menuitem', {name:'Overlay'}));
  width=1000;await act(async () => {notify();});
  // Explicitly choosing Overlay remains Overlay; choosing a side restores split.
  expect(surface.dataset.chatPlacement).toBe('overlay');
  choose('Split on right');
  width=390;await act(async () => {notify();});
  width=1000;await act(async () => {notify();});
  expect(surface.dataset.chatPlacement).toBe('right');
  expect(screen.getByLabelText('Message')).toBe(draft);
});
