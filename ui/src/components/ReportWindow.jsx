import React, {useEffect, useState} from 'react';
import {Button, Callout, Spinner} from '@blueprintjs/core';
import {ReportResourceRuntime} from 'forge/components';
import {useSetting} from 'forge/core';

export function isReportWindow(window = {}) {
  return String(window.windowKey || '').startsWith('report://')
    && window.resourceRef?.uri === window.windowKey;
}

// The shared report runtime owns execution, refinement and exports. The host
// only resolves the selected resource using its existing reporting service.
export default function ReportWindow({window: entry}) {
  const {services = {}} = useSetting();
  const store = services.reportStore;
  const [state, setState] = useState({});
  const [attempt, setAttempt] = useState(0);
  const identity = JSON.stringify(entry.resourceRef);
  useEffect(() => {
    let active = true;
    setState({});
    Promise.resolve().then(() => {
      if (!isReportWindow(entry) || typeof store?.getReport !== 'function') throw new Error('Report reading is unavailable.');
      return store.getReport({resource: entry.resourceRef});
    }).then(report => {
      if (!active) return;
      setState({report, identity});
    }).catch(error => { if (active) setState({error, identity}); });
    return () => { active = false; };
  }, [identity, store, attempt]);
  if (state.identity === identity && state.error) return <Callout intent="danger" title="Report unavailable">{state.error.message} <Button onClick={() => setAttempt(value => value + 1)}>Retry</Button></Callout>;
  if (state.identity !== identity || !state.report) return <Spinner aria-label="Loading report" size={24}/>;
  return <ReportResourceRuntime container={{parameters: {
    reportResource: state.report.resource,
    reportTitle: state.report.title || entry.windowTitle,
    reportCapabilities: {run: true, export: true},
    prefill: entry.parameters || {},
  }}} context={{handlers: services}}/>;
}
