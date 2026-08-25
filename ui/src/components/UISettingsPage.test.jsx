import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';

let developerMode = false;

vi.mock('@blueprintjs/core', () => ({
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

  it('reflects an enabled Developer mode preference', () => {
    developerMode = true;
    const html = renderToStaticMarkup(<UISettingsPage />);
    expect(html).toContain('checked=""');
    expect(html).toContain('Saved automatically · On');
    expect(html).toContain('Reset UI defaults');
  });
});
