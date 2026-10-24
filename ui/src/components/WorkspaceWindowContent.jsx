import React from 'react';
import {WindowContent} from 'forge/components';
import ReportWindow, {isReportWindow} from './ReportWindow.jsx';

export default function WorkspaceWindowContent(props) {
  return isReportWindow(props.window) ? <ReportWindow window={props.window}/> : <WindowContent {...props}/>;
}
