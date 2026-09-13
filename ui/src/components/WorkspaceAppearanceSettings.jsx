import React from 'react';
import {Button} from '@blueprintjs/core';
import {useWorkspaceStyle} from './WorkspaceStyleProvider';

export default function WorkspaceAppearanceSettings() {
  const appearance = useWorkspaceStyle();
  if (!appearance) return null;
  const {state, manager, refresh} = appearance;
  const selected = state.catalog?.themes.find(theme => theme.id === state.themeId);
  return <section className="app-ui-settings-card" aria-label="Workspace appearance">
    <div>
      <h2 className="app-ui-settings-title">Workspace appearance</h2>
      <p>Choose how workspace windows and their controls appear.</p>
      {state.catalog ? <div className="app-workspace-appearance-controls">
        <label>Theme <select aria-label="Workspace theme" value={state.themeId || 'forge-default'}
          onChange={event => manager.setPreference(event.target.value, state.modePreference)}>
          <option value="forge-default">Forge default</option>
          {state.catalog.themes.map(theme => <option key={theme.id} value={theme.id}>{theme.label}</option>)}
        </select></label>
        <label>Color mode <select aria-label="Workspace color mode" value={state.modePreference} disabled={!selected}
          onChange={event => manager.setPreference(state.themeId, event.target.value)}>
          {['system', 'light', 'dark'].map(mode => <option key={mode} value={mode}
            disabled={mode !== 'system' && selected && !selected.modes[mode]}>{mode.charAt(0).toUpperCase() + mode.slice(1)}</option>)}
        </select></label>
        <span aria-live="polite">Currently {state.mode}</span>
      </div> : <p>No named themes are configured for this workspace.</p>}
      {state.diagnostics.length > 0 && <div role="status">{state.diagnostics.join(' · ')}</div>}
      {refresh && <Button minimal icon="refresh" onClick={refresh}>Reload workspace appearance</Button>}
    </div>
  </section>;
}
