// @vitest-environment jsdom
import React, {act} from 'react';
import {createRoot} from 'react-dom/client';
import {afterEach, expect, it, vi} from 'vitest';
import Composer from 'forge/components/chat/Composer.jsx';
import NamedLookupInput from './NamedLookupInput.jsx';
import {flattenStored, missingStarterLookup} from './tokens.js';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;
afterEach(() => {vi.unstubAllGlobals(); sessionStorage.clear();});

const prompt = 'Using Advanced Reporting, show the reports available for /arAdvertiser and help me choose one.';
const starter = {id: 'startertask://measurement/default#choose', fingerprint: 'a'.repeat(64), prompt};
const picker = 'window://measurement/record-picker';
const entry = {
  name: 'arAdvertiser', title: 'Advertiser', required: true, windowId: picker,
  pickerResource: {uri: picker}, providerIdentity: 'studio',
  pickerRoot: {version: '1', contentFingerprint: 'b'.repeat(64)},
  token: {store: '${id}', display: '${name}', modelForm: '${name} (advertiser ID ${id})', queryInput: 'Name'},
};

const pressEnter = node => act(async () => node.dispatchEvent(new KeyboardEvent('keydown', {key: 'Enter', bubbles: true, cancelable: true})));

it('blocks a scoped starter until picker selection, then sends the selected name and ID', async () => {
  let complete;
  let select;
  const fetch = vi.fn(() => new Promise(resolve => {complete = resolve;}));
  vi.stubGlobal('fetch', fetch);
  const openWindow = vi.fn(() => new Promise(resolve => {select = resolve;}));
  const submit = vi.fn();
  const context = {metadata: {resource: {uri: 'window://system/chat/new', providerIdentity: 'local'}}, handlers: {window: {openWindow}}};
  const host = document.createElement('div');
  document.body.append(host);
  const root = createRoot(host);
  const input = () => host.querySelector('[data-testid="chat-composer-input"]');
  const send = () => host.querySelector('[data-testid="chat-composer-send"]');
  try {
    await act(async () => root.render(<Composer appearance="document" draftValue={prompt} onSubmit={submit}
      inputComponent={NamedLookupInput} inputProps={{context, starterLookup: starter}}/>));
    const query = new URL(fetch.mock.calls[0][0], 'http://test').searchParams;
    expect(query.get('owner')).toBe('window://system/chat/new');
    expect(query.get('starterTaskId')).toBe(starter.id);
    expect(query.get('starterTaskFingerprint')).toBe(starter.fingerprint);
    expect(send().disabled).toBe(true);
    await pressEnter(input());
    expect(submit).not.toHaveBeenCalled();

    await act(async () => complete(Response.json({entries: [], starterEntries: [entry]})));
    const chip = host.querySelector('[data-token]');
    expect(chip?.textContent).toBe('Advertiser');
    expect(chip.tabIndex).toBe(0);
    expect(send().disabled).toBe(true);
    await pressEnter(chip);
    expect(host.querySelector('input[data-testid="named-lookup-inline-editor-arAdvertiser"]')).toBeNull();
    await pressEnter(input());
    expect(send().disabled).toBe(true);
    expect(submit).not.toHaveBeenCalled();
    expect(openWindow).toHaveBeenCalledTimes(1);
    expect(openWindow.mock.calls[0][0].execution.args[4]).toMatchObject({providerIdentity: 'studio', resourceRef: {uri: picker}, pickerRoot: entry.pickerRoot});
    await act(async () => select({selected: {id: 42, name: 'Synthetic Advertiser'}}));
    expect(host.querySelector('[data-token]')?.textContent).toBe('Synthetic Advertiser');
    expect(send().disabled).toBe(false);
    await pressEnter(input());
    expect(submit).toHaveBeenCalledTimes(1);
    const {content, lookupSelections} = submit.mock.calls[0][0];
    expect(lookupSelections).toEqual([{token: '@{arAdvertiser:42 "Synthetic Advertiser"}', pickerRoot: entry.pickerRoot}]);
    expect(missingStarterLookup(content + ' @{arAdvertiser:999 "Made Up"}', prompt, [entry], lookupSelections)).toBe('Advertiser');
    expect(missingStarterLookup(content, prompt, [{...entry, pickerRoot: {...entry.pickerRoot, version: '2'}}], lookupSelections)).toBe('Advertiser');
    expect(missingStarterLookup(content, prompt, [{...entry, pickerRoot: {...entry.pickerRoot, contentFingerprint: 'c'.repeat(64)}}], lookupSelections)).toBe('Advertiser');
    expect(flattenStored(content, [entry])).toBe(
      'Using Advanced Reporting, show the reports available for Synthetic Advertiser (advertiser ID 42) and help me choose one.',
    );
  } finally {
    await act(async () => root.unmount());
    host.remove();
  }
});

it('keeps a typed or restored token unverified until the canonical picker returns a selection', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({entries: [], starterEntries: [entry]})));
  const submit = vi.fn();
  const host = document.createElement('div');
  document.body.append(host);
  const root = createRoot(host);
  const forged = prompt.replace('/arAdvertiser', '@{arAdvertiser:999 "Made Up"}');
  try {
    await act(async () => root.render(<Composer appearance="document" draftValue={forged} onSubmit={submit}
      inputComponent={NamedLookupInput} inputProps={{starterLookup: starter}}/>));
    const chip = host.querySelector('[data-token]');
    expect(chip.classList.contains('is-unresolved')).toBe(true);
    expect(chip.textContent).toBe('Advertiser');
    expect(host.querySelector('[data-testid="chat-composer-send"]').disabled).toBe(true);
    await pressEnter(host.querySelector('[data-testid="chat-composer-input"]'));
    expect(submit).not.toHaveBeenCalled();
  } finally {
    await act(async () => root.unmount());
    host.remove();
  }
});

it('fails closed for a scoped starter when the registry is stale or unsupported', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response('', {status: 403})));
  const consoleWarn = vi.spyOn(console, 'warn').mockImplementation(() => {});
  const submit = vi.fn();
  const host = document.createElement('div');
  document.body.append(host);
  const root = createRoot(host);
  try {
    await act(async () => root.render(<Composer appearance="document" draftValue={prompt} onSubmit={submit}
      inputComponent={NamedLookupInput} inputProps={{starterLookup: starter}}/>));
    expect(host.querySelector('[role="status"]').textContent).toContain('lookups are unavailable');
    await pressEnter(host.querySelector('[data-testid="chat-composer-input"]'));
    expect(submit).not.toHaveBeenCalled();
  } finally {
    await act(async () => root.unmount());
    consoleWarn.mockRestore();
    host.remove();
  }
});
