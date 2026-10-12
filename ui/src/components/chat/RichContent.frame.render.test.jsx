// @vitest-environment jsdom
import React, {act} from 'react';
import {createRoot} from 'react-dom/client';
import {describe,it,expect,vi} from 'vitest';
const {applyPermission,executeTool,transport}=vi.hoisted(()=>({applyPermission:vi.fn(),executeTool:vi.fn(),transport:vi.fn()}));
vi.mock('../../services/agentlyClient',()=>({client:{applyPermission,executeTool}}));
vi.mock('forge/core',async()=>({...await vi.importActual('forge/core'),getActiveUIBridgeTransportContext:transport}));
vi.mock('./LazyRichContent',async()=>({default:(await import('./RichContent.jsx')).default}));
import BubbleMessage from './BubbleMessage.jsx';

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
