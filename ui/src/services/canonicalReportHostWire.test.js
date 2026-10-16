import {afterEach, expect, it, vi} from 'vitest';
vi.mock('./agentlyClient', async () => {
  const {AgentlyClient} = await import('agently-core-ui-sdk');
  return {client: new AgentlyClient({baseURL:'http://owned/v1',useCookies:true,retries:0,fetchImpl:(url,init)=>globalThis.fetch(url,init)})};
});
import {runReport} from './reportStoreService';
import {submitReportExportResource, getReportExportArtifact} from './reportExportService';
afterEach(()=>vi.unstubAllGlobals());
it('canonical host Run/export/download uses real SDK routes, exact selected args and cookies', async () => {
  const requests=[];
  vi.stubGlobal('fetch',async(url,init)=>{
    requests.push({url,init,body:JSON.parse(init.body)});
    const payload=url.includes('get_artifact')?{artifactId:'artifact',data:'YWJj',format:'csv'}:{jobId:'job',resource:requests[0].body.resolvedResource};
    return new Response(JSON.stringify({result:JSON.stringify(payload)}),{status:200,headers:{'content-type':'application/json'}});
  });
  const pin={providerIdentity:'ordinary',uri:'report://owned/widgets',kind:'working',contentFingerprint:'exact',authorityBinding:'owned',validUntil:'2030-01-01T00:00:00Z'};
  const request={resource:{uri:pin.uri,revision:'working'},resolvedResource:pin,parameters:{account:'owned',term:'original'}};
  await runReport(request);
  await submitReportExportResource({...request,format:'csv'});
  const artifact=await getReportExportArtifact({artifactId:'artifact'});
  expect(requests.map(r=>r.url)).toEqual(['http://owned/v1/tools/reporting%3Arun_report/execute','http://owned/v1/tools/reporting%3Asubmit_export/execute','http://owned/v1/tools/reporting%3Aget_artifact/execute']);
  expect(requests[0].body).toEqual(request);
  expect(requests[1].body).toEqual({...request,format:'csv'});
  expect(requests[2].body).toEqual({artifactId:'artifact',includeData:true});
  expect(requests.every(r=>r.init.credentials==='include')).toBe(true);
  expect(Array.from(artifact.bytes)).toEqual([97,98,99]);
});
it('canonical host preserves denied SDK call without a successful result', async()=>{
  vi.stubGlobal('fetch',async()=>new Response(JSON.stringify({error:'source drift denied'}),{status:403,headers:{'content-type':'application/json'}}));
  await expect(runReport({resource:{uri:'report://owned/widgets'}})).rejects.toMatchObject({status:403});
});
