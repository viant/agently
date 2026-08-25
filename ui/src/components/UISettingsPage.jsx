import React from 'react';
import { Button, Switch } from '@blueprintjs/core';
import { resetUIPreferences, setDeveloperMode, useDeveloperMode } from '../services/uiPreferences';
import { uiSettingsReturnHref } from '../services/uiSettingsNavigation';

export default function UISettingsPage() {
  const developerMode = useDeveloperMode();
  const returnHref = uiSettingsReturnHref();
  return (
    <main className="app-ui-settings" data-testid="ui-settings-page">
      <header className="app-ui-settings-header">
        <a href={returnHref} className="app-ui-settings-back" aria-label="Back to Agently">← Agently</a>
        <h1>UI Settings</h1>
      </header>
      <section className="app-ui-settings-card">
        <div>
          <div className="app-ui-settings-title">Developer mode</div>
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
      <Button minimal icon="reset" text="Reset UI defaults" onClick={resetUIPreferences} />
    </main>
  );
}
