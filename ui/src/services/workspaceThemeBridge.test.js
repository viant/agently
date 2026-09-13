import {describe, it, expect, vi} from 'vitest';
import {acceptsThemeSelection, installWorkspaceThemeBridge} from './workspaceThemeBridge';
function setup(guest = false) {
  const source = {postMessage: vi.fn()};
  const target = {location: {origin: 'https://app.example', href: 'https://app.example/mcp-ui/forge-window', pathname: guest ? '/mcp-ui/forge-window' : '/'},
    document: {querySelectorAll: () => [{src: 'https://app.example/mcp-ui/forge-window', contentWindow: source}]},
    addEventListener: vi.fn(), removeEventListener: vi.fn()};
  target.parent = guest ? source : target;
  const manager = {workspaceId: 'workspace', state: {revision: 'revision', themeId: 'theme', modePreference: 'system', mode: 'dark',
    catalog: {themes: [{id: 'theme', modes: {light: {}, dark: {}}}]}}, subscribe: vi.fn(() => vi.fn()), applyExternalSelection: vi.fn()};
  const refresh = vi.fn();
  const cleanup = installWorkspaceThemeBridge(manager, refresh, target);
  const receive = data => target.addEventListener.mock.calls[0][1](data);
  return {source, target, manager, refresh, cleanup, receive};
}
const selection = {type: 'agently.workspace-theme', workspaceId: 'workspace', revision: 'revision', themeId: 'theme', modePreference: 'system', mode: 'dark'};
describe('workspace theme bridge', () => {
  it('accepts only known themes/modes and matching workspace/revision', () => {
    const {manager} = setup();
    expect(acceptsThemeSelection(selection, manager)).toBe(true);
    for (const change of [{workspaceId: 'other'}, {revision: 'old'}, {themeId: 'unknown'}, {mode: 'system'}, {modePreference: 'unknown'}]) {
      expect(acceptsThemeSelection({...selection, ...change}, manager)).toBe(false);
    }
  });
  it('ignores unrelated senders and refreshes a guest with stale catalog', () => {
    const {source, manager, refresh, receive, cleanup, target} = setup(true);
    receive({origin: 'https://evil.example', source, data: selection});
    receive({origin: target.location.origin, source: {}, data: selection});
    expect(manager.applyExternalSelection).not.toHaveBeenCalled();
    receive({origin: target.location.origin, source, data: selection});
    expect(manager.applyExternalSelection).toHaveBeenCalledWith(selection);
    receive({origin: target.location.origin, source, data: {...selection, revision: 'new'}});
    expect(refresh).toHaveBeenCalledTimes(1);
    receive({origin: target.location.origin, source, data: {...selection, workspaceId: 'new-workspace'}});
    expect(refresh).toHaveBeenCalledTimes(2);
    expect(manager.applyExternalSelection).toHaveBeenCalledTimes(1);
    cleanup(); expect(target.removeEventListener).toHaveBeenCalled();
  });
  it('sends only to the registered same-origin Forge frame', () => {
    const {source, receive, target} = setup();
    expect(source.postMessage).toHaveBeenCalledWith(selection, target.location.origin);
    source.postMessage.mockClear();
    receive({origin: target.location.origin, source: {}, data: {type: 'agently.workspace-theme-ready', workspaceId: 'workspace'}});
    expect(source.postMessage).not.toHaveBeenCalled();
    receive({origin: target.location.origin, source, data: {type: 'agently.workspace-theme-ready', workspaceId: 'workspace'}});
    expect(source.postMessage).toHaveBeenCalledWith(selection, target.location.origin);
  });
});
