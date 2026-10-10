import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

let developerMode = false;

vi.mock('@blueprintjs/core', () => ({
  Icon: ({ icon, ...props }) => <span {...props} data-icon={icon} />,
  Button: ({ text = '', icon = '', minimal: _minimal, ...props }) => <button {...props} data-icon={icon}>{text}</button>,
  Switch: ({ checked, ...props }) => <input {...props} type="checkbox" checked={checked} readOnly />,
}));

vi.mock('../services/uiPreferences', () => ({
  useDeveloperMode: () => developerMode,
  setDeveloperMode: vi.fn(),
  resetUIPreferences: vi.fn(),
}));

import UISettingsPage from './UISettingsPage';

describe('UISettingsPage', () => {
  afterEach(() => vi.unstubAllGlobals());
  beforeEach(() => {
    developerMode = false;
  });

  it('exposes Developer mode as an off-by-default UI preference', () => {
    const html = renderToStaticMarkup(<UISettingsPage />);
    expect(html).toContain('UI Settings');
    expect(html).toContain('Developer mode');
    expect(html).toContain('Saved automatically · Default');
    expect(html).not.toContain('checked=""');
  });

  it('restores the connectors tab after an OAuth return', () => {
    vi.stubGlobal('window', { location: { search: '?tab=connectors&returnTo=%2Fv1%2Fconversation%2Fexample' } });
    const html = renderToStaticMarkup(<UISettingsPage />);
    expect(html).toContain('id="settings-connectors" role="tab" aria-selected="true"');
    expect(html).toContain('Loading connections');
    expect(html).toContain('href="/v1/conversation/example"');
    expect(html).not.toContain('Reset UI defaults');
  });

  it('reflects an enabled Developer mode preference', () => {
    developerMode = true;
    const html = renderToStaticMarkup(<UISettingsPage />);
    expect(html).toContain('checked=""');
    expect(html).toContain('Saved automatically · On');
    expect(html).toContain('Reset UI defaults');
  });
});
