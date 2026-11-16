import React from 'react';
import WorkspaceSettingsDestination,{isWorkspaceSettingsWindow} from './WorkspaceSettingsDestination';
import {WindowContent} from 'forge/components';
import ReportWindow, {isReportWindow} from './ReportWindow.jsx';

export default function WorkspaceWindowContent(props) {
  return isWorkspaceSettingsWindow(props.window) ? <WorkspaceSettingsDestination/> : isReportWindow(props.window) ? <ReportWindow window={props.window}/> : <WindowContent {...props}/>;
}
