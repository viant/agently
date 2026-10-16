import {afterEach, beforeEach, expect, it, vi} from 'vitest';
vi.mock('forge/core',()=>({findMetadataSignal:vi.fn(),getActiveUIBridgeTransportContext:vi.fn()}));
import {getActiveUIBridgeTransportContext} from 'forge/core';
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
it('does not leak the UI session to foreign MCP/remote destinations or unrelated requests',()=>{
 for(const url of ['http://foreign/v1/api/datasources/records/fetch','/v1/ui/rpc','/v1/api/agently/forge/window/original','http://foreign/mcp']){
  const headers={};prepareAgentlyDataConnectorRequest({url,headers,body:{}});expect(headers).toEqual({});
 }
 getActiveUIBridgeTransportContext.mockReturnValue(null);
 const headers={};prepareAgentlyDataConnectorRequest({url:'/v1/api/datasources/records/fetch',headers,body:{}});expect(headers).toEqual({});
});
