import React, {useCallback, useEffect, useMemo, useState} from 'react';
import {Button, Callout, Spinner} from '@blueprintjs/core';
import {ReportResourceRuntime, ReportFilterBar} from 'forge/components';
import {normalizeReportDatePresets,useReportDatePresetDraft} from 'forge/reporting';
import {initializeReportFilterParameters, applyReportFilterParameters} from '../services/reportFilterParameters';
import {createReportChoiceLoader} from '../services/reportChoiceService';
import {setWorkspaceLifecycle} from '../services/useWorkspaceAdapters.js';
import {useSetting, getViewSignal, findViewSignal} from 'forge/core';

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
    if(predicate.kind==='dateRange'){
      const range={id,label,kind:'dateRange',required:predicate.required,
        start:parameterKey(predicate.prefill?.start,predicate.startParamPath),end:parameterKey(predicate.prefill?.end,predicate.endParamPath)};
      if(Object.hasOwn(predicate,'datePresets')){
        range.requiresExplicitDates=true;
        try{range.datePresets=normalizeReportDatePresets(predicate.datePresets,definition.dataSources);}
        catch(error){range.datePresetError=error.message;}
      }
      return [range];
    }
    const binding=bindings.find(value=>value.parameter===id);
    if(binding && definition.dataSources?.[binding.dataSourceRef])return [{...predicate,...binding,id,label,kind:'choice',dataSourceId:binding.dataSourceRef}];
    if(Array.isArray(predicate.options)&&predicate.options.length&&!predicate.multiple)return [{...predicate,id,label,kind:'select',choices:predicate.options.map(value=>typeof value==='object'?value:{value,label:String(value)})}];
    if(predicate.manualEntry&&predicate.multiple&&predicate.manualValueType==='string')return [{...predicate,id,label,kind:'list'}];
    return [{...predicate,id,label,kind:['number','boolean'].includes(predicate.kind)?predicate.kind:'text'}];
  });
}

function missingReportInputs(fields, values) {
  return fields.filter(field=>(field.required||field.requiresExplicitDates) && (field.kind==='dateRange'
    ? !values[field.start]||!values[field.end]
    : values[field.id]==null || values[field.id]==='' || Array.isArray(values[field.id])&&!values[field.id].length));
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
  const initialParameters = useMemo(() => {const {executeOnOpen, ...inputs}=entry.parameters || {}; return initializeReportFilterParameters(state.report?.resourceDefinition, inputs);}, [state.report?.resourceDefinition, parameterIdentity]);
  useEffect(() => {
    let active = true;
    setState({});
    Promise.resolve().then(() => {
      if (!isReportWindow(entry) || typeof store?.getReport !== 'function') throw new Error('Report reading is unavailable.');
      return store.getReport({resource: resourceRef, ...(entry.windowId ? {windowInstance: {windowId: entry.windowId, conversationId: entry.conversationId}} : {})});
    }).then(report => {
      if (!active) return;
      setState({report, identity});
    }).catch(error => { if (active) setState({error, identity}); });
    return () => { active = false; };
  }, [identity, store, attempt]);
  useEffect(() => {
    if (state.identity !== identity) return;
    if (state.error) setWorkspaceLifecycle(entry.windowId, 'failed');
    else if (state.report) setWorkspaceLifecycle(entry.windowId, 'ready');
  }, [entry.windowId, identity, state]);
  const choiceLoader=useMemo(()=>state.report?.resource ? createReportChoiceLoader(state.report.resource) : null,[JSON.stringify(state.report?.resource),store]);
  const loadCalendar=useCallback((id,inputs,signal)=>{
    if(!choiceLoader)throw Error('Calendar metadata is unavailable.');
    return choiceLoader(id,inputs,{signal});
  },[choiceLoader]);
  const declaredFields=reportParameterFields(state.report);
  const dates=useReportDatePresetDraft({ranges:declaredFields.filter(field=>field.kind==='dateRange'),
    parameters:initialParameters,resetParameters:initialParameters,contextKey:parameterIdentity,loadCalendar,
    onDefaultsReady:values=>{setApplied({identity:parameterIdentity,values:applyReportFilterParameters(values,declaredFields)});return values;}});
  const parameters=dates.draft;
  const reportContext={identity:{windowId:entry.windowId},windowState:{...entry,windowForm:parameters},handlers:services};
  const setParameter=(key,value,raw)=>{
    dates.editField(key);dates.writeDraft({...dates.draftRef.current,[key]:value});
    setEditedParameters(previous=>({identity:parameterIdentity,text:{...(previous?.identity===parameterIdentity?previous.text:{}),[key]:raw}}));
  };
  const loadChoices=async(id,inputs,options)=>{
    const response=await choiceLoader(id,inputs,options);
    const binding=state.report.resourceDefinition?.metadata?.previewChoices?.find(value=>value.dataSourceRef===id);
    const rows=response?.data;
    if(!Array.isArray(rows))throw Error('Choice provider returned an invalid response.');
    const named=rows.find(row=>row?.name===binding?.parameter&&Array.isArray(row.choices));
    return {choices:named?named.choices:rows};
  };
  const appliedParameters=applied?.identity===parameterIdentity?applied.values:initialParameters;
  const publishReportView=useCallback(value=>{
    if(!entry.windowId)return;
    const signal=getViewSignal(entry.windowId),previous=signal.peek() || {};
    const report={...(previous.report?.resourceUri===resourceRef.uri?previous.report:{}),...value,resourceUri:resourceRef.uri};
    if(JSON.stringify(previous.report)!==JSON.stringify(report))signal.value={...previous,report};
  },[entry.windowId,identity]);
  useEffect(()=>{publishReportView({parameters:appliedParameters});},[publishReportView,appliedParameters]);
  useEffect(()=>()=>{const signal=entry.windowId&&findViewSignal(entry.windowId);if(signal?.peek()?.report?.resourceUri===resourceRef.uri){const next={...signal.peek()};delete next.report;signal.value=next;}},[entry.windowId,identity]);
  if (state.identity === identity && state.error) return <Callout intent="danger" title="Report unavailable">This report could not be loaded. Please try again. <Button onClick={() => setAttempt(value => value + 1)}>Retry</Button></Callout>;
  if (state.identity !== identity || !state.report) return <Spinner aria-label="Loading report" size={24}/>;
  const fields=declaredFields.map(field=>dates.controls.find(range=>range.id===field.id)||field);
  const missing=missingReportInputs(fields,parameters);
  const appliedMissing=missingReportInputs(fields,appliedParameters);
  const executeOnOpen=typeof entry.parameters?.executeOnOpen==='boolean'
    ? entry.parameters.executeOnOpen : state.report.resourceDefinition?.metadata?.executeOnOpen===true;
  const reportTitle=state.report.resourceDefinition?.reportDocument?.title
    || state.report.resourceDefinition?.reportSpec?.title
    || state.report.title || entry.windowTitle;
  return <div className="forge-report-filter-layout">
    <ReportFilterBar context={reportContext} controls={fields} values={{...parameters,...Object.fromEntries(fields.filter(field=>field.multiple&&field.kind==='text'&&editedParameters?.identity===parameterIdentity&&editedParameters.text?.[field.id]!==undefined).map(field=>[field.id,editedParameters.text[field.id]]))}} scopeKey={JSON.stringify(state.report.resource)} loadChoices={loadChoices}
      onChange={(id,value)=>{const field=fields.find(field=>field.id===id);setParameter(id,field?.multiple&&field.kind!=='choice'&&typeof value==='string'?reportParameterValue(value,field):value,value);}}
      onDatePreset={dates.requestPreset} onDateRange={dates.manualRange} onDateDraft={dates.cancelDraftRequests}
      onApply={()=>{if(!missing.length&&!dates.dateBlocked){setApplied({identity:parameterIdentity,values:applyReportFilterParameters(parameters,fields)});setRunTrigger(value=>value+1);}}}
      onReset={()=>{dates.reset();setEditedParameters(null);}}
      applyDisabled={missing.length>0||dates.dateBlocked} placement={state.report.resourceDefinition?.builderDefinition?.reportBuilder?.filterPresentation||state.report.resourceDefinition?.metadata?.filterPlacement||'top'}/>
    {missing.length>0&&<Callout intent="warning" title="Required report inputs">Complete these inputs before running the report: {missing.map(field=>field.label).join(', ')}.</Callout>}
    <ReportResourceRuntime onViewStateChange={publishReportView} container={{parameters:{reportResource:state.report.resource,reportTitle,
      reportCapabilities:{run:appliedMissing.length===0,export:true,tableBlockSelection:true,workbookExport:true},prefill:fields.length?appliedParameters:parameters,
      showRunControl:fields.length===0,executeOnOpen:appliedMissing.length===0&&(executeOnOpen||applied?.identity===parameterIdentity&&runTrigger>0),runTrigger:applied?.identity===parameterIdentity?runTrigger:0}}} context={reportContext}/>
  </div>;
}
