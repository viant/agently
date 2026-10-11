// @vitest-environment jsdom
import React, {act} from 'react';
import {createRoot} from 'react-dom/client';
import {afterEach, expect, it, vi} from 'vitest';
vi.mock('forge/components',async()=>({WindowContent:(await import('forge/components/WindowContent.jsx')).default}));
vi.mock('../../services/mcpApps/forgeGuestBridge.js',()=>({installForgeGuestBridge:()=>()=>{}}));
vi.mock('./MCPUIVerifierRouteDebug.jsx',()=>({default:()=>null}));
import {SettingProvider} from 'forge/core';
import {registerContainerRenderer} from 'forge/runtime/containerRendererRegistry.js';
import {getCollectionSignal,removeSignalsForKey} from 'forge/core/store/signals.js';
import {prepareAgentlyDataConnectorRequest} from '../../services/datasourceRequestContext.js';
import MCPUIForgeWindowPage from './MCPUIForgeWindowPage.jsx';

registerContainerRenderer('test.signed-page',()=> <div data-signed-page-content>Admitted page</div>);
const windowId='mcpui:Records';
let root,element;
afterEach(async()=>{if(root)await act(async()=>root.unmount());element?.remove();removeSignalsForKey(windowId);vi.unstubAllGlobals();});

it('passes the one OPEN bundle from the mounted standalone page through real WindowContent and the connector JSON body',async()=>{
  globalThis.IS_REACT_ACT_ENVIRONMENT=true;
  const parameters={EntityID:['9007199254740993'],nested:{selection:{region:'west'}},Limit:7};
  const selectionBinding={actor:{issuer:'issuer',subject:'actor',tenantId:'tenant',accountId:'account'},instanceId:'server-opened-instance',parametersDigest:'original-digest',token:'server-signed-binding',opaque:{preserved:true}};
  const resource={uri:'window://example/Records',kind:'working',contentFingerprint:'original-pin',authorityBinding:'original-actor',validUntil:'2030-01-01T00:00:00.123456789Z'};
  const resourceTarget={platform:'web',formFactor:'desktop',selectionToken:'original-target',dependencyPins:{rows:{uri:'datasource://example/Records/rows',contentFingerprint:'original-source'}},executionProof:{token:'original-proof'}};
  const admitted={resource,resourceTarget,selectionBinding,authorization:{scope:'resource'},authorizationSnapshot:{schemaVersion:2,expiresAt:'2000-01-01T00:00:00Z',resource:{capabilities:{read:false,write:false}}},dataSource:{rows:{service:{uri:'datasources/rows/fetch',endpoint:'appAPI',method:'POST'},selectionMode:'none'}},view:{content:{id:'signed-page',kind:'test.signed-page',dataSourceRef:'rows'}}};
  window.history.replaceState({},'',`/mcp-ui/forge-window?${new URLSearchParams({windowKey:'Records',windowParams:JSON.stringify(parameters)})}`);
  let opens=0,managingFetches=0,body;
  const fetch=vi.fn(async(url,options={})=>{
    const request=new URL(String(url),'http://localhost');
    if(request.pathname==='/v1/api/agently/forge/window/Records') {
      opens++;
      if(opens!==1)throw new Error('A second metadata GET mixes delivery frames');
      expect(request.searchParams.get('applyPermission')).toBe('true');
      expect(JSON.parse(request.searchParams.get('windowParams'))).toEqual(parameters);
      return new Response(JSON.stringify({data:admitted}),{status:200,headers:{'Content-Type':'application/json'}});
    }
    if(request.pathname==='/v1/api/datasources/rows/fetch') {
      managingFetches++;expect(options.method).toBe('POST');body=JSON.parse(options.body);
      return new Response(JSON.stringify({data:[{id:'authorized-row'}]}),{status:200,headers:{'Content-Type':'application/json'}});
    }
    throw new Error(`Unexpected request ${request.pathname}`);
  });
  vi.stubGlobal('fetch',fetch);
  const applyPermission=vi.fn(()=>{throw new Error('Repeated UI business preflight');});
  const services={prepareDataConnectorRequest:prepareAgentlyDataConnectorRequest,applyPermission};
  element=document.createElement('div');document.body.append(element);root=createRoot(element);
  await act(async()=>{root.render(<SettingProvider endpoints={{appAPI:{baseURL:'/v1/api'}}} connectorConfig={{window:{service:{uri:'agently/forge/window',endpoint:'appAPI'}}}} services={services} targetContext={{platform:'web',formFactor:'desktop'}}><MCPUIForgeWindowPage/></SettingProvider>);});
  await act(async()=>{for(let i=0;i<25;i++)await Promise.resolve();});
  expect(opens).toBe(1);expect(managingFetches).toBe(1);expect(applyPermission).not.toHaveBeenCalled();
  expect(body).toMatchObject({resource,target:resourceTarget,selectionBinding,windowParameters:parameters});
  expect(body).not.toHaveProperty('tool_meta');
  expect(element.querySelector('[data-signed-page-content]')).not.toBeNull();
  expect(getCollectionSignal(`${windowId}DSrows`).peek()).toEqual([{id:'authorized-row'}]);
});
