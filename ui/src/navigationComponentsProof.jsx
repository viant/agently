import React from 'react';
import {createRoot} from 'react-dom/client';
import {Typography, SelectControl, ForgeThemeBoundary} from 'forge/components';
import WidgetRenderer from 'forge/runtime/WidgetRenderer.jsx';
import ConversationWorkspaceSurface from './components/ConversationWorkspaceSurface.jsx';
import {useShellNavigation} from './hooks/useShellNavigation.js';
import {useConversationHeading} from './hooks/useConversationHeading.js';
import {publishWorkspaceMetadataSnapshot} from './services/workspaceMetadata.js';
import {openConversationInMainWindow} from './services/conversationWindow.js';
import WorkspaceStyleProvider from './components/WorkspaceStyleProvider.jsx';
import {useWorkspaceStyle} from './components/WorkspaceStyleProvider.jsx';
import WorkspaceSidebar from './components/WorkspaceSidebar.jsx';
import MenuBar from './components/MenuBar.jsx';
import fixture from '/__typography/fixture.js';
import '@blueprintjs/core/lib/css/blueprint.css';
import 'forge/packs/blueprint/index.jsx';
import './styles/shell.css';
import './index.css';
import './styles/typography.css';
import './navigationComponentsProof.css';

publishWorkspaceMetadataSnapshot(fixture.metadata);

function NavigationReview() {
  const [section, setSection] = React.useState('chat');
  const [destination, setDestination] = React.useState({});
  const [brand, setBrand] = React.useState(null);
  const [profile, setProfile] = React.useState(null);
  const [drawer, setDrawer] = React.useState(false);
  const [collapsed, setCollapsed] = React.useState(false);
  const triggerRef = React.useRef(null);
  const [viewerHeader, setViewerHeader] = React.useState(null);
  const [conversationId, setConversationId] = React.useState('');
  const [activeSurface, setActiveSurface] = React.useState('workspace');
  const [workspaceMode, setWorkspaceMode] = React.useState('split');
  const [windows, setWindows] = React.useState([
    {windowId: 'example-output', windowKey: 'example-output', windowTitle: 'Example output'},
    {windowId: 'second-output', windowKey: 'second-output', windowTitle: 'Second output'},
  ]);
  const [selectedId, setSelectedId] = React.useState('example-output');
  const sampleFieldState = React.useState({sampleValue: 'Retained when hidden'});
  const heading = useConversationHeading(conversationId);
  const navigation = useShellNavigation({enabled: true, section, conversationId, onRestore: entry => {setSection(entry.section); setConversationId(entry.conversationId); if (!entry.selectionInProgress && entry.conversationId !== conversationId) openConversationInMainWindow(entry.conversationId);}});
  React.useEffect(() => {
    const select = event => setConversationId(event.detail.id || '');
    window.addEventListener('agently:conversation-select', select);
    window.addEventListener('agently:conversation-new', select);
    return () => {window.removeEventListener('agently:conversation-select', select); window.removeEventListener('agently:conversation-new', select);};
  }, []);
  const selected = windows.find(entry => entry.windowId === selectedId) || windows[0];
  const closeOutput = id => {setWindows(previous => previous.filter(entry => entry.windowId !== id)); if (windows.length === 1) setActiveSurface('conversation');};
  const [compact, setCompact] = React.useState(() => innerWidth <= 760);
  React.useEffect(() => {
    const update = () => setCompact(innerWidth <= 760);
    window.addEventListener('resize', update);
    return () => window.removeEventListener('resize', update);
  }, []);
  return <div className={`app-shell has-section-navigation${compact ? ' is-compact-shell' : ''}`} style={{'--app-sidebar-width': collapsed && !compact ? '48px' : '244px'}}>
    <MenuBar sectionNavigation compactNavigation={compact} heading={section === 'chat' ? heading : destination[section] || (section === 'workspaces' ? 'Workspaces' : 'Scheduled tasks')} history={navigation}
      viewerHeaderRef={setViewerHeader} viewerVisible={section === 'chat' && activeSurface === 'workspace' && !!selected} viewerExpanded={workspaceMode === 'focus'} sidebarCollapsed={collapsed} brandMount={brand} profileMount={profile} sidebarToggleRef={triggerRef} inert={compact && drawer} onToggleSidebar={() => compact ? setDrawer(value => !value) : setCollapsed(value => !value)}/>
    <div className={`app-main${compact ? ' is-compact-shell' : ''}`}>
      {compact && drawer ? <button className="app-sidebar-scrim" aria-label="Close navigation" onClick={() => setDrawer(false)}/> : null}
      <WorkspaceSidebar collapsed={!compact && collapsed} onExpand={() => setCollapsed(false)} drawerMode={compact} drawerHidden={compact && !drawer} onDismiss={() => setDrawer(false)} returnFocusRef={triggerRef} section={section} onSectionChange={navigation.selectSection} brandRef={setBrand} profileRef={setProfile}
        onSectionWindow={(entry, title, target = section) => {setDestination(previous => ({...previous, [target]: title})); window.__navigationOpened = entry;}}
        onNavigate={() => setDrawer(false)}/>
      <main className="app-chat-pane app-section-page" inert={compact && drawer ? '' : undefined}>
        <div className="app-shell-conversation-host" hidden={section !== 'chat'}><ConversationWorkspaceSurface headerActive={section === 'chat'} presentation="document" headerMount={viewerHeader} activeSurface={activeSurface}
          workspaceMode={workspaceMode} onChangeWorkspaceMode={setWorkspaceMode} workspaceWindow={selected} workspaceWindows={windows}
          workspaceTabs={windows.map(entry => ({windowId: entry.windowId, label: entry.windowTitle}))}
          onSelectWorkspaceTab={id => {setSelectedId(id); setActiveSurface('workspace');}} onCloseWorkspaceTab={closeOutput}
          onOpenWorkspace={() => setActiveSurface('workspace')} onBackToConversation={() => setActiveSurface('conversation')}
          renderWorkspace={entry => <ForgeThemeBoundary windowKey={entry.windowKey}><div className="app-section-empty" style={{color: 'var(--forge-text)'}}>
            <Typography as="h2" typeRole="section">{entry.windowTitle}</Typography>
            <Typography as="p" typeRole="body">Sample content for reviewing the real shared viewer controls. This is not a saved business report.</Typography>
            <div style={{maxWidth: 320}}><WidgetRenderer item={{id: 'sampleValue', label: `Example value in ${entry.windowTitle}`, widget: 'text', scope: 'local'}} state={sampleFieldState}/></div>
          </div></ForgeThemeBoundary>}
          renderConversation={() => <div className="app-section-empty">
            <Typography as="h2" typeRole="section">{heading}</Typography>
            <Typography as="p" typeRole="body">Local shell review using the configured workspace layout and theme. The sample output exercises the shared viewer, tabs, and field.</Typography>
          </div>}/></div>
        <div className="app-section-empty" hidden={section === 'chat'}>
          <Typography as="h1" typeRole="heading">{section === 'chat' ? 'New Chat' : section === 'workspaces' ? destination[section] || 'Workspaces' : destination[section] || 'Scheduled tasks'}</Typography>
          <Typography as="p" typeRole="body">Local navigation component review with sample conversation history.</Typography>
          <Typography as="p" typeRole="body">The rail, menus, history, search and account controls use the shared application components. Destination content and the chat redesign are the next integration steps.</Typography>
        </div>
      </main>
    </div>
  </div>;
}

function AppearanceReviewControls() {
  const {state, manager} = useWorkspaceStyle();
  const themes = state.catalog?.themes.filter(theme => ['baseline', 'design-system-2'].includes(theme.id)) || [];
  if (!themes.length) return null;
  return <div className="app-appearance-review" aria-label="Local appearance review">
    <span className="app-appearance-review-caption">Local preview</span>
    <label>Theme <SelectControl aria-label="Review theme" value={state.themeId} style={{background: 'var(--app-surface)', color: 'var(--app-text)', borderColor: 'var(--app-border)', borderRadius: 'var(--app-shell-control-radius)'}} onChange={event => manager.setPreference(event.target.value, state.mode)}>{themes.map(theme => <option key={theme.id} value={theme.id}>{theme.label}</option>)}</SelectControl></label>
    <label>Mode <SelectControl aria-label="Review mode" value={state.mode} style={{background: 'var(--app-surface)', color: 'var(--app-text)', borderColor: 'var(--app-border)', borderRadius: 'var(--app-shell-control-radius)'}} onChange={event => manager.setPreference(state.themeId, event.target.value)}><option value="light">Light</option><option value="dark">Dark</option></SelectControl></label>
  </div>;
}

createRoot(document.getElementById('root')).render(<WorkspaceStyleProvider metadata={fixture.metadata} accountKey="proof" workspaceKey="proof"><NavigationReview/><AppearanceReviewControls/></WorkspaceStyleProvider>);
