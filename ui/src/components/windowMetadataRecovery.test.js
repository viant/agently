// Exercise actual component effect bodies with deterministic hooks and stubbed
// connectors. This verifies retry/state wiring without browser or server access.
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
const require = createRequire(import.meta.url);
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const assert = require('node:assert/strict');
import {test} from 'vitest';
const esbuild = require('esbuild');
const parser = require('@babel/parser');
const dir = path.dirname(fileURLToPath(import.meta.url));
function bus() {
 const listeners = new Map();
 return {addEventListener(type,fn){if(!listeners.has(type))listeners.set(type,new Set());listeners.get(type).add(fn)}, removeEventListener(type,fn){listeners.get(type)?.delete(fn)},dispatchEvent(event){for(const fn of listeners.get(event.type)||[])fn(event)},localStorage:{getItem(){return null},setItem(){}}, count(type){return listeners.get(type)?.size||0}};
}
function compile(file,extra) {
 const source=fs.readFileSync(file,'utf8');
 const ast=parser.parse(source,{sourceType:'module',plugins:['jsx']});
 const body=ast.program.body.filter(n=>n.type!=='ImportDeclaration').map(n=>{
  if(n.type==='ExportDefaultDeclaration'||n.type==='ExportNamedDeclaration')return source.slice(n.declaration.start,n.declaration.end);
  return source.slice(n.start,n.end);
 }).join('\n');
 const code=esbuild.transformSync(body,{loader:'jsx',format:'cjs',target:'node20'}).code;
 const sandbox={console:{error(){},debug(){}},setTimeout,clearTimeout,...extra};
 vm.createContext(sandbox);vm.runInContext(code,sandbox);return sandbox;
}
function hooks(shared={}) {
 const slots=[];let at=0,effects=[],dirty=false,render;
 const React={Fragment:'fragment',createElement(type,props,...children){return{type,props:{...props,children}}}};
 const api={React,
  useState(init){const key=at++;if(!slots[key])slots[key]={value:typeof init==='function'?init():init};const slot=slots[key];return[slot.value,value=>{const next=typeof value==='function'?value(slot.value):value;if(!Object.is(next,slot.value)){slot.value=next;dirty=true}}]},
  useRef(value){const key=at++;if(!slots[key])slots[key]={current:value};return slots[key]},
  useMemo(fn,deps){const key=at++;if(!slots[key]||deps.some((v,i)=>!Object.is(v,slots[key].deps[i]))){slots[key]={value:fn(),deps}}return slots[key].value},
  useEffect(fn,deps){const key=at++;if(!slots[key]||!deps||deps.some((v,i)=>!Object.is(v,slots[key].deps[i]))){const old=slots[key];slots[key]={deps,cleanup:old?.cleanup};effects.push(()=>{slots[key].cleanup?.();slots[key].cleanup=fn()})}},...shared};
 return {api,setRender(fn){render=fn},view:null,async flush(){for(let n=0;n<12;n++){at=0;dirty=false;this.view=render();const pending=effects;effects=[];for(const effect of pending)effect();await new Promise(resolve=>setImmediate(resolve));if(!dirty&&effects.length===0)return this.view}throw Error('render did not settle')},dispose(){for(const slot of slots)slot?.cleanup?.()}};
}
function windowHarness(events,options={}) {
 let healthy=false;let calls=0,permissions=0;const signal={value:undefined,peek(){return this.value}};
 const metadata=options.protected?{authorization:{scope:'resource'},view:{content:{id:'ready'}}}:{view:{content:{id:'ready'}}};
 const services={applyPermission:async value=>{permissions++;if(options.denyPermission)throw Object.assign(new Error('denied'),{status:403});return{view:value.completeMetadata.view}}};
 const auth={};const settings={endpoints:{},connectorConfig:{window:{service:{uri:'/window'}}},services,targetContext:{},useAuth:()=>auth};
 const runner=hooks({useSignals(){},findMetadataSignal:()=>signal,getMetadataSignal:()=>signal,useSetting:()=>settings,
  useDataConnector:()=>({get:async()=>{calls++;if(!healthy)throw Object.assign(new Error('GET error 500'),{status:options.status||500});return{data:metadata}}}),
  buildReportBuilderHostServices:()=>services,activeWindows:{peek:()=>[]},primeWindowSignals(){},injectActions(){},
  resolveMetadataForTarget:value=>value,SoftSkeleton:'skeleton',SoftBlock:'block',
  addEventListener:events.addEventListener,removeEventListener:events.removeEventListener,
 });
 const source=path.resolve(dir,'../../../../forge/src/components/WindowContent.jsx');
 const runtime=compile(source,runner.api);const window={windowKey:'chat',windowId:'chat-1',parameters:{}};
 runner.setRender(()=>runtime.WindowContentRuntime({window}));
 return{runner,recover(){healthy=true},calls:()=>calls,permissions:()=>permissions};
}
function sidebarHarness(events) {
 let healthy=false,calls=0;
 const runner=hooks({Button:'button',Icon:'icon',Sidebar:'sidebar',sdkBaseURL:'/v1',window:events,CustomEvent:class{constructor(type){this.type=type}},getAuthMeSilently:async()=>null,fetch:async()=>{calls++;return{ok:healthy,status:healthy?200:500,json:async()=>({layout:{applications:[],left:{width:{preferenceScope:'global'}}}})}}});
 const runtime=compile(path.join(dir,'WorkspaceSidebar.jsx'),runner.api);
 runner.setRender(()=>runtime.WorkspaceSidebar({}));
 return{runner,recover(){healthy=true},calls:()=>calls};
}
function find(node,predicate){if(!node||typeof node!=='object')return null;if(predicate(node))return node;for(const child of node.props?.children?.flat(Infinity)||[]){const found=find(child,predicate);if(found)return found}return null}

test('existing layout Retry recovers retained child metadata failure without remount',async()=>{
 const events=bus(),window=windowHarness(events),sidebar=sidebarHarness(events);
 await window.runner.flush();await sidebar.runner.flush();assert.equal(window.runner.view.props.role,'alert');assert.equal(window.calls(),1);
 window.recover();sidebar.recover();const retry=find(sidebar.runner.view,n=>n.type==='button'&&n.props.children.includes('Retry'));assert.ok(retry);
 retry.props.onClick();await sidebar.runner.flush();await window.runner.flush();
 assert.equal(window.runner.view.type.name,'WindowContentInner');assert.equal(window.calls(),2);assert.equal(sidebar.calls(),2);
 events.dispatchEvent({type:'forge:retry-window-metadata'});await window.runner.flush();assert.equal(window.calls(),2,'healthy window is not fetched again');
 window.runner.dispose();sidebar.runner.dispose();assert.equal(events.count('forge:retry-window-metadata'),0);
});
test('duplicate recovery signals issue one GET and repeat fresh protected preflight',async()=>{
 const events=bus(),window=windowHarness(events,{protected:true});await window.runner.flush();window.recover();
 events.dispatchEvent({type:'forge:retry-window-metadata'});events.dispatchEvent({type:'forge:retry-window-metadata'});await window.runner.flush();
 assert.equal(window.calls(),2);assert.equal(window.permissions(),1);assert.equal(window.runner.view.type.name,'WindowContentInner');window.runner.dispose();
});
for(const status of [401,403,404,422])test(`recovery never retries denied/invalid status ${status}`,async()=>{
 const events=bus(),window=windowHarness(events,{status});await window.runner.flush();window.recover();events.dispatchEvent({type:'forge:retry-window-metadata'});await window.runner.flush();assert.equal(window.calls(),1);assert.equal(window.runner.view.props.role,'alert');window.runner.dispose();
});
test('permission denial after successful metadata recovery remains closed',async()=>{
 const events=bus(),window=windowHarness(events,{protected:true,denyPermission:true});await window.runner.flush();window.recover();events.dispatchEvent({type:'forge:retry-window-metadata'});await window.runner.flush();assert.equal(window.calls(),2);assert.equal(window.permissions(),1);assert.equal(window.runner.view.props.role,'alert');events.dispatchEvent({type:'forge:retry-window-metadata'});await window.runner.flush();assert.equal(window.calls(),2);window.runner.dispose();
});
