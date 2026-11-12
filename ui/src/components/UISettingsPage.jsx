import {Icon as UnifiedIcon} from 'forge/components';
import ComposerDefaultsSettings from "./ComposerDefaultsSettings";
import {ForgeThemeBoundary} from "forge/components";
import WorkspaceAppearanceSettings from './WorkspaceAppearanceSettings';
import React, { useState } from 'react';
import ConnectorSettings from './ConnectorSettings';
import StarterTaskSettings from './StarterTaskSettings';
import './ConnectorSettings.css';
import {Button, Switch} from '@blueprintjs/core';
import { resetUIPreferences, setDeveloperMode, useDeveloperMode } from '../services/uiPreferences';
import { uiSettingsReturnHref } from '../services/uiSettingsNavigation';

const settingsTabs = [
  {id: 'preferences', label: 'Preferences'},
  {id: 'starting-tasks', label: 'Starting tasks'},
  {id: 'connectors', label: 'Connectors'},
];

export default function UISettingsPage() {
  const [tab, setTab] = useState(() => {
    const requested = typeof window !== 'undefined' && new URLSearchParams(window.location.search).get('tab');
    return settingsTabs.find(item => item.id === requested)?.id ?? 'preferences';
  });
  const selectTab = (next) => {
    setTab(next);
    const url = new URL(window.location.href);
    url.searchParams.set('tab', next);
    window.history.replaceState(null, '', url);
  };
  const developerMode = useDeveloperMode();
  const returnHref = uiSettingsReturnHref();
  return (
    <ForgeThemeBoundary windowKey="ui-settings"><main className="app-ui-settings" data-testid="ui-settings-page">
      <header className="app-ui-settings-header">
        <a href={returnHref} className="app-ui-settings-back" aria-label="Back to Agently"><UnifiedIcon name="arrow-left" aria-hidden="true" /> Agently</a>
        <h1>UI Settings</h1>
      </header>
      <div role="tablist" aria-label="Settings" className="app-settings-tabs">
        {settingsTabs.map(({id: value, label}, index) => <button key={value} id={`settings-${value}`} role="tab" aria-selected={tab === value} aria-controls={`settings-panel-${value}`} tabIndex={tab === value ? 0 : -1} onClick={() => selectTab(value)} onKeyDown={(event) => {
          if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) {
            event.preventDefault();
            const count = settingsTabs.length;
            const positions = {Home: 0, End: count - 1, ArrowLeft: (index + count - 1) % count, ArrowRight: (index + 1) % count};
            const next = settingsTabs[positions[event.key]].id;
            selectTab(next);
            document.getElementById(`settings-${next}`)?.focus();
          }
        }}>{label}</button>)}
      </div>
      {tab === 'connectors' ? <div id="settings-panel-connectors" role="tabpanel" aria-labelledby="settings-connectors"><ConnectorSettings /></div> :
      tab === 'starting-tasks' ? <div id="settings-panel-starting-tasks" role="tabpanel" aria-labelledby="settings-starting-tasks"><StarterTaskSettings /></div> :
      <div id="settings-panel-preferences" role="tabpanel" aria-labelledby="settings-preferences">
      <ComposerDefaultsSettings />
      <WorkspaceAppearanceSettings />
      <section className="app-ui-settings-card app-ui-settings-developer">
        <div>
          <h2 className="app-ui-settings-title">Developer mode</h2>
          <p>Expose execution groups, payloads, and provider diagnostics.</p>
          <div className="app-ui-settings-state" aria-live="polite">
            Saved automatically · {developerMode ? 'On' : 'Default'}
          </div>
        </div>
        <Switch
          checked={developerMode}
          aria-label="Developer mode"
          onChange={(event) => setDeveloperMode(event.target.checked)}
        />
      </section>
      <Button className="app-ui-settings-reset" minimal icon={<UnifiedIcon name="reset"/>} text="Reset UI defaults" onClick={resetUIPreferences} />
      </div>}
    </main></ForgeThemeBoundary>
  );
}
