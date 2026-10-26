// @vitest-environment jsdom
import React from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {it,expect,vi} from 'vitest';
import {createRoot} from 'react-dom/client';
import {act} from 'react-dom/test-utils';
import {submitReportExportRequest} from '../../services/reportExportService.js';
vi.mock('../../services/reportExportService.js',()=>({submitReportExportRequest:vi.fn().mockRejectedValue(new Error('Export stopped by local test')),getReportExportArtifact:vi.fn(),getReportExportStatus:vi.fn()}));
vi.mock('../../services/reportEventService.js',()=>({emitReportUIEvent:vi.fn().mockResolvedValue({})}));
import {HostedInlineReport} from './RichContent.jsx';

it('hosts the shared analytical document and keeps the legacy report path',()=>{
  const source={title:'Example analytical report',subtitle:'Selected scope',viewer:{appearance:'document',allowChartTableSwitch:false},blocks:[{id:'total',kind:'kpiBlock',datasetRef:'records',valueField:'count',valueLabel:'Records',valueFormat:'number'}]};
  const assembly={id:'document-example',scope:'message',grammar:'report-document-v1',status:'committed',authoritative:true,sequence:1,source,dataSources:{records:{id:'records',format:'json',data:[{count:12345}]}}};
  const documentHTML=renderToStaticMarkup(<HostedInlineReport assembly={assembly}/>);
  expect(documentHTML).toContain('data-forge-report-appearance="document"');
  expect(documentHTML).toMatch(/<h1[^>]*data-forge-type="title"/);
  expect(documentHTML).toContain('data-forge-type="metric"');
  expect(documentHTML).toContain('12,345');
  expect(documentHTML).toContain('data-forge-variant="secondary"');
  expect(documentHTML).not.toContain('app-rich-inline-report__export-button');
  const legacyHTML=renderToStaticMarkup(<HostedInlineReport assembly={{...assembly,source:{...source,viewer:undefined}}}/>);
  expect(legacyHTML).not.toContain('data-forge-report-appearance="document"');
  expect(legacyHTML).toContain('app-rich-inline-report__title');
});

// Both canonical and legacy planners use the same neutral review-table renderer.
it('hosts a review table with selections and a local CSV action',()=>{
  const assembly={id:'review',scope:'message',grammar:'report-document-v1',status:'committed',authoritative:true,sequence:1,
    source:{title:'Review choices',viewer:{appearance:'document'},blocks:[{id:'choices',kind:'tableBlock',datasetRef:'choices',selection:{mode:'checkbox',field:'selected'},columns:[{key:'name',label:'Name'}]}]},
    dataSources:{choices:{id:'choices',format:'json',data:[{name:'First choice',selected:true},{name:'Second choice',selected:false}]}}};
  const html=renderToStaticMarkup(<HostedInlineReport assembly={assembly}/>);
  expect(html).toContain('data-forge-review-table="choices"');
  expect(html).toContain('1 of 2 selected');
  expect(html).toContain('Select First choice');
  expect(html).toContain('Download CSV');
  expect(html).not.toContain('Submit');
});

it('retains local review choices and exports them without a real save',async()=>{
  const rows=[{name:'First choice',selected:true,cost:7},{name:'Second choice',selected:true,cost:12}];
  const assembly={id:'review-state',scope:'message',grammar:'report-document-v1',status:'committed',authoritative:true,sequence:1,
    source:{title:'Review choices',viewer:{appearance:'document'},blocks:[{id:'choices',kind:'tableBlock',datasetRef:'choices',selection:{mode:'checkbox',field:'selected'},columns:[{key:'name',label:'Name'}]}]},
    dataSources:{choices:{id:'choices',format:'json',data:rows}}};
  const node=document.createElement('div');document.body.append(node);const root=createRoot(node);
  try {
    await act(async()=>root.render(<HostedInlineReport assembly={assembly}/>));
    const choices=()=>node.querySelectorAll('tbody input[type=checkbox]');
    expect(choices()[1].checked).toBe(true);
    await act(async()=>choices()[1].click());
    expect(choices()[1].checked).toBe(false);
    expect(node.textContent).toContain('1 of 2 selected');
    expect(node.querySelector('thead input').indeterminate).toBe(true);
    expect(node.querySelector('thead input').getAttribute('aria-checked')).toBe('mixed');
    await act(async()=>root.render(<HostedInlineReport assembly={structuredClone(assembly)}/>));
    expect(choices()[1].checked).toBe(false);
    const exportButton=[...node.querySelectorAll('button')].find(button=>button.textContent==='Export PDF');
    await act(async()=>exportButton.click());
    const submitted=submitReportExportRequest.mock.calls.at(-1)[0].request;
    expect(JSON.stringify(submitted)).toContain('Selected');
    const fill=submitted.reportFill || submitted.snapshot?.reportFill;
    expect(fill.datasets.find(dataset=>dataset.id==='choices').rows).toEqual([{name:'First choice',selected:true,cost:7},{name:'Second choice',selected:false,cost:12}]);
    expect(rows[1].selected).toBe(true);
    await act(async()=>root.render(<HostedInlineReport assembly={{...assembly,sequence:2}}/>));
    expect(choices()[1].checked).toBe(true);
  } finally {await act(async()=>root.unmount());node.remove();}
});

it('upgrades only legacy presentation in the opted-in viewer without modifying saved content',async()=>{
 const {normalizeCanonicalReportAssembly}=await import('../../services/workspaceAdapters.js');
 const source={title:'Saved report',viewer:{allowChartTableSwitch:false},blocks:[{id:'total',kind:'kpiBlock',datasetRef:'records',valueField:'count',valueLabel:'Records'}]};
 const saved={id:'saved',status:'committed',grammar:'report-document-v1',source,dataSources:{records:{format:'json',data:[{count:12345}]}}};
 const before=JSON.stringify(saved),documentAssembly=normalizeCanonicalReportAssembly(saved,'document');
 expect(documentAssembly.source.viewer).toEqual({appearance:'document',allowChartTableSwitch:false});
 expect(documentAssembly.source.blocks).toBe(source.blocks);expect(documentAssembly.dataSources.records.data).toBe(saved.dataSources.records.data);
 expect(JSON.stringify(saved)).toBe(before);expect(normalizeCanonicalReportAssembly(saved).source).toBe(source);
 expect(renderToStaticMarkup(<HostedInlineReport assembly={documentAssembly}/>)).toContain('data-forge-report-appearance="document"');
 const explicit={...saved,source:{...source,viewer:{appearance:'future'}}};
 expect(normalizeCanonicalReportAssembly(explicit,'document').source).toBe(explicit.source);
 expect(normalizeCanonicalReportAssembly({...saved,source:JSON.stringify(source)},'document').source.viewer.appearance).toBe('document');
});
