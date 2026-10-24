import {afterEach,beforeEach,expect,it,vi} from 'vitest';

vi.mock('forge/core',()=>({findMetadataSignal:vi.fn(),getActiveUIBridgeTransportContext:()=>({url:'https://host.test/v1/ui/rpc',sessionHeader:'Mcp-Session-Id',sessionId:'negotiated-session',clientId:'client'})}));
vi.mock('./agentlyClient',()=>({client:{},getAuthMeSilently:vi.fn(),getReportRestoreScope:()=> 'account-scope'}));
import {fetchReportBuilderPreviewByRef} from './reportingHostServices.js';

const resource={uri:'window://owner/metrics',providerIdentity:'owner-provider',kind:'stamped',revision:'5',contentFingerprint:'selected-content',authorityBinding:'verified-account'};
const target={platform:'web',formFactor:'desktop',selectionToken:'server-selected-target'};
beforeEach(()=>{vi.stubGlobal('fetch',vi.fn(async()=>new Response(JSON.stringify({rows:[{count:42}]}),{status:200,headers:{'Content-Type':'application/json'}})));});
afterEach(()=>vi.unstubAllGlobals());

it('sends isolated report reads with the same selected window and UI session frame as ordinary datasources',async()=>{
 const parameters={Request:{measure:'count',from:'2026-10-15',to:'2026-10-21'}};
 const builderContext={identity:{windowId:'metrics-instance'},windowState:{windowId:'metrics-instance',presentation:'hosted',region:'chat.top',conversationId:'unsaved-conversation',parameters:{EntityId:[42]}},metadata:{resource,resourceTarget:target}};
 const original=JSON.stringify(builderContext),request=JSON.stringify(parameters);
 const controller=new AbortController();
 await expect(fetchReportBuilderPreviewByRef({dataSourceRef:'metrics',parameters,builderContext,omitConversationId:true,signal:controller.signal})).resolves.toEqual({rows:[{count:42}]});
 expect(fetch).toHaveBeenCalledOnce();
 const [url,options]=fetch.mock.calls[0];
 expect(url).toBe('/v1/api/datasources/metrics/fetch?windowId=metrics-instance');
 expect(options.credentials).toBe('include');expect(options.headers['Mcp-Session-Id']).toBe('negotiated-session');
 expect(JSON.parse(options.body)).toEqual({inputs:parameters,resource,target,windowParameters:{EntityId:[42]}});
 expect(options.signal).toBeInstanceOf(AbortSignal);
 expect(JSON.stringify(builderContext)).toBe(original);expect(JSON.stringify(parameters)).toBe(request);
});

it('retains exact server selection provenance and original parameters while leaving requested dataset inputs unchanged',async()=>{
 const selectionBinding={actor:{issuer:'issuer',subject:'actor'},instanceId:'delivery',parametersDigest:'original-digest',token:'server-selection'};
 const builderContext={windowState:{windowId:'metrics-instance',parameters:{EntityId:[99]},openedWindowParameters:{EntityId:[42]}},metadata:{resource,resourceTarget:target,selectionBinding}};
 const parameters={Request:{EntityId:99}};
 await fetchReportBuilderPreviewByRef({dataSourceRef:'metrics',parameters,builderContext});
 expect(JSON.parse(fetch.mock.calls[0][1].body)).toEqual({inputs:parameters,resource,target,selectionBinding,windowParameters:{EntityId:[42]}});
});

it('preserves the owning datasource error instead of hiding it behind a report message',async()=>{
 fetch.mockResolvedValueOnce(new Response(JSON.stringify({error:'owning datasource denied the selected scope'}),{status:403,headers:{'Content-Type':'application/json'}}));
 await expect(fetchReportBuilderPreviewByRef({dataSourceRef:'metrics',parameters:{},builderContext:{metadata:{resource,resourceTarget:target}}})).rejects.toThrow('owning datasource denied the selected scope');
 expect(fetch).toHaveBeenCalledOnce();
 expect(fetch.mock.calls[0][0]).toBe('/v1/api/datasources/metrics/fetch');
});
