import React from 'react';
import {Typography, WindowContent} from 'forge/components';
import ReportWindow, {isReportWindow} from './ReportWindow.jsx';

// App destinations share the visible section pane. Conversation-owned outputs
// keep their separate retained viewer and do not select the primary route.
export default function AppSectionContent({window: entry, title = 'Workspace', error = ''}) {
  return <section className="app-section-page" data-agently-presentation="legacy" aria-label={title}>
    {entry ? isReportWindow(entry)
      ? <ReportWindow key={entry.windowId} window={entry}/>
      : <WindowContent key={entry.windowId} window={entry} isInTab/>
      : <div className="app-section-empty">
        <Typography as="h1" typeRole="heading">{error ? 'Destination unavailable' : title}</Typography>
        <Typography as="p" typeRole="body" role={error ? 'alert' : undefined}>{error || 'Choose a destination from the menu.'}</Typography>
      </div>}
  </section>;
}
