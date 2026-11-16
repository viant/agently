import React from 'react';
import WorkspaceSettingsDestination,{isWorkspaceSettingsWindow} from './WorkspaceSettingsDestination';
import PersonalWorkspaceTiles from './PersonalWorkspaceTiles';
import {Typography, WindowManager, Icon, typographyProps} from 'forge/components';
import {activeWindows} from 'forge/core';
import {useSignals} from '@preact/signals-react/runtime';
import {sectionWindowIds} from './sectionWindowTree.js';
import ReportWindow, {isReportWindow} from './ReportWindow.jsx';
import ConversationAuditWindow, {isConversationAuditWindow} from './ConversationAuditDestination.jsx';

// App destinations share the visible section pane. Conversation-owned outputs
// keep their separate retained viewer and do not select the primary route.
export default function AppSectionContent({window: entry, title = 'Workspace', error = '', headerMount, overview}) {
  useSignals();
  const owned = sectionWindowIds(activeWindows.value, entry?.windowId);
  return <section className="app-section-page" data-agently-presentation="legacy" aria-label={title}>
    {entry ? <WindowManager includeHosted workspaceRuntime={headerMount ? {
      tabMount:headerMount, tabListClassName:'app-shell-viewer-tabs', tabItemClassName:'app-shell-viewer-tab',
      renderTabLabel:window=><><Icon name="lucide:file-text"/><span {...typographyProps('label')}>{window.windowTitle || window.windowKey}</span></>,
    } : {}} windowFilter={window => owned.has(window.windowId)}
      renderWindowContent={({window, defaultContent}) => isWorkspaceSettingsWindow(window) ? <WorkspaceSettingsDestination key={window.windowId}/> : isConversationAuditWindow(window)
        ? <ConversationAuditWindow key={window.windowId}/>
        : isReportWindow(window) ? <ReportWindow key={window.windowId} window={window}/> : defaultContent}/>
      : overview ? <PersonalWorkspaceTiles title={overview.title} groups={overview.groups} onSelect={overview.onSelect} onManage={overview.onManage}/> : <div className="app-section-empty">
        <Typography as="h1" typeRole="heading">{error ? 'Destination unavailable' : title}</Typography>
        <Typography as="p" typeRole="body" role={error ? 'alert' : undefined}>{error || 'Choose a destination from the menu.'}</Typography>
      </div>}
  </section>;
}
