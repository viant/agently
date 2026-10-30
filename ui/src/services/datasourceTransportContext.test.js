import {afterEach, beforeEach, expect, it, vi} from 'vitest';
vi.mock('forge/core',()=>({findMetadataSignal:vi.fn(),getActiveUIBridgeTransportContext:vi.fn(),waitForUIBridgeReady:vi.fn()}));
import {getActiveUIBridgeTransportContext,waitForUIBridgeReady} from 'forge/core';
import {prepareAgentlyDataConnectorRequest} from './datasourceRequestContext.js';
beforeEach(()=>{
 vi.stubGlobal('window',{location:{href:'http://owned/workspace'}});
 getActiveUIBridgeTransportContext.mockReturnValue({url:'http://owned/v1/ui/rpc',sessionHeader:'Mcp-Session-Id',sessionId:'negotiated-session'});
});
afterEach(()=>vi.unstubAllGlobals());
it('carries only the existing same-origin bridge transport session for datasource fetch',()=>{
 const headers={};const body={inputs:{id:123}};
 prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/records/fetch',headers,body});
 expect(headers).toEqual({'Mcp-Session-Id':'negotiated-session'});expect(body).toEqual({inputs:{id:123}});
});
it('keeps bridge routing on a published cohort datasource URL with a query marker',()=>{
 const headers={};const queryParams=new URLSearchParams();
 prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/records/fetch?cohort=1',headers,queryParams,body:{},windowState:{windowId:'published-instance'}});
 expect(headers).toEqual({'Mcp-Session-Id':'negotiated-session'});
 expect(queryParams.get('windowId')).toBe('published-instance');
});
it('does not leak the UI session to foreign MCP/remote destinations or unrelated requests',()=>{
 for(const url of ['http://foreign/v1/api/datasources/records/fetch','/v1/ui/rpc','http://foreign/v1/api/agently/forge/window/original','http://foreign/mcp']){
  const headers={};prepareAgentlyDataConnectorRequest({url,headers,body:{}});expect(headers).toEqual({});
 }
 getActiveUIBridgeTransportContext.mockReturnValue(null);
 const headers={};prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/records/fetch',headers,body:{}});expect(headers).toEqual({});
});

it('waits for the existing bridge before preparing an early cohort read',async()=>{
 let release;getActiveUIBridgeTransportContext.mockReturnValue(null);
 waitForUIBridgeReady.mockImplementation(()=>new Promise(resolve=>{release=resolve}));
 const headers={},queryParams=new URLSearchParams();
 const pending=prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/records/fetch?cohort=1',headers,queryParams,body:{},windowState:{windowId:'fresh-window'}});
 expect(headers).toEqual({});
 getActiveUIBridgeTransportContext.mockReturnValue({url:'http://owned/v1/ui/rpc',sessionHeader:'Mcp-Session-Id',sessionId:'fresh-session'});
 release(true);await pending;
 expect(headers['Mcp-Session-Id']).toBe('fresh-session');expect(queryParams.get('windowId')).toBe('fresh-window');
});
it('reports an unavailable workspace transport without pretending it is an access denial',async()=>{
 getActiveUIBridgeTransportContext.mockReturnValue(null);waitForUIBridgeReady.mockResolvedValue(false);
 await expect(prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/records/fetch?cohort=1',headers:{},body:{}})).rejects.toThrow('workspace connection is not ready');
});
