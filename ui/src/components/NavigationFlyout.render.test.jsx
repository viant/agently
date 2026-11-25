// @vitest-environment jsdom
import React from 'react';
import {afterEach, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {Menu, MenuItem} from '@blueprintjs/core';
import NavigationFlyout, {NavigationSubmenu} from 'forge/components/NavigationFlyout.jsx';
import {ForgeThemeBoundary, ForgeThemeProvider} from 'forge/components/ThemeBoundary.jsx';

afterEach(cleanup);
function Example({disabled = false, activate = () => {}, nested = false}) {
  return <ForgeThemeProvider themeId="design-system-2"><ForgeThemeBoundary>
    <NavigationFlyout title="Records" icon="lucide:file-text" disabled={disabled}>
      <Menu aria-label="Records"><MenuItem text="Overview" onClick={activate}/><MenuItem text="Unavailable" disabled/>
        <MenuItem text="History" onClick={activate}/>
        {nested ? <NavigationSubmenu text="Collections"><MenuItem text="Shared records" onClick={activate}/></NavigationSubmenu> : null}
      </Menu>
    </NavigationFlyout><button>Outside</button>
  </ForgeThemeBoundary></ForgeThemeProvider>;
}

it('uses native hover timing, retains the owned menu, and dismisses an activated option', async () => {
  const activate = vi.fn();
  render(<Example activate={activate}/>);
  const trigger = screen.getByRole('button', {name: 'Records'});
  expect(trigger.getAttribute('aria-expanded')).toBe('false');
  fireEvent.mouseEnter(trigger);
  const menu = await screen.findByRole('menu', {name: 'Records'});
  expect(trigger.getAttribute('aria-expanded')).toBe('true');
  expect(menu.closest('[data-forge-theme="design-system-2"]')).not.toBeNull();
  fireEvent.mouseLeave(trigger);
  fireEvent.mouseEnter(menu.closest('.bp6-popover'));
  await new Promise(resolve => setTimeout(resolve, 220));
  expect(trigger.getAttribute('aria-expanded')).toBe('true');
  fireEvent.click(screen.getByRole('menuitem', {name: 'History'}));
  expect(activate).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(trigger.getAttribute('aria-expanded')).toBe('false'));
});

it('opens by keyboard, skips unavailable options, and returns focus on Escape', async () => {
  render(<Example/>);
  const trigger = screen.getByRole('button', {name: 'Records'});
  trigger.focus();
  fireEvent.keyDown(trigger, {key: 'ArrowDown'});
  const overview = await screen.findByRole('menuitem', {name: 'Overview'});
  await waitFor(() => expect(document.activeElement).toBe(overview));
  fireEvent.keyDown(overview, {key: 'ArrowDown'});
  expect(document.activeElement).toBe(screen.getByRole('menuitem', {name: 'History'}));
  fireEvent.keyDown(document.activeElement, {key: 'Escape', code: 'Escape', keyCode: 27});
  await waitFor(() => expect(trigger.getAttribute('aria-expanded')).toBe('false'));
  expect(document.activeElement).toBe(trigger);
});

it('opens on click for touch access and leaves disabled categories unavailable', async () => {
  const mounted = render(<Example/>);
  fireEvent.click(screen.getByRole('button', {name: 'Records'}));
  expect(await screen.findByRole('menu', {name: 'Records'})).not.toBeNull();
  mounted.unmount();
  render(<Example disabled/>);
  const trigger = screen.getByRole('button', {name: 'Records'});
  expect(trigger.disabled).toBe(true);
  fireEvent.click(trigger);
  fireEvent.mouseEnter(trigger);
  expect(trigger.getAttribute('aria-expanded')).toBe('false');
  expect(screen.queryByRole('menu', {name: 'Records'})).toBeNull();
});

it('returns keyboard focus to the native parent when dismissing a nested menu', async () => {
  render(<Example nested/>);
  fireEvent.click(screen.getByRole('button', {name: 'Records'}));
  const parent = await screen.findByRole('menuitem', {name: /Collections/});
  parent.focus();
  fireEvent.keyDown(parent, {key: 'ArrowRight'});
  const child = await screen.findByRole('menuitem', {name: 'Shared records'});
  await waitFor(() => expect(document.activeElement).toBe(child));
  fireEvent.keyDown(child, {key: 'Escape'});
  expect(document.activeElement).toBe(parent);
  expect(screen.getByRole('button', {name: 'Records'}).getAttribute('aria-expanded')).toBe('true');
});
