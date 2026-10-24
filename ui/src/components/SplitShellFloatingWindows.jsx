import React from 'react';
import {WindowManager} from 'forge/components';
import ReportWindow, {isReportWindow} from './ReportWindow.jsx';

// Split chat/workspace panes own their tabs; ordinary floating children keep
// the same Forge store, metadata loader and selection/close lifecycle.
export default function SplitShellFloatingWindows() {
  return <div className="split-shell-floating-windows" style={{position:'absolute',inset:0,display:'flex',flexDirection:'column',pointerEvents:'none'}}>
    <style>{'.split-shell-floating-windows .react-draggable { pointer-events: auto; }'}</style>
    <WindowManager windowFilter={entry => entry?.inTab === false && !entry?.windowScopeId}
      renderWindowContent={({window, defaultContent}) => isReportWindow(window) ? <ReportWindow window={window}/> : defaultContent}/>
  </div>;
}
