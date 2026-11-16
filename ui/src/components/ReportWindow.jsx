import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import isEqual from 'lodash/isEqual.js';
import {sha256} from '@noble/hashes/sha2.js';
import {bytesToHex} from '@noble/hashes/utils.js';
import {Button, Callout, Checkbox, Spinner} from '@blueprintjs/core';
import {ReportResourceRuntime, ReportFilterBar} from 'forge/components';
import {resolveHostedExecuteOnOpenHostAction} from 'forge/components/dashboard/reportBuilderHooks.js';
import {reportRestoreSnapshotKey} from 'forge/components/dashboard/reportBuilderFrozenRestore.js';
import {normalizeReportDatePresets,useReportDatePresetDraft} from 'forge/reporting';
import {initializeReportFilterParameters, applyReportFilterParameters} from '../services/reportFilterParameters';
import {createReportChoiceLoader} from '../services/reportChoiceService';
import PersonalReportOrigin from './PersonalReportOrigin';
import './PersonalReportOrigin.css';
import {reopenReportWindow} from '../services/reportWindowRecovery';
import {setWorkspaceLifecycle} from '../services/useWorkspaceAdapters.js';
import {activeWindows, useSetting, getViewSignal, findViewSignal, getActiveUIBridgeTransportContext} from 'forge/core';

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
    if(predicate.lookup?.dataSource)return [{...predicate,id,label,kind:'lookup'}];
    const binding=bindings.find(value=>value.parameter===id);
    if(binding && definition.dataSources?.[binding.dataSourceRef])return [{...predicate,...binding,id,label,kind:'choice',dataSourceId:binding.dataSourceRef}];
    if(Array.isArray(predicate.options)&&predicate.options.length&&!predicate.multiple)return [{...predicate,id,label,kind:'select',choices:predicate.options.map(value=>typeof value==='object'?value:{value,label:String(value)})}];
    if(predicate.manualEntry&&predicate.multiple&&predicate.manualValueType==='string')return [{...predicate,id,label,kind:'list'}];
    return [{...predicate,id,label,kind:['number','boolean'].includes(predicate.kind)?predicate.kind:'text'}];
  });
}

function reportFilterDeclaration(report,fields=reportParameterFields(report)) {
 // View state crosses JSON persistence; prepare the same representation once.
 return JSON.parse(JSON.stringify({predicates:report?.resourceDefinition?.builderDefinition?.reportBuilder?.predicates || [],fields}));
}

function additionalLaunchInputs(parameters, fields) {
  const declared = new Set(['executeOnOpen']);
  for (const field of fields) {
    declared.add(field.id);
    if (field.kind === 'dateRange') {
      declared.add(field.start);
      declared.add(field.end);
    }
  }
  return Object.keys(parameters || {}).filter(key => !declared.has(key));
}

function SavedFilterReview({keys, onContinue}) {
  const [keep, setKeep] = useState([]);
  return <Callout intent="warning" title="Review saved filters">
    <p>These saved inputs have no matching filter on this report. Select the inputs to keep for this window.</p>
    {keys.map(key => <Checkbox key={key} label={key} checked={keep.includes(key)}
      onChange={() => setKeep(previous => previous.includes(key) ? previous.filter(value => value !== key) : [...previous,key])}/>)}
    <Button onClick={() => onContinue(keys.filter(key => !keep.includes(key)))}>Continue</Button>
  </Callout>;
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
  const expectedProvider = entry.providerIdentity;
  const identity = JSON.stringify([resourceRef,expectedProvider || '']);
  const bindingIdentity = JSON.stringify([identity, entry.windowId, entry.conversationId, entry.workspaceObject?.revision || 0, entry.workspaceObject?.lifecycle?.updatedAt || '']);
  const launchIdentity = JSON.stringify(entry.parameters || {});
  const parameterIdentity = JSON.stringify([bindingIdentity, entry.parameters || {}]);
  const previousLaunch = useRef({uri:resourceRef.uri, inputs:launchIdentity});
  const historicalReplay = resolveHostedExecuteOnOpenHostAction({executeOnOpen:true,windowState:entry}) === 'restore';
  const [restoredView, setRestoredView] = useState(() => {
    const report = historicalReplay && entry.windowId ? findViewSignal(entry.windowId)?.peek()?.report : null;
    const inputs = value => value && typeof value === 'object' && !Array.isArray(value);
    if (!report || report.resourceUri !== resourceRef.uri || !inputs(report.parameters) || !inputs(report.draftParameters)) return null;
    return {parameterIdentity,parameters:report.parameters,draftParameters:report.draftParameters,filterDeclaration:report.filterDeclaration,filterFingerprint:report.filterFingerprint};
  });
  const filterDeclaration=state.filterDeclaration;
  const filterFingerprint=state.filterFingerprint;
  const restoreCompatible = state.identity !== bindingIdentity || !state.report
    || (restoredView?.filterFingerprint ? restoredView.filterFingerprint===filterFingerprint
      : restoredView?.filterDeclaration ? isEqual(restoredView.filterDeclaration,filterDeclaration)
      : !restoredView?.reopen && state.report.resource?.kind !== 'working');
  const restored = (historicalReplay || restoredView?.reopen) && restoredView?.parameterIdentity === parameterIdentity && restoreCompatible ? restoredView : null;
  // A mounted window can receive new launch inputs. Retire its replay seed
  // after the first scope change so returning to old inputs cannot reuse it.
  useEffect(() => {
    if (restoredView && !restored) setRestoredView(null);
  }, [restoredView, restored]);
  useEffect(() => {
    let active = true;
    setWorkspaceLifecycle(entry.windowId, 'opening');
    const signal = entry.windowId && findViewSignal(entry.windowId);
    const previousReport = signal?.peek()?.report;
    const previousFingerprint = previousReport?.resourceUri === resourceRef.uri ? previousReport.filterFingerprint : undefined;
    if (previousReport) {
      const previous = signal.peek();
      const inputs = previous.report.resourceUri === resourceRef.uri ? previous.report : {};
      if (state.report && previousLaunch.current.uri === resourceRef.uri && (previousLaunch.current.inputs === launchIdentity || isEqual(JSON.parse(previousLaunch.current.inputs), entry.parameters || {}))
        && inputs.parameters && inputs.draftParameters) {
        setRestoredView({parameterIdentity, parameters:inputs.parameters, draftParameters:inputs.draftParameters, reopen:true,
          filterFingerprint:state.filterFingerprint});
      }
      // Keep only the input draft needed for explicit recovery. Prior output,
      // completion and title cannot describe the newly opening revision.
      signal.value = {...previous, report:{resourceUri:resourceRef.uri,
        parameters:inputs.parameters,draftParameters:inputs.draftParameters,filterDeclaration:inputs.filterDeclaration,filterFingerprint:inputs.filterFingerprint,
        ...(inputs.shortcutInputsPending ? {shortcutInputsPending:true} : {}),
        status:'opening',hasCompletedRun:false}};
    }
    setState({});
    Promise.resolve().then(() => {
      if (!isReportWindow(entry) || typeof store?.getReport !== 'function') throw new Error('Report reading is unavailable.');
      return store.getReport({resource: resourceRef, ...(expectedProvider ? {sourceProvider:expectedProvider} : {}), ...(entry.windowId ? {windowInstance: {windowId: entry.windowId, conversationId: entry.conversationId}} : {})});
    }).then(report => {
      if (!active) return;
      const admittedTransport = getActiveUIBridgeTransportContext();
      const declaredFields=reportParameterFields(report);
      const filterDeclaration=reportFilterDeclaration(report,declaredFields);
      const filterFingerprint=bytesToHex(sha256(new TextEncoder().encode(reportRestoreSnapshotKey(filterDeclaration))));
      boundTransport.current = admittedTransport;
      const firstShortcutOpen = previousReport?.resourceUri === resourceRef.uri && previousReport.shortcutInputsPending && !previousFingerprint;
      const reviewKeys = firstShortcutOpen || previousFingerprint && previousFingerprint !== filterFingerprint
        ? additionalLaunchInputs(entry.parameters, declaredFields) : [];
      setState({report, declaredFields, filterDeclaration, filterFingerprint, reviewKeys, identity:bindingIdentity});
    }).catch(error => { if (active) setState({error, identity:bindingIdentity}); });
    return () => { active = false; };
  }, [bindingIdentity, store, attempt]);
  useEffect(() => {previousLaunch.current={uri:resourceRef.uri, inputs:launchIdentity};},[bindingIdentity,launchIdentity]);
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
        const admitted = await store.getReport({resource:resourceRef,...(expectedProvider ? {sourceProvider:expectedProvider} : {}),windowInstance:{windowId:entry.windowId,conversationId:entry.conversationId}});
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
    const {filterDeclaration: _legacyDeclaration,shortcutInputsPending: _pending,...previousReport}=previous.report?.resourceUri===resourceRef.uri?previous.report:{};
    const report={...previousReport,...value,resourceUri:resourceRef.uri,...(value.parameters && value.draftParameters ? {filterFingerprint} : {})};
    if(JSON.stringify(previous.report)!==JSON.stringify(report))signal.value={...previous,report};
  },[entry.windowId,identity,filterFingerprint]);
  useEffect(()=>()=>{const signal=entry.windowId&&findViewSignal(entry.windowId);if(signal?.peek()?.report?.resourceUri===resourceRef.uri){const next={...signal.peek()};delete next.report;signal.value=next;}},[entry.windowId,identity]);
  if (state.identity === bindingIdentity && state.error) return <Callout intent="danger" title="Report unavailable">{state.error.status === 409
    ? <>This report revision changed. Reopen it in a new window to keep this window’s revision intact. <Button onClick={() => reopenReportWindow(entry)}>Reopen report</Button></>
    : <>{expectedProvider && [403,404,410].includes(state.error.status) ? 'This report is unavailable from its saved publisher.' : 'This report could not be loaded. Please try again.'} <Button onClick={() => setAttempt(value => value + 1)}>Retry</Button></>}</Callout>;
  if (state.identity !== bindingIdentity || !state.report) return <Spinner aria-label="Loading report" size={24}/>;
  if (state.reviewKeys?.length) {
    return <SavedFilterReview key={parameterIdentity} keys={state.reviewKeys} onContinue={removed => {
      const parameters={...entry.parameters};
      for (const key of removed) delete parameters[key];
      if (removed.length) {
        activeWindows.value=activeWindows.peek().map(window => window.windowId===entry.windowId ? {...window,parameters} : window);
      }
      setState(previous=>({...previous,reviewKeys:[],reviewedFrom:launchIdentity,reviewedParameters:parameters}));
    }}/>;
  }
  const effectiveEntry = state.reviewedFrom === launchIdentity ? {...entry,parameters:state.reviewedParameters} : entry;
  // Seed the calendar draft only after authorization, with the full inputs for
  // this scope. Parameter changes do not require another definition read.
  return <AdmittedReportWindow key={parameterIdentity} entry={effectiveEntry} report={state.report} services={services}
    restored={restored} historicalReplay={historicalReplay} publishReportView={publishReportView} declaredFields={state.declaredFields}/>;
}

function AdmittedReportWindow({entry, report, services, restored, historicalReplay, publishReportView, declaredFields}) {
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
    <ReportFilterBar context={reportContext} controls={fields} values={{...parameters,...Object.fromEntries(fields.filter(field=>field.multiple&&field.kind==='text'&&editedParameters[field.id]!==undefined).map(field=>[field.id,editedParameters[field.id]]))}} scopeKey={JSON.stringify(report.resource)} loadChoices={loadChoices} loadLookupRows={choiceLoader}
      onChange={(id,value)=>{const field=fields.find(field=>field.id===id);setParameter(id,field?.multiple&&field.kind!=='choice'&&typeof value==='string'?reportParameterValue(value,field):value,value);}}
      onDatePreset={dates.requestPreset} onDateRange={dates.manualRange} onDateDraft={dates.cancelDraftRequests} onDateTimeZone={dates.setTimeZone}
      onApply={()=>{if(!missing.length&&!dates.dateBlocked){setApplied(applyReportFilterParameters(parameters,fields));setRunTrigger(value=>value+1);}}}
      onReset={()=>{dates.reset();setEditedParameters({});}}
      applyDisabled={missing.length>0||dates.dateBlocked} placement={report.resourceDefinition?.builderDefinition?.reportBuilder?.filterPresentation||report.resourceDefinition?.metadata?.filterPlacement||'top'}/>
    {missing.length>0&&<Callout intent="warning" title="Required report inputs">Complete these inputs before running the report: {missing.map(field=>field.label).join(', ')}.</Callout>}
    {report.resource?.uri?.startsWith('report://users/') && <PersonalReportOrigin
      key={JSON.stringify([report.resource.authorityBinding, report.resourceDefinition?.metadata?.personalReport?.provider, report.resourceDefinition?.metadata?.personalReport?.uri])}
      source={report.resourceDefinition?.metadata?.personalReport} getReport={services.reportStore?.getReport}/>}
    <ReportResourceRuntime onViewStateChange={publishReportView} container={{parameters:{reportResource:report.resource,reportTitle,
      reportCapabilities:{run:appliedMissing.length===0,export:true,tableBlockSelection:true,workbookExport:true},prefill:fields.length||restored?appliedParameters:parameters,
      showRunControl:fields.length===0,executeOnOpen:appliedMissing.length===0&&(!historicalReplay&&executeOnOpen||runTrigger>0),runTrigger}}} context={reportContext}/>
  </div>;
}
