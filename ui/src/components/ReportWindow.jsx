import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {Button, Callout, Spinner} from '@blueprintjs/core';
import {ReportResourceRuntime, ReportFilterBar} from 'forge/components';
import {resolveHostedExecuteOnOpenHostAction} from 'forge/components/dashboard/reportBuilderHooks.js';
import {normalizeReportDatePresets,useReportDatePresetDraft} from 'forge/reporting';
import {initializeReportFilterParameters, applyReportFilterParameters} from '../services/reportFilterParameters';
import {createReportChoiceLoader} from '../services/reportChoiceService';
import {setWorkspaceLifecycle} from '../services/useWorkspaceAdapters.js';
import {useSetting, getViewSignal, findViewSignal, getActiveUIBridgeTransportContext} from 'forge/core';

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
  const boundTransport = useRef(null);
  const resourceRef = entry.resourceRef?.uri ? entry.resourceRef : {uri: entry.windowKey};
  const identity = JSON.stringify(resourceRef);
  const bindingIdentity = JSON.stringify([identity, entry.windowId, entry.conversationId]);
  const parameterIdentity = JSON.stringify([bindingIdentity, entry.parameters || {}]);
  const historicalReplay = resolveHostedExecuteOnOpenHostAction({executeOnOpen:true,windowState:entry}) === 'restore';
  const [restoredView, setRestoredView] = useState(() => {
    const report = historicalReplay && entry.windowId ? findViewSignal(entry.windowId)?.peek()?.report : null;
    const inputs = value => value && typeof value === 'object' && !Array.isArray(value);
    if (!report || report.resourceUri !== resourceRef.uri || !inputs(report.parameters) || !inputs(report.draftParameters)) return null;
    return {parameterIdentity,parameters:report.parameters,draftParameters:report.draftParameters};
  });
  const restored = historicalReplay && restoredView?.parameterIdentity === parameterIdentity ? restoredView : null;
  // A mounted window can receive new launch inputs. Retire its replay seed
  // after the first scope change so returning to old inputs cannot reuse it.
  useEffect(() => {
    if (restoredView && !restored) setRestoredView(null);
  }, [restoredView, restored]);
  useEffect(() => {
    let active = true;
    setState({});
    Promise.resolve().then(() => {
      if (!isReportWindow(entry) || typeof store?.getReport !== 'function') throw new Error('Report reading is unavailable.');
      return store.getReport({resource: resourceRef, ...(entry.windowId ? {windowInstance: {windowId: entry.windowId, conversationId: entry.conversationId}} : {})});
    }).then(report => {
      if (!active) return;
      boundTransport.current = getActiveUIBridgeTransportContext();
      setState({report, identity:bindingIdentity});
    }).catch(error => { if (active) setState({error, identity:bindingIdentity}); });
    return () => { active = false; };
  }, [bindingIdentity, store, attempt]);
  useEffect(() => {
    if (state.identity !== bindingIdentity || !state.report || !entry.windowId) return;
    let active=true, pending=false;
    const rebind=async event=>{
      if(event?.detail?.reason!=='transport-ready' || pending)return;
      const current=getActiveUIBridgeTransportContext(), prior=boundTransport.current;
      if(!current?.sessionId || !prior || current.clientId!==prior.clientId || current.url!==prior.url || current.sessionId===prior.sessionId)return;
      pending=true;
      try {
        // Reuse the authenticated loader for the exact mounted report. Keep
        // local data, applied filters and unsaved edits intact on reconnect.
        const admitted = await store.getReport({resource:resourceRef,windowInstance:{windowId:entry.windowId,conversationId:entry.conversationId}});
        if(active){
          boundTransport.current=current;
          setState(previous=>({...previous,report:{...previous.report,resource:admitted.resource},error:null}));
        }
      } catch(error) {
        if(active)setState(previous=>({...previous,error}));
      } finally {pending=false;}
    };
    globalThis.addEventListener?.('forge:retry-window-metadata',rebind);
    return ()=>{active=false;globalThis.removeEventListener?.('forge:retry-window-metadata',rebind);};
  },[state.report,state.identity,bindingIdentity,store]);
  useEffect(() => {
    if (state.identity !== bindingIdentity) return;
    if (state.error) setWorkspaceLifecycle(entry.windowId, 'failed');
    else if (state.report) setWorkspaceLifecycle(entry.windowId, 'ready');
  }, [entry.windowId, bindingIdentity, state]);
  const publishReportView=useCallback(value=>{
    if(!entry.windowId)return;
    const signal=getViewSignal(entry.windowId),previous=signal.peek() || {};
    const report={...(previous.report?.resourceUri===resourceRef.uri?previous.report:{}),...value,resourceUri:resourceRef.uri};
    if(JSON.stringify(previous.report)!==JSON.stringify(report))signal.value={...previous,report};
  },[entry.windowId,identity]);
  useEffect(()=>()=>{const signal=entry.windowId&&findViewSignal(entry.windowId);if(signal?.peek()?.report?.resourceUri===resourceRef.uri){const next={...signal.peek()};delete next.report;signal.value=next;}},[entry.windowId,identity]);
  if (state.identity === bindingIdentity && state.error) return <Callout intent="danger" title="Report unavailable">This report could not be loaded. Please try again. <Button onClick={() => setAttempt(value => value + 1)}>Retry</Button></Callout>;
  if (state.identity !== bindingIdentity || !state.report) return <Spinner aria-label="Loading report" size={24}/>;
  // Seed the calendar draft only after authorization, with the full inputs for
  // this scope. Parameter changes do not require another definition read.
  return <AdmittedReportWindow key={parameterIdentity} entry={entry} report={state.report} services={services}
    restored={restored} historicalReplay={historicalReplay} publishReportView={publishReportView}/>;
}

function AdmittedReportWindow({entry, report, services, restored, historicalReplay, publishReportView}) {
  const [editedParameters, setEditedParameters] = useState({});
  const [applied, setApplied] = useState(null);
  const [runTrigger, setRunTrigger] = useState(0);
  const initialParameters = useMemo(() => {
    const {executeOnOpen, ...inputs} = entry.parameters || {};
    return initializeReportFilterParameters(report.resourceDefinition, inputs);
  }, [entry.parameters, report.resourceDefinition]);
  const choiceLoader=useMemo(()=>report.resource ? createReportChoiceLoader(report.resource) : null,[JSON.stringify(report.resource),services.reportStore]);
  const loadCalendar=useCallback((id,inputs,signal)=>{
    if(!choiceLoader)throw Error('Calendar metadata is unavailable.');
    return choiceLoader(id,inputs,{signal});
  },[choiceLoader]);
  const declaredFields=reportParameterFields(report);
  const dates=useReportDatePresetDraft({ranges:declaredFields.filter(field=>field.kind==='dateRange'),
    parameters:restored?.draftParameters ?? initialParameters,resetParameters:initialParameters,loadCalendar,
    onDefaultsReady:values=>{
      // A calendar response can complete an unfinished draft on replay. Only
      // Apply may replace the report's last applied filters in that case.
      if (!restored) setApplied(applyReportFilterParameters(values,declaredFields));
      return values;
    }});
  const parameters=dates.draft;
  const reportContext={identity:{windowId:entry.windowId},windowState:{...entry,windowForm:parameters},handlers:services};
  const setParameter=(key,value,raw)=>{
    dates.editField(key);dates.writeDraft({...dates.draftRef.current,[key]:value});
    setEditedParameters(previous=>({...previous,[key]:raw}));
  };
  const loadChoices=async(id,inputs,options)=>{
    const response=await choiceLoader(id,inputs,options);
    const binding=report.resourceDefinition?.metadata?.previewChoices?.find(value=>value.dataSourceRef===id);
    const rows=response?.data;
    if(!Array.isArray(rows))throw Error('Choice provider returned an invalid response.');
    const named=rows.find(row=>row?.name===binding?.parameter&&Array.isArray(row.choices));
    return {choices:named?named.choices:rows};
  };
  const appliedParameters=applied ?? restored?.parameters ?? initialParameters;
  const fields=declaredFields.map(field=>dates.controls.find(range=>range.id===field.id)||field);
  const missing=missingReportInputs(fields,parameters);
  const appliedMissing=missingReportInputs(fields,appliedParameters);
  const executeOnOpen=typeof entry.parameters?.executeOnOpen==='boolean'
    ? entry.parameters.executeOnOpen : report.resourceDefinition?.metadata?.executeOnOpen===true;
  const reportTitle=report.resourceDefinition?.reportDocument?.title
    || report.resourceDefinition?.reportSpec?.title
    || report.title || entry.windowTitle;
  return <div className="forge-report-filter-layout">
    <ReportFilterBar context={reportContext} controls={fields} values={{...parameters,...Object.fromEntries(fields.filter(field=>field.multiple&&field.kind==='text'&&editedParameters[field.id]!==undefined).map(field=>[field.id,editedParameters[field.id]]))}} scopeKey={JSON.stringify(report.resource)} loadChoices={loadChoices}
      onChange={(id,value)=>{const field=fields.find(field=>field.id===id);setParameter(id,field?.multiple&&field.kind!=='choice'&&typeof value==='string'?reportParameterValue(value,field):value,value);}}
      onDatePreset={dates.requestPreset} onDateRange={dates.manualRange} onDateDraft={dates.cancelDraftRequests}
      onApply={()=>{if(!missing.length&&!dates.dateBlocked){setApplied(applyReportFilterParameters(parameters,fields));setRunTrigger(value=>value+1);}}}
      onReset={()=>{dates.reset();setEditedParameters({});}}
      applyDisabled={missing.length>0||dates.dateBlocked} placement={report.resourceDefinition?.builderDefinition?.reportBuilder?.filterPresentation||report.resourceDefinition?.metadata?.filterPlacement||'top'}/>
    {missing.length>0&&<Callout intent="warning" title="Required report inputs">Complete these inputs before running the report: {missing.map(field=>field.label).join(', ')}.</Callout>}
    <ReportResourceRuntime onViewStateChange={publishReportView} container={{parameters:{reportResource:report.resource,reportTitle,
      reportCapabilities:{run:appliedMissing.length===0,export:true,tableBlockSelection:true,workbookExport:true},prefill:fields.length||restored?appliedParameters:parameters,
      showRunControl:fields.length===0,executeOnOpen:appliedMissing.length===0&&(!historicalReplay&&executeOnOpen||runTrigger>0),runTrigger}}} context={reportContext}/>
  </div>;
}
