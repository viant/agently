import React from 'react';
import {Typography} from 'forge/components';
import SectionNavigation, {SectionDestinationButton} from './SectionNavigation.jsx';
import {ShellLocation} from './ShellHeader.jsx';
import ConversationWorkspaceSurface from './ConversationWorkspaceSurface.jsx';

export default function NavigationSpecimen() {
  const [section, setSection] = React.useState('chat');
  const [mount, setMount] = React.useState(null);
  const [surface, setSurface] = React.useState('workspace');
  const [mode, setMode] = React.useState('split');
  const [open, setOpen] = React.useState(true);
  const [destination, setDestination] = React.useState('overview');
  const example = {windowId: 'specimen-output', windowTitle: 'Example output'};
  return <section id="foundation-navigation" aria-label="Section navigation specimen">
    <Typography as="h2" typeRole="section">Section navigation</Typography>
    <Typography as="p" typeRole="body">App sections have separate menus. Switching sections keeps the conversation available.</Typography>
    <div className="app-navigation-specimen app-layout-sidebar is-sections">
      <SectionNavigation section={section} actions={[{id: 'schedule', title: 'Scheduled tasks', icon: 'lucide:clock-3'}, {id: 'unavailable', title: 'Unavailable section', disabled: true}]}
        onSelect={setSection}>
        <Typography as="h3" typeRole="section">{section === 'chat' ? 'Conversations' : section === 'workspaces' ? 'Workspaces' : 'Scheduled tasks'}</Typography>
        {section === 'workspaces' ? <>
          <SectionDestinationButton selected={destination === 'overview'} onClick={() => setDestination('overview')}>Overview</SectionDestinationButton>
          <SectionDestinationButton selected={destination === 'records'} onClick={() => setDestination('records')}>Example records</SectionDestinationButton>
          <SectionDestinationButton disabled>Unavailable destination</SectionDestinationButton>
        </> : <Typography as="p" typeRole="body">{section === 'chat' ? 'Example conversation' : 'Example schedule'}</Typography>}
      </SectionNavigation>
    </div>
    <Typography as="h3" typeRole="section">Shell and viewer controls</Typography>
    <div className="app-header-specimen">
      <header className={`app-topbar is-document${open && surface === 'workspace' ? ' has-viewer' : ''}${mode === 'focus' ? ' viewer-expanded' : ''}`}>
        <div className="app-topbar-left"><ShellLocation title="Example conversation"/></div>
        <div className="app-shell-viewer-header" ref={setMount}/>
      </header>
      <ConversationWorkspaceSurface presentation="document" headerMount={mount} activeSurface={surface} workspaceMode={mode} onChangeWorkspaceMode={setMode}
        workspaceWindow={open ? example : null} workspaceWindows={open ? [example] : []}
        onOpenWorkspace={() => setSurface('workspace')} onBackToConversation={() => setSurface('conversation')}
        onCloseWorkspaceTab={() => {setOpen(false); setSurface('conversation');}}
        renderConversation={() => <Typography as="p" typeRole="body">The conversation stays mounted while the viewer changes.</Typography>}
        renderWorkspace={() => <Typography as="p" typeRole="body">A named tab, separate hide and expand actions, and a retained content panel.</Typography>}/>
    </div>
  </section>;
}
