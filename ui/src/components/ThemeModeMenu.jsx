import React from 'react';
import {Icon, Popover, typographyProps} from 'forge/components';
import {useWorkspaceStyle} from './WorkspaceStyleProvider';

const MODES = [
  {id: 'light', label: 'Light', icon: 'lucide:sun'},
  {id: 'dark', label: 'Dark', icon: 'lucide:moon'},
  {id: 'system', label: 'System', icon: 'lucide:monitor'},
];

export default function ThemeModeMenu() {
  const appearance = useWorkspaceStyle();
  const state = appearance?.state;
  const manager = appearance?.manager;
  const triggerRef = React.useRef(null);
  const menuRef = React.useRef(null);
  const [open, setOpen] = React.useState(false);

  if (!state || !manager) return null;

  const selectedMode = ['light', 'dark', 'system'].includes(state.modePreference)
    ? state.modePreference
    : 'system';
  const selectMode = mode => {
    if (state.catalog) manager.setPreference(state.themeId || state.catalog.defaultTheme, mode);
    setOpen(false);
  };
  const moveFocus = (event, direction) => {
    const current = MODES.findIndex(mode => mode.id === event.currentTarget.dataset.mode);
    const next = (current + direction + MODES.length) % MODES.length;
    event.preventDefault();
    menuRef.current?.querySelector(`[data-mode="${MODES[next].id}"]`)?.focus();
  };

  return <div className="app-theme-mode-control">
    <Popover placement="right-end" minimal isOpen={open} onInteraction={setOpen} disabled={!state.catalog}
      autoFocus={false} enforceFocus={false} popoverClassName="app-theme-mode-popover"
      onOpened={() => menuRef.current?.querySelector('[aria-checked="true"]')?.focus()}
      content={<div className="app-theme-mode-menu" ref={menuRef} role="menu" aria-label="Color mode" {...typographyProps('body')}>
      {MODES.map(mode => <button key={mode.id} type="button" role="menuitemradio"
        className="app-theme-mode-option" data-mode={mode.id} tabIndex={mode.id === selectedMode ? 0 : -1}
        aria-checked={mode.id === selectedMode} onClick={() => selectMode(mode.id)}
        onKeyDown={event => {
          if (event.key === 'ArrowDown') moveFocus(event, 1);
          else if (event.key === 'ArrowUp') moveFocus(event, -1);
          else if (event.key === 'Home') { event.preventDefault(); menuRef.current?.querySelector('[data-mode="light"]')?.focus(); }
          else if (event.key === 'End') { event.preventDefault(); menuRef.current?.querySelector('[data-mode="system"]')?.focus(); }
        }}>
        <Icon name={mode.icon}/><span>{mode.label}</span>{mode.id === selectedMode ? <Icon name="lucide:check"/> : null}
      </button>)}
      </div>}>
      <button type="button" className="app-section-rail-button app-theme-mode-trigger" ref={triggerRef} disabled={!state.catalog}
        aria-label="Theme settings" title="Theme settings" aria-haspopup="menu" aria-expanded={open}
        onKeyDown={event => {
          if (event.key === 'ArrowDown' || event.key === 'ArrowUp') { event.preventDefault(); setOpen(true); }
        }}>
        <Icon name="lucide:sun-moon"/>
      </button>
    </Popover>
  </div>;
}
