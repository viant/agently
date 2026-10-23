import React from 'react';
import {typographyProps, Button as SharedButton} from 'forge/components';
import {Button} from '@blueprintjs/core';
import {useWorkspaceStyle} from './WorkspaceStyleProvider';

export default function WorkspaceAppearanceSettings() {
  const appearance = useWorkspaceStyle();
  if (!appearance) return null;
  const {state, manager, refresh} = appearance;
  const selected = state.catalog?.themes.find(theme => theme.id === state.themeId);
  const text = role => selected?.modes[state.mode]?.[`typography.${role}.weight`] === undefined ? {} : typographyProps(role);
  const sharedActions = !!text('label')['data-forge-type'];
  const RefreshButton = sharedActions ? SharedButton : Button;
  return <section className="app-ui-settings-card" aria-label="Workspace appearance">
    <div>
      <h2 {...text('section')} className="app-ui-settings-title">Workspace appearance</h2>
      <p {...text('body')}>Choose how workspace windows and their controls appear.</p>
      {state.catalog ? <div className="app-workspace-appearance-controls">
        <label {...text('label')}>Theme <select {...text('body')} aria-label="Workspace theme" value={state.themeId || 'forge-default'}
          onChange={event => manager.setPreference(event.target.value, state.modePreference)}>
          <option value="forge-default">Forge default</option>
          {state.catalog.themes.map(theme => <option key={theme.id} value={theme.id}>{theme.label}</option>)}
        </select></label>
        <label {...text('label')}>Color mode <select {...text('body')} aria-label="Workspace color mode" value={state.modePreference} disabled={!selected}
          onChange={event => manager.setPreference(state.themeId, event.target.value)}>
          {['system', 'light', 'dark'].map(mode => <option key={mode} value={mode}
            disabled={mode !== 'system' && selected && !selected.modes[mode]}>{mode.charAt(0).toUpperCase() + mode.slice(1)}</option>)}
        </select></label>
        <span {...text('metadata')} aria-live="polite">Currently {state.mode}</span>
      </div> : <p {...text('body')}>No named themes are configured for this workspace.</p>}
      {state.diagnostics.length > 0 && <div {...text('metadata')} role="status">{state.diagnostics.join(' · ')}</div>}
      {refresh && <RefreshButton {...(sharedActions ? {variant: 'ghost', icon: 'lucide:refresh-cw'} : {minimal: true, icon: 'refresh'})} onClick={refresh}>Reload workspace appearance</RefreshButton>}
    </div>
  </section>;
}
