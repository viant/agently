// Shared by the application and standalone window preview. Selection is local;
// CSS/catalog bytes are immutable server-owned snapshots.
const requiredColors = ['surface', 'text', 'control.background', 'control.foreground', 'control.border',
  'focus.color', 'button.background', 'button.foreground', 'disabled.background', 'disabled.foreground', 'validation.border'];
const optionalColors = ['canvas', 'surface.subtle', 'surface.raised', 'text.secondary', 'text.muted', 'text.inverse',
  'border', 'border.strong', 'interaction.foreground', 'interaction.hover', 'interaction.active',
  'interaction.selectedBackground', 'status.info.background', 'status.info.foreground', 'status.info.border',
  'status.success.background', 'status.success.foreground', 'status.success.border',
  'status.warning.background', 'status.warning.foreground', 'status.warning.border',
  'status.danger.background', 'status.danger.foreground', 'status.danger.border',
  ...Array.from({length: 8}, (_, index) => `data.categorical.${index + 1}`),
  ...Array.from({length: 5}, (_, index) => `data.sequential.${index + 1}`)];
const requiredDimensions = {'typography.size': [8, 72], 'control.minHeight': [16, 128], 'control.radius': [0, 64], 'control.paddingInline': [0, 64]};
const typographyRoles = ['caption', 'small', 'body', 'section', 'heading', 'title', 'metric', 'display', 'code', 'label', 'metadata'];
const optionalDimensions = Object.fromEntries(typographyRoles.flatMap(role => [
  [`typography.${role}.size`, [8, 96]],
  [`typography.${role}.lineHeight`, [8, 144]],
  [`typography.${role}.weight`, [100, 900]],
  [`typography.${role}.letterSpacing`, [-0.04, 0.2]],
]));
const foundationDimensions = {"spacing.1": [0, 64], "spacing.2": [0, 64], "spacing.3": [0, 64], "spacing.4": [0, 64], "spacing.5": [0, 64], "spacing.6": [0, 64], "border.width": [0, 4], "radius.surface": [0, 32], "radius.pill": [0, 999], "focus.width": [1, 4], "focus.offset": [0, 8], "motion.fast": [0, 500], "icon.size": [12, 32]};
const dimensions = {...requiredDimensions, ...optionalDimensions, ...foundationDimensions};
const requiredTokenNames = new Set([...requiredColors, ...Object.keys(requiredDimensions), 'typography.family']);
const roleFamilies = typographyRoles.map(role => `typography.${role}.family`);
const tokenNames = new Set([...requiredTokenNames, ...optionalColors, ...Object.keys(optionalDimensions), ...roleFamilies, ...Object.keys(foundationDimensions)]);
const fontFamilies = new Set(['system', 'workspace-primary', 'workspace-secondary', 'monospace']);
const identifier = /^[a-z][a-z0-9-]{0,63}$/;
export const APPLICATION_THEME_CLASS = 'agently-application';
const APPLICATION_THEME_ATTRIBUTE = 'data-agently-theme';
const APPLICATION_MODE_ATTRIBUTE = 'data-agently-color-mode';

export function applyApplicationThemeBoundary(element, state = {}) {
  if (!element) return;
  element.classList?.add(APPLICATION_THEME_CLASS);
  const themeId = String(state.themeId || '').trim();
  if (!themeId) {
    element.removeAttribute?.(APPLICATION_THEME_ATTRIBUTE);
    element.removeAttribute?.(APPLICATION_MODE_ATTRIBUTE);
    return;
  }
  element.setAttribute?.(APPLICATION_THEME_ATTRIBUTE, themeId);
  element.setAttribute?.(APPLICATION_MODE_ATTRIBUTE, state.mode === 'dark' ? 'dark' : 'light');
}

export function clearApplicationThemeBoundary(element) {
  if (!element) return;
  element.classList?.remove(APPLICATION_THEME_CLASS);
  element.removeAttribute?.(APPLICATION_THEME_ATTRIBUTE);
  element.removeAttribute?.(APPLICATION_MODE_ATTRIBUTE);
}

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
      if (!['light', 'dark'].includes(mode) || !tokens) throw new Error('Invalid theme mode');
      const keys = Object.keys(tokens);
      if (![...requiredTokenNames].every(key => Object.hasOwn(tokens, key)) || keys.some(key => !tokenNames.has(key))) throw new Error('Invalid theme mode');
      for (const [key, value] of Object.entries(tokens)) {
        if (!tokenNames.has(key)) throw new Error(`Unknown theme token: ${key}`);
        if (key === 'typography.family' || roleFamilies.includes(key)) { if (!fontFamilies.has(value)) throw new Error('Invalid theme font'); }
        else if (dimensions[key]) {
          const [min, max] = dimensions[key];
          if (typeof value !== 'number' || !Number.isFinite(value) || value < min || value > max || (key.endsWith('.weight') && !Number.isInteger(value))) throw new Error(`Invalid theme dimension: ${key}`);
        } else if (typeof value !== 'string' || !/^#[\da-f]{6}([\da-f]{2})?$/i.test(value)) throw new Error(`Invalid theme color: ${key}`);
      }
    }
  }
  for (const theme of catalog.themes) {
    if (theme.presentations == null) continue;
    if (typeof theme.presentations !== 'object' || Array.isArray(theme.presentations)
      || Object.entries(theme.presentations).some(([appearance, profile]) => appearance !== 'document' || !ids.has(profile))) throw new Error('Invalid presentation profile');
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
    this.applicationStyles = new Map(); this.applicationStyleGeneration = 0; this.applicationStyleKey = ''; this.applicationStyleAccount = '';
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
  clear({preserveApplicationStyles = false} = {}) {
    if (!preserveApplicationStyles) this.clearApplicationStyles();
    this.sequence++; this.style?.remove(); this.style = null;
    this.identity = ''; this.workspaceId = ''; this.externalSelection = null; this.storageKey = ''; this.preference = null; this.pending = null;
    this.publish({catalog: null, revision: '', themeId: '', mode: 'light', modePreference: 'system', diagnostics: []});
  }
  clearApplicationStyles() {
    this.applicationStyleGeneration++;
    for (const style of this.applicationStyles.values()) style.remove();
    this.applicationStyles.clear(); this.applicationStyleKey = ''; this.applicationStyleAccount = ''; this.applicationStylePending = null;
  }
  loadApplicationStyles(applications, {accountKey = '', readResource, force = false} = {}) {
    const entries = applications.flatMap(app => (app.styleResources || []).map(uri => ({uri, server: app.resourceConnection, providerIdentity: app.providerIdentity, namespace: app.resourceNamespace})))
      .filter(entry => entry.server && entry.providerIdentity && entry.namespace);
    const key = JSON.stringify([accountKey, entries]);
    if (!force && key === this.applicationStyleKey) return this.applicationStylePending || Promise.resolve();
    if (accountKey !== this.applicationStyleAccount) this.clearApplicationStyles();
    this.applicationStyleAccount = accountKey; this.applicationStyleKey = key;
    const generation = ++this.applicationStyleGeneration;
    const wanted = new Set(entries.map(entry => JSON.stringify(entry)));
    for (const [id, style] of this.applicationStyles) if (!wanted.has(id)) { style.remove(); this.applicationStyles.delete(id); }
    this.applicationStylePending = Promise.all(entries.map(async entry => {
      const id = JSON.stringify(entry);
      try {
        if (!entry.uri.startsWith('style://') || typeof readResource !== 'function') throw new Error('Invalid application style reference');
        const response = await readResource(entry);
        const result = response?.result || response;
        if (result?.contents?.length !== 1 || result.contents[0].uri !== entry.uri || typeof result.contents[0].text !== 'string') throw new Error('Invalid application style response');
        const definition = JSON.parse(result.contents[0].text);
        const {compileResourceCSS} = await import('forge/resources/scopedStyles');
        const css = await compileResourceCSS(definition, entry);
        if (generation !== this.applicationStyleGeneration) return;
        const style = this.doc.createElement('style');
        style.dataset.applicationResourceStyle = entry.uri;
        if (this.nonce) style.nonce = this.nonce;
        style.textContent = css; this.doc.head.appendChild(style);
        if ('sheet' in style && (!style.sheet || !style.sheet.cssRules.length)) { style.remove(); throw new Error('Scoped CSS was rejected by this browser'); }
        const previous = this.applicationStyles.get(id);
        this.applicationStyles.set(id, style); previous?.remove();
      } catch (error) {
        if (generation === this.applicationStyleGeneration) this.publish({...this.state, diagnostics: [...this.state.diagnostics, `${entry.uri}: ${error.message}`]});
      }
    }));
    return this.applicationStylePending;
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
      this.clear({preserveApplicationStyles: !this.identity}); this.identity = identity;
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
