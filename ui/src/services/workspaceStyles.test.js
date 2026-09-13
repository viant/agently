import {describe, it, expect} from 'vitest';
import fs from 'node:fs';
import {WorkspaceStyleManager, resolveThemeSelection, validateThemeCatalog} from './workspaceStyles';
const catalog = JSON.parse(fs.readFileSync(new URL('../../../../agently-core/protocol/ui/theme/testdata/baseline.json', import.meta.url)));
const revision = 'a'.repeat(64);
const metadata = (rev = revision, id = 'workspace') => ({workspaceId: id,
  uiStyles: {version: 1, revision: rev, themeRevision: rev, href: `/v1/workspace/ui/styles/${rev}.css`},
  uiThemes: {version: 1, revision: rev, href: `/v1/workspace/ui/themes/${rev}.json`},
});
function fixture() {
  const elements = [], stored = new Map();
  const document = {head: {
    appendChild(el) { const old = elements.indexOf(el); if (old >= 0) elements.splice(old, 1); elements.push(el); },
    get lastElementChild() { return elements.at(-1); },
  }, createElement() { const el = {dataset: {}, textContent: '', remove() { const index = elements.indexOf(el); if (index >= 0) elements.splice(index, 1); }}; return el; }};
  const media = {matches: false, listeners: new Set(), addEventListener(_, fn) { this.listeners.add(fn); }, removeEventListener(_, fn) { this.listeners.delete(fn); }};
  const storage = {getItem: key => stored.get(key) || null, setItem: (key, value) => stored.set(key, value), removeItem: key => stored.delete(key)};
  const fetchAsset = async url => ({ok: true, status: 200, text: async () => url.endsWith('.json') ? JSON.stringify(catalog) : '.agently-workspace {color:red}'});
  const manager = new WorkspaceStyleManager({document, storage, matchMedia: () => media, fetchAsset});
  return {manager, elements, stored, media, fetchAsset};
}
describe('portable theme catalog', () => {
  it('accepts the shared native fixture and applies system/explicit modes', () => {
    validateThemeCatalog(catalog);
    expect(resolveThemeSelection(catalog, null, 'dark')).toMatchObject({themeId: 'baseline', mode: 'dark'});
    expect(resolveThemeSelection(catalog, {themeId: 'baseline', modePreference: 'light'}, 'dark').mode).toBe('light');
    const single = structuredClone(catalog); delete single.themes[0].modes.dark;
    expect(resolveThemeSelection(single, null, 'dark').mode).toBe('light');
  });
  it('rejects malformed portable tokens', () => {
    const invalid = structuredClone(catalog); invalid.themes[0].modes.light['control.radius'] = '8px';
    expect(() => validateThemeCatalog(invalid)).toThrow();
  });
});
describe('workspace style lifecycle', () => {
  it('deduplicates, persists selections, observes system mode, and restores ordering', async () => {
    const {manager, elements, media, stored} = fixture();
    await manager.refresh(metadata(), {accountKey: 'user'});
    await manager.refresh(metadata(), {accountKey: 'user'});
    expect(elements).toHaveLength(1);
    media.matches = true; media.listeners.forEach(fn => fn());
    expect(manager.state.mode).toBe('dark');
    manager.setPreference('baseline', 'light');
    media.listeners.forEach(fn => fn()); expect(manager.state.mode).toBe('light');
    expect(stored.size).toBe(1);
    elements.push({}); manager.ensureOrder(); expect(elements.at(-1)).toBe(manager.style);
    manager.dispose(); expect(media.listeners.size).toBe(0); expect(elements).toHaveLength(1);
    await manager.refresh(metadata(), {accountKey: 'user'});
    expect(media.listeners.size).toBe(1); expect(manager.state.mode).toBe('light');
    manager.dispose();
  });
  it('retains a valid revision on failure, but clears styles on workspace changes and deletion', async () => {
    const {manager, elements} = fixture();
    await manager.refresh(metadata(), {accountKey: 'user'});
    manager.fetchAsset = async () => { throw new Error('offline'); };
    await manager.refresh(metadata('b'.repeat(64)), {accountKey: 'user'});
    expect(manager.state.revision).toBe(revision); expect(elements).toHaveLength(1);
    await manager.refresh(metadata('b'.repeat(64), 'other'), {accountKey: 'user'});
    expect(manager.state.revision).toBe(''); expect(elements).toHaveLength(0);
    await manager.refresh({workspaceId: 'other'}, {accountKey: 'user'});
    expect(manager.state.catalog).toBe(null);
    manager.dispose();
  });
  it('ignores stale asynchronous results', async () => {
    const {manager, elements, fetchAsset} = fixture();
    let release;
    manager.fetchAsset = async url => { await new Promise(resolve => { if (url.endsWith('.css')) release = resolve; else resolve(); }); return fetchAsset(url); };
    const pending = manager.refresh(metadata(), {accountKey: 'user'});
    manager.clear(); release(); await pending;
    expect(elements).toHaveLength(0); expect(manager.state.catalog).toBe(null);
    manager.dispose();
  });
  it('keeps a valid parent selection when loading a new catalog revision', async () => {
    const {manager} = fixture();
    await manager.refresh(metadata());
    manager.applyExternalSelection({themeId: 'baseline', modePreference: 'dark', mode: 'dark'});
    await manager.refresh(metadata('b'.repeat(64)));
    expect(manager.state.mode).toBe('dark');
    expect(manager.state.themeId).toBe('baseline');
    manager.dispose();
  });
  it('coalesces an overlapping refresh without losing newer diagnostics', async () => {
    const {manager, fetchAsset} = fixture();
    let release;
    let calls = 0;
    manager.fetchAsset = async url => {
      calls++;
      if (url.endsWith('.css')) await new Promise(resolve => { release = resolve; });
      return fetchAsset(url);
    };
    const first = manager.refresh(metadata());
    const second = manager.refresh({...metadata(), uiStyleDiagnostics: ['Latest edit is invalid']});
    expect(second).toBe(first);
    release(); await first;
    expect(calls).toBe(2);
    expect(manager.state.diagnostics).toEqual(['Latest edit is invalid']);
    manager.dispose();
  });
  it('passes a nonce and retains the previous style when the document rejects a replacement', async () => {
    const {manager, elements} = fixture();
    manager.nonce = 'proof-nonce';
    await manager.refresh(metadata());
    expect(manager.style.nonce).toBe('proof-nonce');
    const previous = manager.style;
    const create = manager.doc.createElement;
    manager.doc.createElement = () => { const element = create(); element.sheet = null; return element; };
    await manager.refresh(metadata('b'.repeat(64)));
    expect(manager.style).toBe(previous);
    expect(manager.state.revision).toBe(revision);
    expect(manager.state.diagnostics.join(' ')).toContain('style policy');
    expect(elements).toEqual([previous]);
    manager.dispose();
  });
  it('rejects remote asset descriptors and mismatched revisions without fetching', async () => {
    const {manager, elements} = fixture(); let calls = 0;
    manager.fetchAsset = async () => { calls++; throw new Error('unexpected'); };
    const bad = metadata(); bad.uiStyles.href = 'https://example.com/styles.css';
    await manager.refresh(bad);
    const mismatch = metadata(); mismatch.uiStyles.themeRevision = 'b'.repeat(64);
    await manager.refresh(mismatch);
    expect(calls).toBe(0); expect(elements).toHaveLength(0); manager.dispose();
  });
});
