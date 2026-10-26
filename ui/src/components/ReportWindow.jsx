import React, {useEffect, useState} from 'react';
import {Button, Callout, Spinner} from '@blueprintjs/core';
import {ReportResourceRuntime} from 'forge/components';
import {useSetting} from 'forge/core';

export function isReportWindow(window = {}) {
  return String(window.windowKey || '').startsWith('report://')
    && window.resourceRef?.uri === window.windowKey;
}

function parameterKey(path, fallback) {
  return String(path || fallback || '').replace(/^filters\./, '');
}

function reportParameterFields(report) {
  const predicates = report?.resourceDefinition?.builderDefinition?.reportBuilder?.predicates;
  return (Array.isArray(predicates) ? predicates : []).flatMap(predicate => {
    const label = predicate.label || predicate.id;
    if (predicate.kind === 'dateRange') return [
      {key: parameterKey(predicate.startParamPath, predicate.prefill?.start), label: `${label} from`, kind: 'date'},
      {key: parameterKey(predicate.endParamPath, predicate.prefill?.end), label: `${label} to`, kind: 'date'},
    ].filter(field => field.key);
    const key = parameterKey(predicate.paramPath, predicate.id);
    return key ? [{key, label, kind: predicate.kind, multiple: predicate.multiple, required: predicate.required}] : [];
  });
}

function reportParameterValue(raw, field) {
  if (field.kind === 'boolean') return raw;
  if (raw === '') return field.multiple || field.kind === 'multiSelect' ? [] : '';
  if (field.multiple || field.kind === 'multiSelect') return raw.split(',').map(value => value.trim()).filter(value => value !== '')
    .map(value => /^-?\d+$/.test(value) && Number.isSafeInteger(Number(value)) ? Number(value) : value);
  return field.kind === 'number' ? Number(raw) : raw;
}

// The shared report runtime owns execution, refinement and exports. The host
// resolves the selected resource and collects its declared input parameters.
export default function ReportWindow({window: entry}) {
  const {services = {}} = useSetting();
  const store = services.reportStore;
  const [state, setState] = useState({});
  const [attempt, setAttempt] = useState(0);
  const [editedParameters, setEditedParameters] = useState(null);
  const identity = JSON.stringify(entry.resourceRef);
  const parameterIdentity = JSON.stringify([identity, entry.parameters || {}]);
  const parameters = editedParameters?.identity === parameterIdentity ? editedParameters.values : entry.parameters || {};
  const setParameter = (key, value, raw) => setEditedParameters(previous => ({
    identity: parameterIdentity,
    values: {...(previous?.identity === parameterIdentity ? previous.values : entry.parameters || {}), [key]: value},
    text: {...(previous?.identity === parameterIdentity ? previous.text : {}), [key]: raw},
  }));
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
  if (state.identity === identity && state.error) return <Callout intent="danger" title="Report unavailable">This report could not be loaded. Please try again. <Button onClick={() => setAttempt(value => value + 1)}>Retry</Button></Callout>;
  if (state.identity !== identity || !state.report) return <Spinner aria-label="Loading report" size={24}/>;
  const fields = reportParameterFields(state.report);
  return <>
    {fields.length > 0 && <fieldset style={{border: 0, margin: 0, padding: 16}}>
      <legend>Report parameters</legend>
      <div style={{display: 'flex', flexWrap: 'wrap', gap: 12}}>
        {fields.map(field => <label key={field.key} style={{display: 'flex', flexDirection: 'column', gap: 4}}>
          <span>{field.label}{field.required ? ' *' : ''}</span>
          <input aria-label={field.label} className="bp6-input" type={field.kind === 'boolean' ? 'checkbox' : field.kind === 'number' ? 'number' : field.kind === 'date' ? 'date' : 'text'}
            {...(field.kind === 'boolean' ? {checked: parameters[field.key] ?? false} : {value: editedParameters?.identity === parameterIdentity && Object.hasOwn(editedParameters.text, field.key)
              ? editedParameters.text[field.key] : Array.isArray(parameters[field.key]) ? parameters[field.key].join(', ') : parameters[field.key] ?? ''})}
            placeholder={field.multiple || field.kind === 'multiSelect' ? 'Comma-separated values' : undefined}
            onChange={event => {
              const raw = field.kind === 'boolean' ? event.target.checked : event.target.value;
              setParameter(field.key, reportParameterValue(raw, field), raw);
            }}/>
        </label>)}
      </div>
    </fieldset>}
    <ReportResourceRuntime container={{parameters: {
      reportResource: state.report.resource,
      reportTitle: state.report.title || entry.windowTitle,
      reportCapabilities: {run: true, export: true},
      prefill: parameters,
    }}} context={{handlers: services}}/>
  </>;
}
