// Synchronize only same-origin Forge renderer frames. Messages carry selections,
// never CSS, asset URLs, or arbitrary workspace paths.
const MESSAGE = 'agently.workspace-theme';
const READY = 'agently.workspace-theme-ready';
const rendererPaths = new Set(['/mcp-ui/forge-window', '/ui/mcp-ui/forge-window']);

export function acceptsThemeSelection(payload, manager) {
  if (payload?.type !== MESSAGE || payload.workspaceId !== manager.workspaceId || payload.revision !== manager.state.revision ||
      !['light', 'dark', 'system'].includes(payload.modePreference) || !['light', 'dark'].includes(payload.mode)) return false;
  if (payload.themeId === 'forge-default') return payload.mode === 'light';
  const theme = manager.state.catalog?.themes.find(entry => entry.id === payload.themeId);
  return !!theme && Object.hasOwn(theme.modes, payload.mode);
}

export function installWorkspaceThemeBridge(manager, refresh, target = window) {
  const origin = target.location.origin;
  const isGuest = target.parent !== target && rendererPaths.has(target.location.pathname);
  const validFrame = frame => {
    try { const url = new URL(frame.src, target.location.href); return url.origin === origin && rendererPaths.has(url.pathname); } catch { return false; }
  };
  const frames = () => [...target.document.querySelectorAll('iframe[src]')].filter(validFrame);
  const selection = () => ({type: MESSAGE, workspaceId: manager.workspaceId, revision: manager.state.revision,
    themeId: manager.state.themeId || 'forge-default', modePreference: manager.state.modePreference, mode: manager.state.mode});
  let lastGuestKey = '';
  const publish = () => {
    if (isGuest) {
      const key = JSON.stringify([manager.workspaceId, manager.state.revision]);
      if (key === lastGuestKey) return;
      lastGuestKey = key;
      target.parent.postMessage({type: READY, workspaceId: manager.workspaceId, revision: manager.state.revision}, origin);
    } else frames().forEach(frame => frame.contentWindow?.postMessage(selection(), origin));
  };
  const onMessage = event => {
    if (event.origin !== origin) return;
    if (isGuest) {
      if (event.source !== target.parent || event.data?.type !== MESSAGE) return;
      // A trusted parent may have switched workspaces. Re-read our own metadata;
      // never apply the other identity's selection without matching it locally.
      if (event.data.workspaceId !== manager.workspaceId || event.data.revision !== manager.state.revision) { refresh?.(); return; }
      if (acceptsThemeSelection(event.data, manager)) manager.applyExternalSelection(event.data);
    } else if (event.data?.type === READY) {
      const frame = frames().find(entry => entry.contentWindow === event.source);
      if (!frame || event.data.workspaceId !== manager.workspaceId) return;
      frame.contentWindow.postMessage(selection(), origin);
    }
  };
  target.addEventListener('message', onMessage);
  const unsubscribe = manager.subscribe(publish);
  publish();
  return () => { unsubscribe(); target.removeEventListener('message', onMessage); };
}
