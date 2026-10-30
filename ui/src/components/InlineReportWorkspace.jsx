import React, {lazy, Suspense, useEffect, useState} from 'react';
import {getWindowContext} from 'forge/core/context/registry.js';
import {useChatProjection} from '../services/chatStore.js';
import {getReportRestoreScope} from '../services/agentlyClient.js';
import {MAIN_CHAT_WINDOW_ID} from '../services/conversationWindow.js';
import {normalizeCanonicalReportAssembly} from '../services/workspaceAdapters.js';
import {compareReportVersions, readWorkspaceReport, selectCanonicalReport} from '../services/workspaceReportSource.js';
import {AssetLoadBoundary} from './ApplicationRecovery.jsx';
import WorkspaceOutputState from './WorkspaceOutputState.jsx';

const Report = lazy(() => import('./chat/RichContent.jsx').then(module => ({default: module.HostedInlineReport})));

export default function InlineReportWorkspace({entry, presentation, onLifecycle, onReturn}) {
  const rows = useChatProjection(entry.conversationId);
  const [attempt, setAttempt] = useState(0);
  const [load, setLoad] = useState({});
  useEffect(() => {
    let active = true;
    setLoad({});
    readWorkspaceReport(entry).then(result => {if (active) setLoad({...result, windowId: entry.windowId});})
      .catch(error => {if (active) setLoad({error, windowId: entry.windowId});});
    return () => {active = false;};
  }, [entry.conversationId, entry.windowId, attempt]);
  const current = load.windowId === entry.windowId;
  const loading = !current || (!load.selected && !load.error);
  const validScope = load.scope === getReportRestoreScope();
  const selected = current && validScope && !load.error ? [load.selected, selectCanonicalReport(rows, entry.workspaceObject.content)]
    .filter(Boolean).sort(compareReportVersions)[0] : null;
  const error = load.error || (!loading && !validScope ? new Error('Report account changed.') : null);
  useEffect(() => {if (loading || error) onLifecycle?.(loading ? 'opening' : 'failed');}, [loading, !!error, onLifecycle]);
  const retry = () => setAttempt(value => value + 1);
  if (loading || error) return <WorkspaceOutputState loading={loading} error={error} onRetry={retry} onReturn={onReturn}/>;
  return <AssetLoadBoundary><Suspense fallback={<WorkspaceOutputState loading/>}>
    <Report onLifecycle={onLifecycle} onRetry={retry} onReturn={onReturn} assembly={normalizeCanonicalReportAssembly(selected.assembly, presentation)} diagnostics={selected.renderedContent.diagnostics || []}
      conversationId={entry.conversationId} messageId={selected.messageId} context={getWindowContext(MAIN_CHAT_WINDOW_ID)}/>
  </Suspense></AssetLoadBoundary>;
}
