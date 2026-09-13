// Shared by the application and standalone window preview. Selection is local;
// CSS/catalog bytes are immutable server-owned snapshots.
const colors = ['surface', 'text', 'control.background', 'control.foreground', 'control.border',
  'focus.color', 'button.background', 'button.foreground', 'disabled.background', 'disabled.foreground', 'validation.border'];
const dimensions = {'typography.size': [8, 72], 'control.minHeight': [16, 128], 'control.radius': [0, 64], 'control.paddingInline': [0, 64]};
const tokenNames = new Set([...colors, ...Object.keys(dimensions), 'typography.family']);
const identifier = /^[a-z][a-z0-9-]{0,63}$/;

export function validateThemeCatalog(catalog) {
  if (!catalog || catalog.version !== 1 || catalog.paletteVersion !== 1 ||
      !['light', 'dark', 'system'].includes(catalog.defaultMode) || !Array.isArray(catalog.themes) ||
      !catalog.themes.length || catalog.themes.length > 32) throw new Error('Unsupported theme catalog');
  const ids = new Set();
  for (const theme of catalog.themes) {
    if (!identifier.test(theme.id) || theme.id === 'forge-default' || ids.has(theme.id) ||
        typeof theme.label !== 'string' || !theme.label.trim() || new TextEncoder().encode(theme.label).length > 256 ||
        !theme.modes || !Object.hasOwn(theme.modes, theme.fallbackMode)) throw new Error('Invalid theme definition');
    ids.add(theme.id);
    for (const [mode, tokens] of Object.entries(theme.modes)) {
      if (!['light', 'dark'].includes(mode) || !tokens || Object.keys(tokens).length !== tokenNames.size) throw new Error('Invalid theme mode');
      for (const [key, value] of Object.entries(tokens)) {
        if (!tokenNames.has(key)) throw new Error(`Unknown theme token: ${key}`);
        if (key === 'typography.family') { if (value !== 'system') throw new Error('Invalid theme font'); }
        else if (dimensions[key]) {
          const [min, max] = dimensions[key];
          if (typeof value !== 'number' || !Number.isFinite(value) || value < min || value > max) throw new Error(`Invalid theme dimension: ${key}`);
        } else if (typeof value !== 'string' || !/^#[\da-f]{6}([\da-f]{2})?$/i.test(value)) throw new Error(`Invalid theme color: ${key}`);
      }
    }
  }
  if (!ids.has(catalog.defaultTheme)) throw new Error('Missing default theme');
  return catalog;
}

export function resolveThemeSelection(catalog, preference, systemMode = 'light') {
  if (!catalog || preference?.themeId === 'forge-default') return {themeId: '', mode: 'light', modePreference: 'system'};
  const saved = catalog.themes.find(theme => theme.id === preference?.themeId);
  const theme = saved || catalog.themes.find(theme => theme.id === catalog.defaultTheme);
  const modePreference = saved && ['light', 'dark', 'system'].includes(preference?.modePreference) ? preference.modePreference : catalog.defaultMode;
  const desired = modePreference === 'system' ? systemMode : modePreference;
  return {themeId: theme.id, mode: Object.hasOwn(theme.modes, desired) ? desired : theme.fallbackMode, modePreference};
}
function descriptorURL(descriptor, kind) {
  const extension = kind === 'styles' ? 'css' : 'json';
  if (descriptor?.version !== 1 || !/^[a-f0-9]{64}$/.test(descriptor.revision) ||
      descriptor.href !== `/v1/workspace/ui/${kind}/${descriptor.revision}.${extension}`) throw new Error('Invalid workspace asset descriptor');
  return descriptor.href;
}

const documentManagers = new WeakMap();
export function getWorkspaceStyleManager(options = {}) {
  const doc = options.document || globalThis.document;
  if (!doc) return new WorkspaceStyleManager(options);
  if (!documentManagers.has(doc)) documentManagers.set(doc, new WorkspaceStyleManager({...options, document: doc}));
  return documentManagers.get(doc);
}

export class WorkspaceStyleManager {
  constructor({document: doc = globalThis.document, fetchAsset = (...args) => fetch(...args), storage,
    matchMedia = globalThis.matchMedia?.bind(globalThis), nonce = ''} = {}) {
    this.doc = doc; this.fetchAsset = fetchAsset; this.nonce = nonce;
    try { this.storage = storage === undefined ? globalThis.localStorage : storage; } catch { this.storage = null; }
    this.listeners = new Set(); this.references = 0; this.sequence = 0; this.identity = ''; this.storageKey = ''; this.preference = null;
    this.state = {catalog: null, revision: '', themeId: '', mode: 'light', modePreference: 'system', diagnostics: []};
    this.matchMedia = matchMedia; this.media = null; this.connected = false;
    this.onSystemChange = () => this.publish({...this.state, ...(this.externalSelection || resolveThemeSelection(this.state.catalog, this.preference, this.systemMode()))});
    this.onStorage = event => { if (this.storageKey && event.key === this.storageKey) { this.preference = this.readPreference(); this.onSystemChange(); } };
    this.observer = null;
  }
  retain() { this.references++; this.connect(); }
  release() { if (this.references > 0 && --this.references === 0) this.dispose(); }
  connect() {
    if (this.connected) return;
    this.connected = true;
    this.media = this.matchMedia?.('(prefers-color-scheme: dark)');
    this.media?.addEventListener?.('change', this.onSystemChange);
    this.doc?.defaultView?.addEventListener?.('storage', this.onStorage);
    const Observer = this.doc?.defaultView?.MutationObserver;
    this.observer = Observer ? new Observer(() => this.ensureOrder()) : null;
    if (this.doc?.head) this.observer?.observe(this.doc.head, {childList: true});
  }
  systemMode() { return this.media?.matches ? 'dark' : 'light'; }
  subscribe = listener => { this.listeners.add(listener); return () => this.listeners.delete(listener); };
  getSnapshot = () => this.state;
  publish(state) { this.state = state; this.listeners.forEach(listener => listener()); }
  ensureOrder() { if (this.style && this.doc.head.lastElementChild !== this.style) this.doc.head.appendChild(this.style); }
  readPreference() { try { return this.storageKey ? JSON.parse(this.storage?.getItem(this.storageKey) || 'null') : null; } catch { return null; } }
  applyExternalSelection(selection) {
    this.externalSelection = {themeId: selection.themeId === 'forge-default' ? '' : selection.themeId,
      modePreference: selection.modePreference, mode: selection.mode};
    this.publish({...this.state, ...this.externalSelection});
  }
  setPreference(themeId, modePreference = 'system') {
    if (themeId !== 'forge-default' && !this.state.catalog?.themes.some(theme => theme.id === themeId)) return;
    if (!['light', 'dark', 'system'].includes(modePreference)) return;
    this.externalSelection = null;
    this.preference = {themeId, modePreference};
    try { if (this.storageKey) this.storage?.setItem(this.storageKey, JSON.stringify(this.preference)); } catch { /* session selection still works */ }
    this.onSystemChange();
  }
  clear() {
    this.sequence++; this.style?.remove(); this.style = null;
    this.identity = ''; this.workspaceId = ''; this.externalSelection = null; this.storageKey = ''; this.preference = null; this.pending = null;
    this.publish({catalog: null, revision: '', themeId: '', mode: 'light', modePreference: 'system', diagnostics: []});
  }
  refresh(metadata, options = {}) {
    this.latestDiagnostics = Array.isArray(metadata?.uiStyleDiagnostics) ? metadata.uiStyleDiagnostics.filter(value => typeof value === 'string') : [];
    const key = JSON.stringify([options.accountKey, options.workspaceKey, metadata?.workspaceId, metadata?.workspaceRoot, metadata?.uiStyles, metadata?.uiThemes]);
    if (this.pending?.key === key) {
      if (this.state.revision === metadata?.uiStyles?.revision) this.publish({...this.state, diagnostics: this.latestDiagnostics});
      return this.pending.promise;
    }
    const promise = this.load(metadata, options);
    this.pending = {key, promise};
    promise.finally(() => { if (this.pending?.promise === promise) this.pending = null; });
    return promise;
  }
  async load(metadata, {accountKey = '', workspaceKey = ''} = {}) {
    this.connect();
    const identity = JSON.stringify([accountKey, metadata?.workspaceId || workspaceKey || metadata?.workspaceRoot || 'session']);
    if (identity !== this.identity) {
      this.clear(); this.identity = identity;
      this.storageKey = accountKey && metadata?.workspaceId ? `agently.theme.v1:${JSON.stringify([accountKey, metadata.workspaceId])}` : '';
      this.preference = this.readPreference();
    }
    this.workspaceId = metadata?.workspaceId || '';
    const request = ++this.sequence;
    const diagnostics = metadata?.uiStyleDiagnostics || [];
    if (!metadata?.uiStyles) {
      this.style?.remove(); this.style = null;
      this.preference = null;
      try { if (this.storageKey) this.storage?.removeItem(this.storageKey); } catch {}
      this.publish({catalog: null, revision: '', themeId: '', mode: 'light', modePreference: 'system', diagnostics});
      return;
    }
    try {
      const styleURL = descriptorURL(metadata.uiStyles, 'styles');
      const catalogURL = metadata.uiThemes ? descriptorURL(metadata.uiThemes, 'themes') : null;
      if (catalogURL && metadata.uiStyles.themeRevision !== metadata.uiThemes.revision) throw new Error('Theme catalog and CSS revisions differ');
      if (!catalogURL && metadata.uiStyles.themeRevision) throw new Error('Theme catalog is missing');
      if (metadata.uiStyles.revision === this.state.revision) { this.publish({...this.state, diagnostics}); return; }
      const read = async (url, json) => {
        const response = await this.fetchAsset(url, {credentials: 'include', headers: {Accept: json ? 'application/json' : 'text/css'}});
        if (!response.ok) throw new Error(`Workspace style fetch failed (${response.status})`);
        const contentType = response.headers?.get?.('content-type') || '';
        if (contentType && !contentType.startsWith(json ? 'application/json' : 'text/css')) throw new Error('Unexpected workspace asset type');
        const text = await response.text();
        if (new TextEncoder().encode(text).length > 512 * 1024) throw new Error('Workspace style asset is too large');
        return json ? validateThemeCatalog(JSON.parse(text)) : text;
      };
      const [css, catalog] = await Promise.all([read(styleURL, false), catalogURL ? read(catalogURL, true) : null]);
      if (request !== this.sequence) return;
      const style = this.doc.createElement('style');
      style.dataset.workspaceStyle = metadata.uiStyles.revision;
      if (this.nonce) style.nonce = this.nonce;
      style.textContent = css;
      this.doc.head.appendChild(style);
      // Browsers leave sheet null if CSP rejects this style element.
      if ('sheet' in style && !style.sheet) { style.remove(); throw new Error('Workspace CSS was rejected by the document style policy'); }
      const previous = this.style; this.style = style; previous?.remove();
      const external = this.externalSelection;
      const externalTheme = catalog?.themes.find(theme => theme.id === external?.themeId);
      this.externalSelection = external && (!external.themeId || externalTheme?.modes?.[external.mode]) ? external : null;
      if (this.preference?.themeId !== 'forge-default' && !catalog?.themes.some(theme => theme.id === this.preference?.themeId)) {
        this.preference = null;
        try { if (this.storageKey) this.storage?.removeItem(this.storageKey); } catch {}
      }
      this.publish({catalog, revision: metadata.uiStyles.revision, diagnostics: this.latestDiagnostics,
        ...resolveThemeSelection(catalog, this.preference, this.systemMode()), ...(this.externalSelection || {})});
    } catch (error) {
      if (request === this.sequence) this.publish({...this.state, diagnostics: [...this.latestDiagnostics, error.message]});
    }
  }
  dispose() {
    this.observer?.disconnect(); this.media?.removeEventListener?.('change', this.onSystemChange);
    this.doc?.defaultView?.removeEventListener?.('storage', this.onStorage); this.connected = false; this.clear();
  }
}
