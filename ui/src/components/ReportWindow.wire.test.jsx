// @vitest-environment jsdom
import React from 'react';
import {afterEach, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
vi.mock('../services/agentlyClient', async () => {
  const {AgentlyClient} = await import('agently-core-ui-sdk');
  return {client: new AgentlyClient({baseURL:'http://owned/v1', useCookies:true, retries:0, fetchImpl:(url,init)=>globalThis.fetch(url,init)})};
});


vi.mock('forge/components', async () => ({ReportResourceRuntime: (await import('forge/components/dashboard/ReportResourceRuntime.jsx')).default}));
vi.mock('forge/components/dashboard/ReportRuntime.jsx', () => ({default: ({reportSpec}) => <output>{reportSpec.title}</output>}));
vi.mock('forge/core', async () => ({useSetting: () => ({services: (hostServices)})}));
let hostServices;
import {getReport, runReport} from '../services/reportStoreService';
import {submitReportExportResource, getReportExportStatus, getReportExportArtifact} from '../services/reportExportService';
import ReportWindow from './ReportWindow.jsx';
afterEach(() => {cleanup(); vi.unstubAllGlobals(); vi.restoreAllMocks();});

it('renders the shared resource runtime and uses canonical SDK get/run/export routes with current scope and the producer pin', async () => {
  const pin = {providerIdentity:'native-producer', uri:'report://ar/overview', kind:'working', contentFingerprint:'exact', authorityBinding:'owned', validUntil:'2030-01-01T00:00:00Z'};
  const calls=[];
  vi.stubGlobal('fetch', async (url, init) => {
    calls.push({url, init, args:JSON.parse(init.body)});
    const payload = url.includes('get_report') ? {resource:pin,title:'Overview'}
      : url.includes('run_report') ? {resource:pin,reportSpec:{title:'Server overview'},reportFill:{},reportPrint:{}}
      : url.includes('submit_export') ? {jobId:'export-1'}
      : url.includes('get_export_status') ? {status:'succeeded',artifactId:'artifact-1'}
      : {artifactId:'artifact-1',jobId:'export-1',artifactRef:pin.uri,format:'csv',data:'YWJj'};
    return new Response(JSON.stringify({result:JSON.stringify(payload)}),{headers:{'content-type':'application/json'}});
  });
  hostServices = {reportStore:{getReport,runReport}, reportExport:{submitResource:submitReportExportResource,getStatus:getReportExportStatus,getArtifact:getReportExportArtifact}};
  const createURL=vi.fn(()=> 'blob:report');
  URL.createObjectURL=createURL; URL.revokeObjectURL=vi.fn();
  vi.spyOn(HTMLAnchorElement.prototype,'click').mockImplementation(()=>{});
  const entry={windowKey:pin.uri,resourceRef:{uri:pin.uri},parameters:{AdvertiserIds:[42]}};
  const view=render(<ReportWindow window={entry}/>);
  fireEvent.click(await screen.findByRole('button',{name:'Run report'}));
  await screen.findByRole('heading',{name:'Server overview'});
  view.rerender(<ReportWindow window={{...entry,parameters:{AdvertiserIds:[0],Search:''}}}/>);
  fireEvent.click(screen.getByRole('button',{name:'Run report'}));
  await screen.findByRole('heading',{name:'Server overview'});
  fireEvent.click(screen.getByRole('button',{name:'Export CSV'}));
  await waitFor(()=>expect(createURL).toHaveBeenCalledOnce());
  expect(calls.map(call=>call.url)).toEqual(['get_report','run_report','run_report','submit_export','get_export_status','get_artifact'].map(name=>`http://owned/v1/tools/reporting%3A${name}/execute`));
  const selected={resource:{uri:pin.uri,revision:'working'},resolvedResource:pin,parameters:{AdvertiserIds:[0],Search:''}};
  expect(calls[0].args).toEqual({resource:{uri:pin.uri}});
  expect(calls[2].args).toEqual(selected);
  expect(calls[3].args).toEqual({...selected,format:'csv'});
  expect(calls.every(call=>call.init.credentials==='include')).toBe(true);
  expect(view.container.querySelector('iframe')).toBeNull();
});

it('collects published predicate inputs before manual execution and sends typed current values through the shared runtime', async () => {
  const pin = {providerIdentity:'native-producer',uri:'report://ar/example',kind:'working',contentFingerprint:'exact',authorityBinding:'owned',validUntil:'2030-01-01T00:00:00Z'};
  const predicates = [
    {id:'advertiserId',kind:'number',label:'Advertiser',paramPath:'filters.advertiserId',required:true},
    {id:'suiteId',kind:'number',label:'Subscription suite',paramPath:'filters.suiteId',required:true},
    {id:'campaignIds',kind:'multiSelect',label:'Campaigns',multiple:true,paramPath:'filters.campaignIds'},
    {id:'dateRange',kind:'dateRange',label:'Date Range',prefill:{start:'from',end:'to'},startParamPath:'filters.from',endParamPath:'filters.to'},
  ];
  const calls=[];
  vi.stubGlobal('fetch',async(url,init)=>{
    calls.push({url,args:JSON.parse(init.body)});
    const payload=url.includes('get_report')?{resource:pin,resourceDefinition:{builderDefinition:{reportBuilder:{predicates}}}}
      :{resource:pin,reportSpec:{title:'Selected scope'},reportFill:{},reportPrint:{}};
    return new Response(JSON.stringify({result:JSON.stringify(payload)}),{headers:{'content-type':'application/json'}});
  });
  hostServices={reportStore:{getReport,runReport}};
  render(<ReportWindow window={{windowKey:pin.uri,resourceRef:{uri:pin.uri},parameters:{}}}/>);
  await screen.findByLabelText('Advertiser');
  expect(calls).toHaveLength(1);
  fireEvent.change(screen.getByLabelText('Campaigns'),{target:{value:'31,'}});
  expect(screen.getByLabelText('Campaigns').value).toBe('31,');
  for(const [label,value] of [['Advertiser','42'],['Subscription suite','7'],['Campaigns','31, 32'],['Date Range from','2026-01-02'],['Date Range to','2026-01-03']]) {
    fireEvent.change(screen.getByLabelText(label),{target:{value}});
  }
  fireEvent.click(screen.getByRole('button',{name:'Run report'}));
  await screen.findByRole('heading',{name:'Selected scope'});
  expect(calls[1].args).toEqual({resource:{uri:pin.uri,revision:'working'},resolvedResource:pin,parameters:{advertiserId:42,suiteId:7,campaignIds:[31,32],from:'2026-01-02',to:'2026-01-03'}});
  fireEvent.change(screen.getByLabelText('Advertiser'),{target:{value:'0'}});
  fireEvent.change(screen.getByLabelText('Campaigns'),{target:{value:''}});
  fireEvent.change(screen.getByLabelText('Date Range from'),{target:{value:''}});
  fireEvent.click(screen.getByRole('button',{name:'Run report'}));
  await screen.findByRole('heading',{name:'Selected scope'});
  expect(calls[2].args.parameters).toEqual({advertiserId:0,suiteId:7,campaignIds:[],from:'',to:'2026-01-03'});
});
