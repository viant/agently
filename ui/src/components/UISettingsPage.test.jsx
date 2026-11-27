import {MemoryRouter} from 'react-router-dom';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

let developerMode = false;

vi.mock('@blueprintjs/core', () => ({
  Dialog:({isOpen,children})=>isOpen?<div>{children}</div>:null,
  InputGroup:props=><input {...props}/>,
  Callout:({children})=><div>{children}</div>,
  FormGroup:({children})=><div>{children}</div>,
  TextArea:props=><textarea {...props}/>,
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
    const html = renderToStaticMarkup(<MemoryRouter><UISettingsPage /></MemoryRouter>);
    expect(html).toContain('UI Settings');
    expect(html).toContain('Developer mode');
    expect(html).toContain('Saved automatically · Default');
    expect(html).not.toContain('checked=""');
  });

  it('restores the connectors tab after an OAuth return', () => {
    vi.stubGlobal('window', { location: { search: '?tab=connectors&returnTo=%2Fv1%2Fconversation%2Fexample' } });
    const html = renderToStaticMarkup(<MemoryRouter><UISettingsPage /></MemoryRouter>);
    expect(html).toContain('id="settings-connectors" role="tab" aria-selected="true"');
    expect(html).toContain('Loading connections');
    expect(html).toContain('href="/v1/conversation/example"');
    expect(html).not.toContain('Reset UI defaults');
  });

  it('keeps workspace editing out of user settings even at the old starting-task URL', () => {
    vi.stubGlobal('window', { location: { search: '?tab=starting-tasks' } });
    const html = renderToStaticMarkup(<MemoryRouter><UISettingsPage /></MemoryRouter>);
    expect(html).toContain('Manage Workspace');
    expect(html).not.toContain('id="settings-starting-tasks"');
    expect(html).not.toContain('Loading starting tasks');
    expect(html).toContain('Reset UI defaults');
    expect(html).toContain('href="/ui/workspace?returnTo=');
    expect(html).not.toContain('aria-label="Manage Workspace"');
  });

  it('reflects an enabled Developer mode preference', () => {
    developerMode = true;
    const html = renderToStaticMarkup(<MemoryRouter><UISettingsPage /></MemoryRouter>);
    expect(html).toContain('checked=""');
    expect(html).toContain('Saved automatically · On');
    expect(html).toContain('Reset UI defaults');
  });
});
