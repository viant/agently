import ComposerDefaultsSettings from "./ComposerDefaultsSettings";
import {ForgeThemeBoundary} from "forge/components";
import WorkspaceAppearanceSettings from './WorkspaceAppearanceSettings';
import React from 'react';
import { Button, Icon, Switch } from '@blueprintjs/core';
import { resetUIPreferences, setDeveloperMode, useDeveloperMode } from '../services/uiPreferences';
import { uiSettingsReturnHref } from '../services/uiSettingsNavigation';

export default function UISettingsPage() {
  const developerMode = useDeveloperMode();
  const returnHref = uiSettingsReturnHref();
  return (
    <ForgeThemeBoundary windowKey="ui-settings"><main className="app-ui-settings" data-testid="ui-settings-page">
      <header className="app-ui-settings-header">
        <a href={returnHref} className="app-ui-settings-back" aria-label="Back to Agently"><Icon icon="arrow-left" aria-hidden="true" /> Agently</a>
        <h1>UI Settings</h1>
      </header>
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
      <Button className="app-ui-settings-reset" minimal icon="reset" text="Reset UI defaults" onClick={resetUIPreferences} />
    </main></ForgeThemeBoundary>
  );
}
