import React from 'react';
import Chat from 'forge/components/Chat.jsx';
import {signal} from '@preact/signals-react';
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
  const chatContext = React.useMemo(() => ({
    identity: {windowId: 'specimen-workspace-chat'},
    signals: {collection: signal([]), control: signal({}), selection: signal(null), message: signal('')},
    handlers: {chat: {}}, Context: () => null,
    lookupHandler: name => name === 'chat.renderFeed'
      ? () => <div className="app-chat-feed">{Array.from({length: 12}, (_, index) => <Typography as="p" typeRole="body" key={index}>Example conversation item {index + 1}. The conversation remains available while reviewing the output.</Typography>)}</div>
      : name === 'chat.resolveComposerProps' ? () => ({conversationId: 'specimen-workspace-chat'}) : undefined,
  }), []);

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
        renderConversation={() => <div className="app-navigation-chat-fixture" style={{display: 'flex'}}><Chat context={chatContext} container={{chat:{appearance:'document',showUpload:true,showMic:true}}}/></div>}
        renderWorkspace={() => <Typography as="p" typeRole="body">A named tab, separate hide and expand actions, and a retained content panel.</Typography>}/>
    </div>
  </section>;
}
