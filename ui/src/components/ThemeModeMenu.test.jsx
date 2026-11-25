// @vitest-environment jsdom
import React from 'react';
import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';

const appearance = {
  state: {themeId: 'baseline', modePreference: 'light', catalog: {defaultTheme: 'baseline'}},
  manager: {setPreference: vi.fn()},
};

vi.mock('./WorkspaceStyleProvider', () => ({useWorkspaceStyle: () => appearance}));

const {default: ThemeModeMenu} = await import('./ThemeModeMenu.jsx');

describe('ThemeModeMenu', () => {
  afterEach(() => {cleanup(); appearance.manager.setPreference.mockClear();});

  it('shows the selected mode and saves a new preference', async () => {
    render(<ThemeModeMenu/>);
    fireEvent.click(screen.getByRole('button', {name: 'Theme settings'}));
    expect(screen.getByRole('menuitemradio', {name: /Light/}).getAttribute('aria-checked')).toBe('true');
    fireEvent.click(screen.getByRole('menuitemradio', {name: /Dark/}));
    expect(appearance.manager.setPreference).toHaveBeenCalledWith('baseline', 'dark');
    await waitFor(() => expect(screen.queryByRole('menu')).toBeNull());
  });

  it('supports arrow navigation, Escape, and outside dismissal', async () => {
    render(<><button>Outside</button><ThemeModeMenu/></>);
    const trigger = screen.getByRole('button', {name: 'Theme settings'});
    fireEvent.click(trigger);
    const light = screen.getByRole('menuitemradio', {name: /Light/});
    light.focus();
    fireEvent.keyDown(light, {key: 'ArrowDown'});
    expect(document.activeElement).toBe(screen.getByRole('menuitemradio', {name: /Dark/}));
    fireEvent.keyDown(document.activeElement, {key: 'Escape'});
    await waitFor(() => expect(screen.queryByRole('menu')).toBeNull());
    fireEvent.click(trigger);
    fireEvent.mouseDown(screen.getByRole('button', {name: 'Outside'}));
    await waitFor(() => expect(screen.queryByRole('menu')).toBeNull());
  });
});
