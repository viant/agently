import React,{useEffect,useMemo,useState} from 'react';
import {SettingProvider} from 'forge/core/context/Setting.jsx';
import {activeWindows,selectedWindowId} from 'forge/core/store/signals.js';
import {runUICommand} from 'forge/core/ui/commands.js';
import WindowManager from 'forge/components/WindowManager.jsx';
import {readRoute,windowURL} from './route.js';
import './preview.css';
export default function WindowPreview(){
 const [workspace,setWorkspace]=useState(null),[error,setError]=useState('');
 const [selected,setSelected]=useState(''),[variant,setVariant]=useState('default'),[parameters,setParameters]=useState('{}'),[query,setQuery]=useState('{}');
 const [initial,setInitial]=useState(null);
 const open=async(id,params={},catalog=workspace)=>{
  if(!catalog?.windows[id])throw new Error(`Unknown preview window: ${id}`);
  await runUICommand({method:'ui.window.open',params:{windowKey:id,windowTitle:catalog.windows[id].title||id,parameters:params,inTab:true}});
 };
 useEffect(()=>{let live=true;fetch('/api/workspace').then(async r=>{if(!r.ok)throw new Error(await r.text());return r.json()}).then(async catalog=>{if(!live)return;const route=readRoute(location.search,catalog.defaultWindow);setWorkspace(catalog);setInitial(route);setVariant(route.variant);setQuery(JSON.stringify(route.query||{},null,2));setParameters(JSON.stringify(route.parameters,null,2));setSelected(route.windowKey);await open(route.windowKey,route.parameters,catalog)}).catch(e=>setError(e.message));return()=>{live=false}},[]);
 useEffect(()=>{if(!workspace)return;const sync=()=>{const entry=activeWindows.peek().find(w=>w.windowId===selectedWindowId.peek());if(!entry)return;setSelected(entry.windowKey);setParameters(JSON.stringify(entry.parameters||{},null,2));const next=windowURL(location.href,entry);if(next!==location.pathname+location.search)history.pushState({},'',next)};const a=activeWindows.subscribe(sync),b=selectedWindowId.subscribe(sync);const pop=()=>{try{const route=readRoute(location.search,workspace.defaultWindow);open(route.windowKey,route.parameters).catch(e=>setError(e.message))}catch(e){setError(e.message)}};addEventListener('popstate',pop);return()=>{a();b();removeEventListener('popstate',pop)}},[workspace]);
 const endpoints=useMemo(()=>({preview:{baseURL:location.origin}}),[]);
 const connectorConfig=useMemo(()=>({window:{service:{endpoint:'preview',uri:'/api/windows'}}}),[]);
 const services=useMemo(()=>({prepareDataConnectorRequest(request){if(!request.url.includes('/datasources/'))return request;const body={...(request.body||{}),inputs:{...(request.windowState?.parameters||{}),...(request.body?.inputs||{})},variant:initial?.variant||'default'};if(initial?.query&&request.windowState?.windowKey===initial.windowKey)body.inputs={...(body.inputs||{}),query:initial.query};return {...request,body}}}),[initial]);
 const apply=()=>{try{const params=JSON.parse(parameters),q=JSON.parse(query);const url=new URL(location.href);url.searchParams.set('window',selected);url.searchParams.set('variant',variant);url.searchParams.set('parameters',JSON.stringify(params));if(Object.keys(q).length)url.searchParams.set('query',JSON.stringify(q));else url.searchParams.delete('query');location.assign(url.href)}catch(e){setError(e.message)}};
 return <><header><div><h1>Forge window preview</h1><p>Native window runtime · MCP datasources</p></div><span>Synthetic mock workspace</span></header><main><aside><label>Window<select value={selected} onChange={e=>{setError('');open(e.target.value).catch(e=>setError(e.message))}}>{Object.entries(workspace?.windows||{}).map(([id,w])=><option key={id} value={id}>{w.title||id}</option>)}</select></label><label>Variant<select value={variant} onChange={e=>setVariant(e.target.value)}>{['default','empty','error','large'].map(v=><option key={v}>{v}</option>)}</select></label><label>Window parameters (JSON)<textarea value={parameters} onChange={e=>setParameters(e.target.value)}/></label><label>Query override (JSON)<textarea value={query} onChange={e=>setQuery(e.target.value)}/></label><button onClick={apply}>Apply and reload</button><p>Open a project link to view its tasks. Window IDs and parameters are preserved in the URL.</p><a href="/api/workspace" target="_blank" rel="noreferrer">Workspace metadata</a></aside><section>{error&&<div role="alert">{error}</div>}{workspace&&initial&&<SettingProvider endpoints={endpoints} connectorConfig={connectorConfig} services={services}><WindowManager/></SettingProvider>}</section></main></>;
}
