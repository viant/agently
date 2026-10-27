import React, {useEffect, useMemo, useState} from 'react';
import {Button, Callout, Spinner} from '@blueprintjs/core';
import {ReportResourceRuntime, ReportFilterBar} from 'forge/components';
import {initializeReportFilterParameters} from '../services/reportFilterParameters';
import {createReportChoiceLoader} from '../services/reportChoiceService';
import {useSetting} from 'forge/core';

export function isReportWindow(window = {}) {
  return String(window.windowKey || '').startsWith('report://')
    && (!window.resourceRef?.uri || window.resourceRef.uri === window.windowKey);
}

function parameterKey(path, fallback) {
  return String(path || fallback || '').replace(/^filters\./, '');
}

function reportParameterFields(report) {
  const definition=report?.resourceDefinition;
  const predicates=definition?.builderDefinition?.reportBuilder?.predicates;
  const bindings=definition?.metadata?.previewChoices || [];
  return (Array.isArray(predicates)?predicates:[]).flatMap(predicate=>{
    const id=String(predicate.id||'').trim(),label=predicate.label||id;
    if(!id)return [];
    if(predicate.kind==='dateRange')return [{id,label,kind:'dateRange',required:predicate.required,
      start:parameterKey(predicate.prefill?.start,predicate.startParamPath),end:parameterKey(predicate.prefill?.end,predicate.endParamPath)}];
    const binding=bindings.find(value=>value.parameter===id);
    if(binding && definition.dataSources?.[binding.dataSourceRef])return [{...predicate,...binding,id,label,kind:'choice',dataSourceId:binding.dataSourceRef}];
    if(Array.isArray(predicate.options)&&predicate.options.length&&!predicate.multiple)return [{...predicate,id,label,kind:'select',choices:predicate.options.map(value=>typeof value==='object'?value:{value,label:String(value)})}];
    if(predicate.manualEntry&&predicate.multiple&&predicate.manualValueType==='string')return [{...predicate,id,label,kind:'list'}];
    return [{...predicate,id,label,kind:['number','boolean'].includes(predicate.kind)?predicate.kind:'text'}];
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
  const [applied, setApplied] = useState(null);
  const [runTrigger, setRunTrigger] = useState(0);
  const resourceRef = entry.resourceRef?.uri ? entry.resourceRef : {uri: entry.windowKey};
  const identity = JSON.stringify(resourceRef);
  const parameterIdentity = JSON.stringify([identity, entry.parameters || {}]);
  const initialParameters = useMemo(() => initializeReportFilterParameters(state.report?.resourceDefinition, entry.parameters || {}), [state.report?.resourceDefinition, parameterIdentity]);
  const parameters = editedParameters?.identity === parameterIdentity ? editedParameters.values : initialParameters;
  const setParameter = (key, value, raw) => setEditedParameters(previous => ({
    identity: parameterIdentity,
    values: {...(previous?.identity === parameterIdentity ? previous.values : initialParameters), [key]: value},
    text: {...(previous?.identity === parameterIdentity ? previous.text : {}), [key]: raw},
  }));
  useEffect(() => {
    let active = true;
    setState({});
    Promise.resolve().then(() => {
      if (!isReportWindow(entry) || typeof store?.getReport !== 'function') throw new Error('Report reading is unavailable.');
      return store.getReport({resource: resourceRef});
    }).then(report => {
      if (!active) return;
      setState({report, identity});
    }).catch(error => { if (active) setState({error, identity}); });
    return () => { active = false; };
  }, [identity, store, attempt]);
  const choiceLoader=useMemo(()=>state.report?.resource ? createReportChoiceLoader(state.report.resource) : null,[state.report?.resource?.uri,state.report?.resource?.providerIdentity]);
  const loadChoices=async(id,inputs,options)=>{
    const response=await choiceLoader(id,inputs,options);
    const binding=state.report.resourceDefinition?.metadata?.previewChoices?.find(value=>value.dataSourceRef===id);
    const rows=response?.data;
    if(!Array.isArray(rows))throw Error('Choice provider returned an invalid response.');
    const named=rows.find(row=>row?.name===binding?.parameter&&Array.isArray(row.choices));
    return {choices:named?named.choices:rows};
  };
  if (state.identity === identity && state.error) return <Callout intent="danger" title="Report unavailable">This report could not be loaded. Please try again. <Button onClick={() => setAttempt(value => value + 1)}>Retry</Button></Callout>;
  if (state.identity !== identity || !state.report) return <Spinner aria-label="Loading report" size={24}/>;
  const fields=reportParameterFields(state.report);
  const missing=fields.filter(field=>field.required && (field.kind==='dateRange'
    ? !parameters[field.start]||!parameters[field.end]
    : parameters[field.id]==null || parameters[field.id]==='' || Array.isArray(parameters[field.id])&&!parameters[field.id].length));
  const appliedParameters=applied?.identity===parameterIdentity?applied.values:initialParameters;
  return <div className="forge-report-filter-layout">
    <ReportFilterBar controls={fields} values={{...parameters,...Object.fromEntries(fields.filter(field=>field.multiple&&field.kind==='text'&&editedParameters?.identity===parameterIdentity&&editedParameters.text?.[field.id]!==undefined).map(field=>[field.id,editedParameters.text[field.id]]))}} scopeKey={JSON.stringify(state.report.resource)} loadChoices={loadChoices}
      onChange={(id,value)=>{const field=fields.find(field=>field.id===id);setParameter(id,field?.multiple&&field.kind!=='choice'&&typeof value==='string'?reportParameterValue(value,field):value,value);}}
      onApply={()=>{if(!missing.length){setApplied({identity:parameterIdentity,values:{...parameters}});setRunTrigger(value=>value+1);}}}
      onReset={()=>setEditedParameters({identity:parameterIdentity,values:{...initialParameters}})}
      applyDisabled={missing.length>0} placement={state.report.resourceDefinition?.builderDefinition?.reportBuilder?.filterPresentation||state.report.resourceDefinition?.metadata?.filterPlacement||'top'}/>
    {missing.length>0&&<Callout intent="warning" title="Required report inputs">Complete these inputs before running the report: {missing.map(field=>field.label).join(', ')}.</Callout>}
    <ReportResourceRuntime container={{parameters:{reportResource:state.report.resource,reportTitle:state.report.title||entry.windowTitle,
      reportCapabilities:{run:missing.length===0,export:true,tableBlockSelection:true,workbookExport:true},prefill:fields.length?appliedParameters:parameters,
      showRunControl:fields.length===0,executeOnOpen:applied?.identity===parameterIdentity&&runTrigger>0,runTrigger:applied?.identity===parameterIdentity?runTrigger:0}}} context={{handlers:services}}/>
  </div>;
}
