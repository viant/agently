import {beforeEach, expect, it, vi} from 'vitest';
import fs from 'node:fs';
import {activeWindows, buildUISnapshot, selectedWindowId, selectedTabId, getMetadataSignal, getViewSignal, getInputSignal, getSelectionSignal} from 'forge/core';
import {bindLandingWorkspaceWindows, captureLandingWorkspaceSelection, getScopedActiveSurface, getScopedWorkspaceSelection, MAIN_CHAT_WINDOW_ID, openConversationInMainWindow} from './conversationWindow';
import {buildWorkspaceQueryContext} from './workspaceQueryContext';

beforeEach(() => {
  const values = new Map();
  const storage = {getItem: key => values.get(key) || null, setItem: (key, value) => values.set(key, String(value)), removeItem: key => values.delete(key)};
  global.window = {location: {pathname: '/', hostname: 'localhost'}, history: {replaceState() {}}, localStorage: storage, sessionStorage: storage, dispatchEvent() {}};
  global.CustomEvent = class {constructor(type, options = {}) {this.type = type; this.detail = options.detail;}};
  activeWindows.value = [];
  selectedWindowId.value = selectedTabId.value = null;
});

function landing() {
  const entry = {windowId: 'landing-record', windowKey: 'record', windowTitle: 'Record', parentKey: MAIN_CHAT_WINDOW_ID,
    presentation: 'hosted', region: 'chat.top', parameters: {RecordId: [7]}, resource: {uri: 'window://test/record', revision: 'v1'}, resourceTarget: {windowId: 'landing-record'}};
  activeWindows.value = [{windowId: MAIN_CHAT_WINDOW_ID, windowKey: 'chat/new'}, entry];
  selectedWindowId.value = selectedTabId.value = entry.windowId;
  getMetadataSignal(entry.windowId).value = {dataSource: {records: {uniqueKey: [{field: 'id'}]}}, view: {id: 'recordView', dataSourceRef: 'records'}};
  getViewSignal(entry.windowId).value = {tabs: {primary: 'details', details: 'properties'}};
  getInputSignal(entry.windowId + 'DSrecords').value = {filter: {status: 'active'}};
  getSelectionSignal(entry.windowId + 'DSrecords').value = {selected: {id: 7}};
  return entry;
}

it('keeps the explicitly selected first-message workspace in the actual UI snapshot and query context', () => {
  landing();
  openConversationInMainWindow('baseline-chat');
  const baseline = buildUISnapshot({includeCollection: true});
  expect(baseline.selected.windowId).toBe(MAIN_CHAT_WINDOW_ID);
  const entry = landing();
  const captured = captureLandingWorkspaceSelection();
  // React can bind the live window before the awaiting submit continuation.
  activeWindows.value = bindLandingWorkspaceWindows(activeWindows.peek(), 'new-chat');
  openConversationInMainWindow('new-chat', {landingWorkspaceSelection: captured});
  expect(getScopedActiveSurface('new-chat')).toBe('workspace');
  expect(getScopedWorkspaceSelection('new-chat')).toBe(entry.windowId);
  const snapshot = buildUISnapshot({includeCollection: true});
  expect(snapshot.selected.windowId).toBe(entry.windowId);
  const current = snapshot.windows.find(window => window.windowId === snapshot.selected.windowId);
  expect(current.conversationId).toBe('new-chat');
  expect(current.viewState.tabs).toEqual({primary: 'details', details: 'properties'});
  expect(current.semanticContext).toBeTruthy();
  expect(buildWorkspaceQueryContext('new-chat').workspace).toMatchObject({windowId: entry.windowId, internalTabs: {primary: 'details', details: 'properties'}});
  expect(buildWorkspaceQueryContext('other-chat')).toEqual({});
  expect(activeWindows.peek().find(window => window.windowId === entry.windowId).resourceTarget).toBe(entry.resourceTarget);
  // Explicit contract-proof command can hand these synthetic, actual Forge
  // snapshots to Core's bridge -> registry -> LLM bootstrap regression.
  if (process.env.UI_LANDING_SNAPSHOT_PROOF) {
    fs.writeFileSync(process.env.UI_LANDING_SNAPSHOT_PROOF, JSON.stringify({baseline: {...baseline, ts: 1500}, preserved: {...snapshot, ts: 1500}}, null, 2) + '\n', {mode: 0o600});
  }
});

it('does not invent workspace context for ordinary chat navigation or an explicit return to chat', () => {
  landing();
  const captured = captureLandingWorkspaceSelection();
  selectedWindowId.value = selectedTabId.value = MAIN_CHAT_WINDOW_ID;
  openConversationInMainWindow('new-chat', {landingWorkspaceSelection: captured});
  expect(buildUISnapshot().selected.windowId).toBe(MAIN_CHAT_WINDOW_ID);
  expect(buildWorkspaceQueryContext('new-chat')).toEqual({});
});

it('rejects a closed/replaced/pin-changed landing window and another conversation owner', () => {
  const entry = landing();
  const captured = captureLandingWorkspaceSelection();
  activeWindows.value = activeWindows.peek().map(window => window === entry ? {...entry, resourceTarget: {...entry.resourceTarget}} : window);
  openConversationInMainWindow('new-chat', {landingWorkspaceSelection: captured});
  expect(buildUISnapshot().selected.windowId).toBe(MAIN_CHAT_WINDOW_ID);
  const other = {...entry, conversationId: 'other-chat'};
  activeWindows.value = [{windowId: MAIN_CHAT_WINDOW_ID, windowKey: 'chat/new'}, other];
  selectedWindowId.value = other.windowId;
  expect(captureLandingWorkspaceSelection()).toBeNull();
  openConversationInMainWindow('current-chat', {landingWorkspaceSelection: captured});
  expect(buildWorkspaceQueryContext('current-chat')).toEqual({});
  expect(buildUISnapshot().windows.some(window => window.conversationId === 'other-chat')).toBe(false);
});

it('publishes only fixed diagnostic codes when the private host build explicitly opts in',()=>{
  landing();
  const events=[];window.dispatchEvent=event=>events.push(event);
  try {
    vi.stubEnv('VITE_UI_ADOPTION_DIAGNOSTICS','false');
    captureLandingWorkspaceSelection();
    expect(events).toEqual([]);
    vi.stubEnv('VITE_UI_ADOPTION_DIAGNOSTICS','true');
    captureLandingWorkspaceSelection();
    expect(events.map(event=>({type:event.type,detail:event.detail}))).toEqual([{type:'forge:adoption-diagnostic',detail:{code:'capture.resource_missing'}}]);
  }finally{vi.unstubAllEnvs();}
});
