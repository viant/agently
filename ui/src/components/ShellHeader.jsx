import React from 'react';
import {Icon, Typography, typographyProps} from 'forge/components';
import './shellHeader.css';

// The shell owns geometry; the workspace's semantic theme owns appearance.
export const ShellIconButton = React.forwardRef(function ShellIconButton({icon, label, children, className = '', minimal, intent, ...props}, ref) {
  return <button type="button" {...typographyProps('label')} {...props} ref={ref}
    aria-label={props['aria-label'] || label} title={props.title || label}
    className={`app-shell-icon-button ${className}`}>
    {icon ? typeof icon === 'string' ? <Icon name={icon}/> : icon : null}{children}
  </button>;
});

export function ShellLocation({title = 'New Chat', history, onToggleNavigation, navigationRef}) {
  return <>
    <ShellIconButton ref={navigationRef} icon="lucide:menu" label="Open navigation"
      className="app-shell-menu-toggle" data-testid="sidebar-toggle" disabled={!onToggleNavigation} onClick={onToggleNavigation}/>
    <nav className="app-shell-history" aria-label="Navigation history">
      <ShellIconButton icon="lucide:arrow-left" label="Go back" disabled={!history?.canGoBack} onClick={history?.back}/>
      <ShellIconButton icon="lucide:arrow-right" label="Go forward" disabled={!history?.canGoForward} onClick={history?.forward}/>
    </nav>
    <Typography as="h1" typeRole="label" className="app-shell-location" title={title}>{title}</Typography>
  </>;
}
