import React from 'react';
import {Icon, typographyProps} from 'forge/components';
import {Button} from '@blueprintjs/core';
import './sectionNavigation.css';

export function SectionDestinationButton({selected = false, className = '', ...props}) {
  return <Button {...props} minimal fill alignText="left" className={`app-layout-menu ${className}`}
    {...typographyProps('body')} aria-current={selected ? 'page' : undefined}/>;
}

// Section selection never changes a conversation or opens a business window.
// The host supplies those actions through the existing layout contract.
export default function SectionNavigation({section = 'chat', actions = [], onSelect, brandRef, headingActionsRef, profileRef, children, drawerOpen = false, onDismiss, returnFocusRef}) {
  const rootRef = React.useRef(null);
  const dismissRef = React.useRef(onDismiss);
  dismissRef.current = onDismiss;
  React.useEffect(() => {
    if (!drawerOpen) return undefined;
    const root = rootRef.current;
    const invoking = returnFocusRef?.current || document.activeElement;
    const controls = () => [...root.querySelectorAll('button:not(:disabled), input:not(:disabled), select:not(:disabled), a[href], [tabindex="0"]')]
      .filter(node => node.getBoundingClientRect().width > 0);
    (root.querySelector('[aria-current="page"]') || controls()[0])?.focus();
    const keydown = event => {
      if (event.defaultPrevented || !root.contains(event.target)) return;
      if (event.key === 'Escape' && !root.querySelector('[role="menu"]')) {
        event.preventDefault(); dismissRef.current?.();
      } else if (event.key === 'Tab') {
        const nodes = controls();
        const first = nodes[0];
        const last = nodes[nodes.length - 1];
        if (event.shiftKey && document.activeElement === first) {event.preventDefault(); last?.focus();}
        else if (!event.shiftKey && document.activeElement === last) {event.preventDefault(); first?.focus();}
      }
    };
    document.addEventListener('keydown', keydown);
    return () => {document.removeEventListener('keydown', keydown); if (invoking?.isConnected) invoking.focus();};
  }, [drawerOpen, returnFocusRef]);
  const destinations = [
    {id: 'chat', title: 'Chat', icon: 'lucide:house'},
    {id: 'workspaces', title: 'Workspaces', icon: 'lucide:layout-grid'},
    ...actions.map(action => ({...action, id: `action:${action.id}`})),
  ];
  return <div className="app-section-navigation" ref={rootRef} role={drawerOpen ? 'dialog' : undefined} aria-modal={drawerOpen ? true : undefined} aria-label={drawerOpen ? 'Navigation' : undefined}>
    <nav className="app-section-rail" aria-label="App sections">
      {drawerOpen ? <button type="button" className="app-section-rail-button" aria-label="Close navigation" onClick={onDismiss}><Icon name="lucide:x"/></button> : null}
      {destinations.map(destination => <button key={destination.id} type="button"
        className="app-section-rail-button" disabled={destination.disabled}
        aria-label={destination.title} title={destination.title}
        aria-current={section === destination.id ? 'page' : undefined}
        onClick={() => onSelect?.(destination.id)}>
        <Icon name={destination.icon || 'lucide:clock-3'}/>
      </button>)}
      <div className="app-section-profile" ref={profileRef}/>
    </nav>
    <div className="app-section-menu" {...typographyProps('body')}>
      <div className="app-section-masthead"><div className="app-section-brand" ref={brandRef}/><div ref={headingActionsRef} hidden={section !== 'chat'}/></div>
      {children}
    </div>
  </div>;
}
