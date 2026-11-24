// @vitest-environment jsdom
import React, {act} from 'react';
import {createRoot} from 'react-dom/client';
import {describe,it,expect,vi} from 'vitest';
const {applyPermission,executeTool,transport,appearance}=vi.hoisted(()=>({applyPermission:vi.fn(),executeTool:vi.fn(),transport:vi.fn(),appearance:{themeId:'baseline'}}));
vi.mock('../WorkspaceStyleProvider.jsx',async()=>({...await vi.importActual('../WorkspaceStyleProvider.jsx'),useWorkspaceStyle:()=>({state:appearance})}));
vi.mock('../../services/agentlyClient',()=>({client:{applyPermission,executeTool}}));
vi.mock('forge/core',async()=>({...await vi.importActual('forge/core'),getActiveUIBridgeTransportContext:transport}));
vi.mock('./LazyRichContent',async()=>({default:(await import('./RichContent.jsx')).default}));
import BubbleMessage from './BubbleMessage.jsx';
import RichContent from './RichContent.jsx';

describe('mounted report retains the original host frame',()=>{
 it('renders after a task-entry lease expires and opens before one exact report event',async()=>{
  globalThis.IS_REACT_ACT_ENVIRONMENT=true;
  const original={providerIdentity:'configured-framework',uri:'window://agently/chat-new',kind:'working',contentFingerprint:'a'.repeat(64),authorityBinding:'original-owner',validUntil:'2000-01-01T00:00:00Z'};
  const context={identity:{windowId:'chat/new'},metadata:{resource:original},windowState:{windowId:'chat/new',windowKey:'chat/new',parameters:{EntityId:['original']}},services:{__connectorRuntime:{targetContext:{platform:'web',formFactor:'desktop'}}}};
  transport.mockReturnValue({clientId:'own-client',sessionId:'own-session'});
  const order=[];applyPermission.mockImplementation(async()=>{order.push('OPEN');return {};});executeTool.mockImplementation(async()=>{order.push('record');return {recorded:true};});
  const renderedContent={schemaVersion:'1',parts:[{kind:'markdown',text:'Retained public highlights'}],reports:[{scope:'message',id:'brief',grammar:'dashboard-v1',status:'committed',sequence:1,resetVersion:0,source:{title:'Owned report',blocks:[{id:'rows',kind:'dashboard.table',title:'Owned rows',dataSourceRef:'rows',columns:[{key:'channel',label:'Channel'}]}]},dataSources:{rows:{version:2,id:'rows',format:'json',mode:'replace',payload:[{channel:'CTV'}]}}}],diagnostics:[]};
  const host=document.createElement('div');document.body.appendChild(host);const root=createRoot(host);
  try{
   await act(async()=>{root.render(<BubbleMessage context={context} conversationId="conv-owned" message={{id:'report-msg',role:'assistant',content:'Retained public highlights',renderedContent}}/>);await new Promise(resolve=>setTimeout(resolve,20));});
   expect(host.textContent).toContain('CTV');expect(host.textContent).toContain('Retained public highlights');
   expect(order).toEqual(['OPEN','record']);expect(applyPermission).toHaveBeenCalledTimes(1);
   expect(applyPermission.mock.calls[0][1]).toMatchObject({windowId:'chat/new',clientId:'own-client',expectedResource:{uri:original.uri,contentFingerprint:original.contentFingerprint},windowParams:{EntityId:['original']}});
   expect(executeTool.mock.calls[0][1]).toMatchObject({windowId:'chat/new',windowKey:'chat/new',clientId:'own-client',kind:'report.context_updated'});
   expect(context.metadata.resource).toBe(original);
  }finally{await act(async()=>root.unmount());host.remove();delete globalThis.IS_REACT_ACT_ENVIRONMENT;}
 });
});

it('applies the chart palette only in Design System 2.0 and retains baseline, authored colors and exact values',async()=>{
 globalThis.IS_REACT_ACT_ENVIRONMENT=true;
 const width=Object.getOwnPropertyDescriptor(HTMLElement.prototype,'clientWidth'),height=Object.getOwnPropertyDescriptor(HTMLElement.prototype,'clientHeight');
 Object.defineProperty(HTMLElement.prototype,'clientWidth',{configurable:true,get:()=>600});
 Object.defineProperty(HTMLElement.prototype,'clientHeight',{configurable:true,get:()=>240});
 const Resize=globalThis.ResizeObserver;
 globalThis.ResizeObserver=class{constructor(callback){this.callback=callback;}observe(target){this.callback([{target,contentRect:{width:600,height:240}}]);}unobserve(){}disconnect(){}};
 const host=document.createElement('div');document.body.append(host);const root=createRoot(host);
 const spec={title:'Synthetic daily records',chart:{type:'bar',x:{key:'day'},y:[{key:'records'}]},data:[{day:'First',records:12},{day:'Second',records:28}]};
 const render=async options=>{await act(async()=>root.render(<RichContent key={appearance.themeId} content={'```chart\n'+JSON.stringify({...spec,...options})+'\n```'}/>));await act(async()=>{await new Promise(resolve=>setTimeout(resolve,50));});};
 try{
  appearance.themeId='baseline';await render({});
  expect(host.querySelector('.recharts-legend-icon').getAttribute('fill')).toBe('#2563eb');
  expect(host.querySelector('.recharts-default-tooltip').style.color).not.toContain('--forge-chart-text');
  appearance.themeId='design-system-2';
  await render({});
  expect(host.querySelector('.app-rich-chart .recharts-legend-icon').getAttribute('fill')).toContain('--forge-chart-series-1');
  expect(host.querySelector('.recharts-default-tooltip').style.color).toContain('--forge-chart-text');
  await render({options:{palette:['#102030']}});
  expect(host.querySelector('.recharts-legend-icon').getAttribute('fill')).toBe('#102030');
  await act(async()=>[...host.querySelectorAll('button')].find(button=>button.textContent==='Table').click());
  expect(host.textContent).toContain('First');expect(host.textContent).toContain('12');expect(host.textContent).toContain('Second');expect(host.textContent).toContain('28');
 }finally{
  await act(async()=>root.unmount());host.remove();
  if(width)Object.defineProperty(HTMLElement.prototype,'clientWidth',width);else delete HTMLElement.prototype.clientWidth;
  if(height)Object.defineProperty(HTMLElement.prototype,'clientHeight',height);else delete HTMLElement.prototype.clientHeight;
  globalThis.ResizeObserver=Resize;appearance.themeId='baseline';delete globalThis.IS_REACT_ACT_ENVIRONMENT;
 }
});
