import {describe, it, expect} from 'vitest';
import fs from 'node:fs';
import path from 'node:path';
import {pathToFileURL} from 'node:url';
import {applyApplicationThemeBoundary, clearApplicationThemeBoundary, WorkspaceStyleManager, resolveThemeSelection, validateThemeCatalog} from './workspaceStyles';
const baselineURL = process.env.AGENTLY_CORE_SOURCE_ROOT
  ? pathToFileURL(path.resolve(process.env.AGENTLY_CORE_SOURCE_ROOT, 'protocol/ui/theme/testdata/baseline.json'))
  : new URL('../../../../agently-core/protocol/ui/theme/testdata/baseline.json', import.meta.url);
const catalog = JSON.parse(fs.readFileSync(baselineURL));
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
    get children() { return elements; },
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
  it('accepts a workspace-owned primary family without exposing a vendor font', () => {
    const workspace = structuredClone(catalog);
    workspace.themes[0].modes.light['typography.family'] = 'workspace-primary';
    expect(validateThemeCatalog(workspace)).toBe(workspace);
    workspace.themes[0].modes.light['typography.family'] = 'ibm-plex-sans';
    expect(() => validateThemeCatalog(workspace)).toThrow('Invalid theme font');
  });
  it('accepts a third registered family for a separate design system', () => {
    const workspace = structuredClone(catalog);
    workspace.themes[0].modes.light['typography.family'] = 'workspace-tertiary';
    expect(validateThemeCatalog(workspace)).toBe(workspace);
  });
  it('accepts validated optional semantic colors without requiring them', () => {
    const workspace = structuredClone(catalog);
    workspace.themes[0].modes.light['text.secondary'] = '#3a4460';
    workspace.themes[0].modes.light['interaction.foreground'] = '#1a3a8f';
    workspace.themes[0].modes.light['status.danger.foreground'] = '#ae0020';
    workspace.themes[0].modes.light['data.categorical.1'] = '#1a3a8f';
    expect(validateThemeCatalog(workspace)).toBe(workspace);
    workspace.themes[0].modes.light['status.danger.foreground'] = 'red';
    expect(() => validateThemeCatalog(workspace)).toThrow('Invalid theme color');
  });
  it('accepts optional semantic typography roles without requiring them', () => {
    const workspace = structuredClone(catalog);
    workspace.themes[0].modes.light['typography.caption.size'] = 11;
    workspace.themes[0].modes.light['typography.caption.lineHeight'] = 16;
    workspace.themes[0].modes.light['typography.metric.size'] = 28;
    expect(validateThemeCatalog(workspace)).toBe(workspace);
    workspace.themes[0].modes.light['typography.caption.lineHeight'] = 200;
    expect(() => validateThemeCatalog(workspace)).toThrow('Invalid theme dimension');
  });
  it('rejects malformed portable tokens', () => {
    const invalid = structuredClone(catalog); invalid.themes[0].modes.light['control.radius'] = '8px';
    expect(() => validateThemeCatalog(invalid)).toThrow();
  });
});
describe('application theme boundary', () => {
  it('exposes the active workspace selection to the full application and cleans it up', () => {
    const classes = new Set();
    const attributes = new Map();
    const element = {
      classList: {add: value => classes.add(value), remove: value => classes.delete(value)},
      setAttribute: (key, value) => attributes.set(key, value),
      removeAttribute: key => attributes.delete(key),
    };
    applyApplicationThemeBoundary(element, {themeId: 'baseline', mode: 'dark'});
    expect(classes.has('agently-application')).toBe(true);
    expect(attributes.get('data-agently-theme')).toBe('baseline');
    expect(attributes.get('data-agently-color-mode')).toBe('dark');
    applyApplicationThemeBoundary(element, {themeId: '', mode: 'light'});
    expect(classes.has('agently-application')).toBe(true);
    expect(attributes.size).toBe(0);
    clearApplicationThemeBoundary(element);
    expect(classes.size).toBe(0);
  });
});
describe('workspace style lifecycle', () => {
  it('loads the complete Steward stylesheet and rejects an oversized replacement', async () => {
    const {manager, elements} = fixture();
    let css = 'a'.repeat(600 * 1024);
    manager.fetchAsset = async url => ({ok: true, status: 200, text: async () => url.endsWith('.json') ? JSON.stringify(catalog) : css});
    await manager.refresh(metadata());
    expect(manager.state.revision).toBe(revision);
    expect(elements[0].textContent).toHaveLength(600 * 1024);

    css = 'a'.repeat(769 * 1024);
    await manager.refresh(metadata('b'.repeat(64)));
    expect(manager.state.revision).toBe(revision);
    expect(elements[0].textContent).toHaveLength(600 * 1024);
    expect(manager.state.diagnostics.join(' ')).toContain('too large');
    manager.dispose();
  });
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

describe('one managed global stylesheet', () => {
  const source = (styleRevision='1') => ({resourceUri:'style://shared/base',resourceConnection:'public-catalog',providerIdentity:'studio',styleRevision});
  const namespaceStyles = [{allowStyleOverrides:true,providerIdentity:'studio',resourceNamespace:'one',resourceConnection:'private-catalog',styleResources:['style://one/workspace']}];
  const response = (uri,css='.shared{color:red}',extra={}) => ({contents:[{uri,text:JSON.stringify({schemaVersion:1,css,tokens:{'--edge':'red'},...extra})}]});
  it('ignores all namespace sheets and reads only the configured global base',async()=>{
    const {manager,elements}=fixture();const reads=[];const readResource=async({uri})=>{reads.push(uri);return response(uri);};
    await manager.loadApplicationStyles(namespaceStyles,{accountKey:'alice',styleSource:'managed',globalStyle:source(),readResource});
    expect(reads).toEqual(['style://shared/base']);expect(elements).toHaveLength(1);expect(elements[0].textContent).not.toContain('@scope');
    await manager.loadApplicationStyles(namespaceStyles,{accountKey:'alice',styleSource:'managed',globalStyle:source(),readResource});expect(reads).toHaveLength(1);
    await manager.loadApplicationStyles(namespaceStyles,{accountKey:'alice',styleSource:'managed',readResource});expect(elements).toHaveLength(0);expect(reads).toHaveLength(1);
    expect(manager.getSnapshot().diagnostics).toContain('Managed global style is unavailable.');manager.dispose();
  });
  it('uses local workspace CSS only in explicit local mode with no remote fallback',async()=>{
    const {manager,elements}=fixture();await manager.refresh({...metadata(),uiStyleSource:'local'},{accountKey:'alice'});
    const local=elements[0];let reads=0;
    await manager.loadApplicationStyles(namespaceStyles,{accountKey:'alice',styleSource:'local',globalStyle:source(),readResource:async()=>{reads++;throw new Error('must not read remote');}});
    expect(reads).toBe(0);expect(elements).toEqual([local]);manager.dispose();
  });
  it('refreshes a changed global revision and retains last admitted CSS only on transient failure',async()=>{
    const {manager,elements}=fixture();let reads=0;
    const readResource=async({uri,revision})=>{reads++;if(revision==='unavailable')throw new Error('Dependency unavailable');if(revision==='denied')throw Object.assign(new Error('Withdrawn'),{status:403});return response(uri,`.shared{color:${revision==='2'?'blue':'red'}}`);};
    await manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed',globalStyle:source(),readResource});
    await manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed',globalStyle:source('2'),readResource});expect(elements).toHaveLength(1);expect(elements[0].textContent).toContain('color:blue');
    await manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed',globalStyle:source('unavailable'),readResource});expect(elements[0].textContent).toContain('color:blue');
    await manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed',globalStyle:source('denied'),readResource});expect(elements).toHaveLength(0);expect(reads).toBe(4);manager.dispose();
  });
  it('does not install a pending sheet after withdrawal or logout',async()=>{
    const {manager,elements}=fixture();let release;
    const pending=manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed',globalStyle:source(),readResource:({uri})=>new Promise(resolve=>{release=()=>resolve(response(uri));})});
    await manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed'});release();await pending;expect(elements).toHaveLength(0);
    const next=manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed',globalStyle:source('2'),readResource:({uri})=>new Promise(resolve=>{release=()=>resolve(response(uri));})});
    manager.clear();release();await next;expect(elements).toHaveLength(0);manager.dispose();
  });
  it('retains embedded fonts on warm reads and cleans up only owned faces',async()=>{
    const {manager,elements}=fixture();const unrelated={};manager.doc.fonts=new Set([unrelated]);const prior=globalThis.FontFace;let installs=0;
    globalThis.FontFace=class{constructor(family){this.family=family;installs++;}load(){return Promise.resolve(this);}};
    try{
      const readResource=async({uri})=>response(uri,'.text{font-family:Sample}',{fonts:[{family:'Sample',format:'woff2',data:btoa('wOF2synthetic'),weight:400,style:'normal'}]});
      await manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed',globalStyle:source(),readResource});expect(installs).toBe(1);expect(elements[0].textContent).toContain([...manager.doc.fonts][1].family);
      await manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed',globalStyle:source(),readResource});expect(installs).toBe(1);
      await manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed',globalStyle:source('2'),readResource});expect(installs).toBe(2);expect(manager.doc.fonts.size).toBe(2);
      manager.clearApplicationStyles();expect([...manager.doc.fonts]).toEqual([unrelated]);
    }finally{globalThis.FontFace=prior;manager.dispose();}
  });
  it('never loads local CSS or catalog in Studio mode, including before the base arrives',async()=>{
    const {manager,elements}=fixture();let localReads=0;manager.fetchAsset=async()=>{localReads++;throw new Error('Local asset forbidden');};
    await manager.refresh({...metadata(),uiStyleSource:'managed'},{accountKey:'alice',workspaceKey:'root'});expect(localReads).toBe(0);expect(elements).toHaveLength(0);expect(manager.getSnapshot().catalog).toBeNull();
    const remote=structuredClone(catalog);remote.defaultTheme='studio';remote.themes[0].id='studio';
    await manager.loadApplicationStyles(namespaceStyles,{accountKey:'alice',styleSource:'managed',globalStyle:source(),readResource:async({uri})=>response(uri,'.shared{color:red}',{themeCatalog:remote})});
    expect(manager.getSnapshot().themeId).toBe('studio');expect(elements).toHaveLength(1);
    await manager.loadApplicationStyles(namespaceStyles,{accountKey:'alice',styleSource:'managed'});expect(elements).toHaveLength(0);expect(manager.getSnapshot().catalog).toBeNull();expect(manager.getSnapshot().themeId).toBe('');expect(localReads).toBe(0);manager.dispose();
  });
  it.each(['frame-first','layout-first'])('deduplicates %s startup and ignores pending null metadata',async order=>{
    const {manager,elements}=fixture();let reads=0,localReads=0;
    manager.fetchAsset=async()=>{localReads++;throw new Error('Local asset forbidden');};
    const options={accountKey:'alice',styleSource:'managed',globalStyle:source(),readResource:async({uri})=>{reads++;return response(uri,'.shared{color:red}',{themeCatalog:catalog});}};
    const frame=async()=>{await manager.refresh({uiStyleSource:'managed',workspaceRoot:'root',globalStyle:source()},{accountKey:'alice',workspaceKey:'endpoint'});await manager.loadApplicationStyles([],options);};
    if(order==='layout-first'){await manager.loadApplicationStyles([],options);await manager.refresh(null,{workspaceKey:'endpoint'});await frame();}
    else{await frame();await manager.loadApplicationStyles([],options);}
    expect(reads).toBe(1);expect(localReads).toBe(0);expect(elements).toHaveLength(1);expect(manager.getSnapshot().revision).toBe(source().styleRevision);
    await manager.refresh(null,{workspaceKey:'endpoint'});expect(elements).toHaveLength(1);manager.dispose();
  });
  it('a pending local load cannot flash after switching to Studio ownership',async()=>{
    const {manager,elements}=fixture();let release;
    manager.fetchAsset=url=>url.endsWith('.json')?Promise.resolve({ok:true,text:async()=>JSON.stringify(catalog)}):new Promise(resolve=>{release=()=>resolve({ok:true,text:async()=>'.old-local{color:red}'});});
    const pending=manager.refresh({...metadata(),uiStyleSource:'local'},{accountKey:'alice'});
    await manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed',globalStyle:source(),readResource:async({uri})=>response(uri)});release();await pending;
    expect(elements).toHaveLength(1);expect(elements[0].dataset.applicationResourceStyle).toBe(source().resourceUri);expect(elements.some(sheet=>sheet.textContent.includes('old-local'))).toBe(false);manager.dispose();
  });
  it('allows explicit retry without installing invalid CSS or crossing account ownership',async()=>{
    const {manager,elements}=fixture();let reads=0;const readResource=async({uri})=>{reads++;return response(uri);};
    await manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed',globalStyle:source(),readResource});
    await manager.loadApplicationStyles([],{accountKey:'alice',styleSource:'managed',globalStyle:source(),readResource,force:true});expect(reads).toBe(2);expect(elements).toHaveLength(1);
    await manager.loadApplicationStyles([],{accountKey:'bob',styleSource:'managed',globalStyle:source(),readResource:async({uri})=>response(uri,'@import "foreign.css";')});expect(elements).toHaveLength(0);expect(manager.getSnapshot().diagnostics.some(message=>message.includes('unsupported global'))).toBe(true);manager.dispose();
  });
});

describe('complete optional typography contract', () => {
  it('validates every role property and preserves legacy catalogs', () => {
    const next = structuredClone(catalog);
    for (const role of ['caption', 'small', 'body', 'section', 'heading', 'title', 'metric', 'display', 'code', 'label', 'metadata']) {
      Object.assign(next.themes[0].modes.light, {
        [`typography.${role}.family`]: role === 'code' ? 'monospace' : 'workspace-primary',
        [`typography.${role}.size`]: 14, [`typography.${role}.weight`]: 600,
        [`typography.${role}.lineHeight`]: 20, [`typography.${role}.letterSpacing`]: -0.01,
      });
      expect(validateThemeCatalog(next)).toBe(next);
      for (const [key, value] of [['weight', 600.5], ['weight', 901], ['family', 'url(evil)'], ['letterSpacing', -0.05]]) {
        const bad = structuredClone(next);
        bad.themes[0].modes.light[`typography.${role}.${key}`] = value;
        expect(() => validateThemeCatalog(bad)).toThrow();
      }
    }
    const legacy = structuredClone(catalog);
    for (const key of Object.keys(legacy.themes[0].modes.light)) if (key.startsWith('typography.') && key.split('.').length > 2) delete legacy.themes[0].modes.light[key];
    expect(validateThemeCatalog(legacy)).toBe(legacy);
  });
});

it('accepts bounded presentation references without changing selection and rejects invalid profiles',()=>{
 const scoped=structuredClone(catalog);scoped.themes[0].presentations={document:scoped.themes[0].id};
 expect(validateThemeCatalog(scoped)).toBe(scoped);expect(resolveThemeSelection(scoped,null,'light').themeId).toBe(catalog.defaultTheme);
 for(const presentations of [{document:'missing'},{other:scoped.themes[0].id},[], 'document']) {
  const invalid=structuredClone(scoped);invalid.themes[0].presentations=presentations;
  expect(()=>validateThemeCatalog(invalid)).toThrow('Invalid presentation profile');
 }
});
