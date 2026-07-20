import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

import {
  applyForgeDataBlocks,
  assembleForgeReportEvents,
  createPlannerTableSubmitPayload,
  forgeFenceSample,
  rowsToCsv,
  validateForgeUIBlock,
  validateForgeDataBlock,
  validateForgeReportWorkspaceReferences,
} from './forgeFenceContract.js';

test('shared progressive report fixtures match the web reducer', async () => {
  const fixtureURL = new URL('../../../../agently-core/sdk/testdata/report_inline_cases.json', import.meta.url);
  const fixture = JSON.parse(await readFile(fixtureURL, 'utf8'));
  for (const scenario of fixture.cases) {
    const events = [];
    const expression = /```(forge-data|forge-report)\s*\n([\s\S]*?)\n```/g;
    let match;
    let index = 0;
    while ((match = expression.exec(scenario.content)) !== null) {
      events.push({ kind: match[1] === 'forge-data' ? 'data' : 'report', index, payload: JSON.parse(match[2]) });
      index += 1;
    }
    const result = assembleForgeReportEvents(events);
    assert.equal(result.assemblies.length, scenario.reports.length, scenario.name);
    scenario.reports.forEach((expected, reportIndex) => {
      const actual = result.assemblies[reportIndex];
      assert.equal(actual.scope, expected.scope, scenario.name);
      assert.equal(actual.id, expected.id, scenario.name);
      assert.equal(actual.grammar, expected.grammar, scenario.name);
      assert.equal(actual.status, expected.status, scenario.name);
      assert.equal(actual.sequence, expected.sequence, scenario.name);
      assert.equal(actual.source.blocks.length, expected.blockCount, scenario.name);
      assert.equal(Object.keys(actual.dataSources).length, expected.dataSourceCount, scenario.name);
    });
    scenario.diagnostics.forEach((code) => {
      assert.equal(result.diagnostics.some((entry) => entry.code === code), true, `${scenario.name}: ${code}`);
    });
  }
});

test('validateForgeReportWorkspaceReferences gates live sources by effective-user catalog', () => {
  const assembly = {
    id: 'brief', sequence: 2,
    source: { datasets: [{ id: 'delivery', kind: 'workspaceRef', dataSourceRef: 'metrics_ad_cube_report' }] },
  };
  assert.deepEqual(validateForgeReportWorkspaceReferences(assembly, ['metrics_ad_cube_report']), []);
  const denied = validateForgeReportWorkspaceReferences(assembly, ['other_source']);
  assert.equal(denied[0].code, 'REPORT_WORKSPACE_REF_DENIED');
  assert.equal(denied[0].dataSourceId, 'delivery');
});

test('assembleForgeReportEvents progressively builds one canonical dashboard source', () => {
  const result = assembleForgeReportEvents([
    { kind: 'data', index: 0, payload: { version: 2, scope: 'campaign', reportRef: 'brief', id: 'rows', sequence: 1, data: [{ spend: 12 }] } },
    { kind: 'report', index: 1, payload: { version: 1, scope: 'campaign', id: 'brief', sequence: 2, mode: 'start', title: 'Delivery', blocks: [{ id: 'summary', kind: 'dashboard.summary', dataSourceRef: 'rows' }] } },
    { kind: 'report', index: 2, payload: { version: 1, scope: 'campaign', id: 'brief', sequence: 3, mode: 'append', blocks: [{ id: 'table', kind: 'dashboard.table', dataSourceRef: 'rows' }] } },
    { kind: 'report', index: 3, payload: { version: 1, scope: 'campaign', id: 'brief', sequence: 4, mode: 'commit' } },
  ]);

  assert.equal(result.assemblies.length, 1);
  assert.equal(result.assemblies[0].grammar, 'dashboard-v1');
  assert.equal(result.assemblies[0].status, 'committed');
  assert.equal(result.assemblies[0].source.blocks.length, 2);
  assert.deepEqual(result.assemblies[0].dataSources.rows.data, [{ spend: 12 }]);
  assert.deepEqual(result.diagnostics, []);
});

test('assembleForgeReportEvents isolates instances and rejects dashboard nested targets atomically', () => {
  const result = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, scope: 'shared', id: 'one', sequence: 1, mode: 'start', blocks: [{ id: 'summary', kind: 'dashboard.summary' }] } },
    { kind: 'report', index: 1, payload: { version: 1, scope: 'shared', id: 'one', sequence: 2, mode: 'append', target: { kind: 'block', ref: 'summary', slot: 'children' }, blocks: [{ id: 'nested', kind: 'dashboard.table' }] } },
    { kind: 'report', index: 2, payload: { version: 1, scope: 'shared', id: 'two', sequence: 1, mode: 'start', blocks: [{ id: 'other', kind: 'dashboard.table' }] } },
  ]);

  assert.equal(result.assemblies.length, 2);
  assert.equal(result.assemblies[0].source.blocks.length, 1);
  assert.equal(result.assemblies[1].source.blocks[0].id, 'other');
  assert.equal(result.diagnostics.some((entry) => entry.code === 'REPORT_TRANSACTION_INVALID'), true);
});

test('assembleForgeReportEvents supports report-document nested targets', () => {
  const result = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'nested', sequence: 1, mode: 'start', grammar: 'report-document-v1', blocks: [{ id: 'group', kind: 'compositeBlock', childBlockIds: [] }] } },
    { kind: 'report', index: 1, payload: { version: 1, id: 'nested', sequence: 2, mode: 'append', target: { kind: 'block', ref: 'group', slot: 'childBlockIds' }, blocks: [{ id: 'table', kind: 'tableBlock' }] } },
  ]);

  assert.deepEqual(result.assemblies[0].source.blocks[0].childBlockIds, ['table']);
  assert.equal(result.assemblies[0].source.blocks[1].id, 'table');
  assert.equal(result.assemblies[0].status, 'committed');
});

test('assembleForgeReportEvents rejects dangling datasources and invalid layout references', () => {
  const dangling = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', blocks: [{ id: 'table', kind: 'dashboard.table', dataSourceRef: 'missing' }] } },
  ]);
  assert.equal(dangling.diagnostics.some((entry) => entry.code === 'REPORT_TRANSACTION_INVALID'), true);

  const layout = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', blocks: [], layout: { items: [{ blockId: 'missing' }] } } },
  ]);
  assert.equal(layout.diagnostics.some((entry) => entry.code === 'REPORT_TRANSACTION_INVALID'), true);
});

test('assembleForgeReportEvents preserves committed sequence and compares raw replay envelopes', () => {
  const committed = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', blocks: [] } },
    { kind: 'report', index: 1, payload: { version: 1, id: 'brief', sequence: 2, mode: 'commit' } },
    { kind: 'report', index: 2, payload: { version: 1, id: 'brief', sequence: 3, mode: 'append', blocks: [] } },
  ]);
  assert.equal(committed.assemblies[0].sequence, 2);
  assert.equal(committed.diagnostics.some((entry) => entry.code === 'REPORT_ALREADY_COMMITTED'), true);

  const replay = assembleForgeReportEvents([
    { kind: 'data', index: 0, payload: { version: 2, reportRef: 'brief', id: 'rows', sequence: 1, extension: 'one', data: [] } },
    { kind: 'data', index: 1, payload: { version: 2, reportRef: 'brief', id: 'rows', sequence: 1, extension: 'two', data: [] } },
    { kind: 'report', index: 2, payload: { version: 1, id: 'brief', sequence: 2, mode: 'start', blocks: [] } },
  ]);
  assert.equal(replay.diagnostics.some((entry) => entry.code === 'REPORT_SEQUENCE_CONFLICT'), true);
});

test('assembleForgeReportEvents supports patch null deletion, array replacement, and explicit removal', () => {
  const result = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', grammar: 'report-document-v1', blocks: [{ id: 'first', kind: 'markdownBlock', title: 'Old', tags: ['a', 'b'] }, { id: 'second', kind: 'calloutBlock' }] } },
    { kind: 'report', index: 1, payload: { version: 1, id: 'brief', sequence: 2, mode: 'patch', blocks: [{ id: 'first', title: null, tags: ['c'] }], removeBlockIds: ['second'] } },
  ]);
  assert.equal(result.diagnostics.length, 0);
  assert.equal(result.assemblies[0].source.blocks.length, 1);
  assert.equal('title' in result.assemblies[0].source.blocks[0], false);
  assert.deepEqual(result.assemblies[0].source.blocks[0].tags, ['c']);
});

test('assembleForgeReportEvents applies datasource append and object patch', () => {
  const arrays = assembleForgeReportEvents([
    { kind: 'data', index: 0, payload: { version: 2, reportRef: 'brief', id: 'rows', sequence: 1, mode: 'replace', data: [{ id: 1 }] } },
    { kind: 'data', index: 1, payload: { version: 2, reportRef: 'brief', id: 'rows', sequence: 2, mode: 'append', data: [{ id: 2 }] } },
    { kind: 'report', index: 2, payload: { version: 1, id: 'brief', sequence: 3, mode: 'start', blocks: [{ id: 'table', kind: 'dashboard.table', dataSourceRef: 'rows' }] } },
  ]);
  assert.deepEqual(arrays.assemblies[0].dataSources.rows.data, [{ id: 1 }, { id: 2 }]);

  const objects = assembleForgeReportEvents([
    { kind: 'data', index: 0, payload: { version: 2, reportRef: 'brief', id: 'summary', sequence: 1, mode: 'replace', data: { spend: 1, nested: { a: 1 } } } },
    { kind: 'data', index: 1, payload: { version: 2, reportRef: 'brief', id: 'summary', sequence: 2, mode: 'patch', data: { spend: 2, nested: { a: null, b: 3 } } } },
    { kind: 'report', index: 2, payload: { version: 1, id: 'brief', sequence: 3, mode: 'start', blocks: [{ id: 'kpi', kind: 'dashboard.summary', dataSourceRef: 'summary' }] } },
  ]);
  assert.deepEqual(objects.assemblies[0].dataSources.summary.data, { spend: 2, nested: { b: 3 } });
});

test('assembleForgeReportEvents treats explicit and implicit sequence gaps identically', () => {
  const explicit = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', blocks: [] } },
    { kind: 'report', index: 1, payload: { version: 1, id: 'brief', sequence: 3, mode: 'commit' } },
  ]);
  const implicit = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 2, mode: 'start', blocks: [] } },
  ]);
  [explicit, implicit].forEach((result) => {
    assert.equal(result.assemblies[0].status, 'incomplete');
    assert.equal(result.diagnostics.some((entry) => entry.code === 'REPORT_SEQUENCE_GAP'), true);
  });
});

test('assembleForgeReportEvents resolves targets against the prior snapshot and permits replace id reuse', () => {
  const target = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', grammar: 'report-document-v1', blocks: [] } },
    { kind: 'report', index: 1, payload: { version: 1, id: 'brief', sequence: 2, mode: 'append', target: { kind: 'block', ref: 'new-group', slot: 'childBlockIds' }, blocks: [{ id: 'new-group', kind: 'compositeBlock', childBlockIds: [] }, { id: 'child', kind: 'tableBlock' }] } },
  ]);
  assert.equal(target.assemblies[0].source.blocks.length, 0);
  assert.equal(target.diagnostics.some((entry) => entry.code === 'REPORT_TRANSACTION_INVALID'), true);

  const replaced = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', grammar: 'report-document-v1', blocks: [{ id: 'same', kind: 'markdownBlock', body: 'old' }] } },
    { kind: 'report', index: 1, payload: { version: 1, id: 'brief', sequence: 2, mode: 'replace', grammar: 'report-document-v1', blocks: [{ id: 'same', kind: 'markdownBlock', body: 'new' }] } },
  ]);
  assert.equal(replaced.diagnostics.length, 0);
  assert.equal(replaced.assemblies[0].source.blocks[0].body, 'new');
  assert.equal(replaced.assemblies[0].resetVersion, 1);
});

test('resetVersion advances only when replace discards interaction state', () => {
  const started = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', grammar: 'report-document-v1', blocks: [{ id: 'same', kind: 'markdownBlock', body: 'initial' }] } },
  ]);
  assert.equal(started.assemblies[0].resetVersion, 0);

  const replacedTwice = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', grammar: 'report-document-v1', blocks: [{ id: 'same', kind: 'markdownBlock', body: 'initial' }] } },
    { kind: 'report', index: 1, payload: { version: 1, id: 'brief', sequence: 2, mode: 'replace', grammar: 'report-document-v1', blocks: [{ id: 'same', kind: 'markdownBlock', body: 'second' }] } },
    { kind: 'report', index: 2, payload: { version: 1, id: 'brief', sequence: 3, mode: 'replace', grammar: 'report-document-v1', blocks: [{ id: 'same', kind: 'markdownBlock', body: 'third' }] } },
  ]);
  assert.equal(replacedTwice.assemblies[0].resetVersion, 2);
});

test('assembleForgeReportEvents rejects unsafe datasource ids and authored credentials', () => {
  const unsafeData = assembleForgeReportEvents([
    { kind: 'data', index: 0, payload: { version: 2, reportRef: 'brief', id: 'rows/other', sequence: 1, data: [] } },
  ]);
  assert.equal(unsafeData.diagnostics[0].code, 'REPORT_DATA_INVALID');

  const secret = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', title: 'Brief', metadata: { authorization: 'Bearer model-authored' }, blocks: [] } },
  ]);
  assert.equal(secret.diagnostics[0].code, 'REPORT_TRANSACTION_INVALID');
  assert.match(secret.diagnostics[0].message, /metadata\.authorization/);
});

test('assembleForgeReportEvents rejects unknown versioned envelope fields', () => {
  const report = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', blocks: [], surprise: true } },
  ]);
  assert.equal(report.diagnostics[0].code, 'REPORT_TRANSACTION_INVALID');
  assert.match(report.diagnostics[0].message, /unknown field "surprise"/);

  const data = assembleForgeReportEvents([
    { kind: 'data', index: 0, payload: { version: 2, reportRef: 'brief', id: 'rows', sequence: 1, data: [], query: 'select *' } },
  ]);
  assert.equal(data.diagnostics[0].code, 'REPORT_DATA_INVALID');
  assert.match(data.diagnostics[0].message, /unknown field "query"/);
});

test('assembleForgeReportEvents rejects unknown kinds and dangling canonical references with actionable diagnostics', () => {
  const unknown = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', blocks: [{ id: 'mystery', kind: 'dashboard.unknown' }] } },
  ]);
  assert.equal(unknown.diagnostics[0].code, 'REPORT_TRANSACTION_INVALID');
  assert.equal(unknown.diagnostics[0].blockId, 'mystery');
  assert.equal(Boolean(unknown.diagnostics[0].suggestedFix), true);

  const dangling = assembleForgeReportEvents([
    { kind: 'report', index: 0, payload: { version: 1, id: 'brief', sequence: 1, mode: 'start', grammar: 'report-document-v1', blocks: [{ id: 'group', kind: 'compositeBlock', childBlockIds: ['missing'] }] } },
  ]);
  assert.equal(dangling.diagnostics.some((entry) => entry.code === 'REPORT_TRANSACTION_INVALID'), true);
});

test('applyForgeDataBlocks supports replace and append', () => {
  const store = applyForgeDataBlocks([
    {
      version: 1,
      id: 'rows',
      format: 'json',
      mode: 'replace',
      data: [{ id: 1 }, { id: 2 }],
    },
    {
      version: 1,
      id: 'rows',
      format: 'json',
      mode: 'append',
      data: [{ id: 3 }],
    },
  ]);

  assert.equal(Array.isArray(store.rows.rows), true);
  assert.deepEqual(store.rows.rows, [{ id: 1 }, { id: 2 }, { id: 3 }]);
});

test('planner submit payload reports selected and changed rows', () => {
  const ui = validateForgeUIBlock(forgeFenceSample.ui);
  const block = ui.blocks[0];
  const originalRows = applyForgeDataBlocks(forgeFenceSample.data).recommended_sites.rows;
  const currentRows = originalRows.map((row, index) => (
    index === 1 ? { ...row, selected: false } : row
  ));

  const payload = createPlannerTableSubmitPayload(ui, block, currentRows, originalRows);

  assert.equal(payload.eventName, 'planner_table_submit');
  assert.equal(payload.tableId, 'site-review');
  assert.equal(payload.dataSourceRef, 'recommended_sites');
  assert.equal(payload.selectionField, 'selected');
  assert.equal(payload.callback.type, 'llm_event');
  assert.equal(payload.selectedRows.length, 2);
  assert.equal(payload.unselectedRows.length, 1);
  assert.equal(payload.changedRows.length, 1);
  assert.equal(payload.finalDataSourceSnapshot.length, 3);
  assert.equal(payload.changedRows[0].site_id, 202);
});

test('planner submit payload filters selected rows to workspace-declared keys', () => {
  const ui = validateForgeUIBlock({
    version: 1,
    title: 'Planner',
    blocks: [{
      id: 'site-review',
      kind: 'planner.table',
      dataSourceRef: 'recommended_sites',
      selection: { mode: 'checkbox', field: 'selected' },
      actions: [{
        id: 'submit-sites',
        kind: 'submit',
        label: 'Submit changes',
        callback: {
          type: 'llm_event',
          eventName: 'planner_table_submit',
          context: {
            domain: 'site_list',
            submitIntent: 'submit_selected',
            allowedSubmitIntents: ['submit_selected', 'preview_selected'],
            selectedKeys: ['site_id', 'recommendation_patch'],
            toolGuidance: {
              tool: 'steward-RecommendationPatch',
              useSelectedRowsOnly: true,
            },
          },
        },
      }],
    }],
  });
  const block = ui.blocks[0];
  const originalRows = [
    { site_id: 101, recommendation_patch: { op: 'add' }, rationale: 'keep', selected: true },
    { site_id: 202, recommendation_patch: { op: 'cut' }, rationale: 'drop', selected: false },
  ];

  const payload = createPlannerTableSubmitPayload(ui, block, originalRows, originalRows);

  assert.deepEqual(payload.plannerSubmit, {
    domain: 'site_list',
    submitIntent: 'submit_selected',
    allowedSubmitIntents: ['submit_selected', 'preview_selected'],
    selectedKeys: ['site_id', 'recommendation_patch'],
    toolGuidance: {
      tool: 'steward-RecommendationPatch',
      useSelectedRowsOnly: true,
    },
  });
  assert.deepEqual(payload.selectedRows, [
    { site_id: 101, recommendation_patch: { op: 'add' } },
  ]);
  assert.deepEqual(payload.selectedRowsRaw, [
    { site_id: 101, recommendation_patch: { op: 'add' }, rationale: 'keep', selected: true },
  ]);
});

test('validateForgeUIBlock defaults missing version to 1', () => {
  const ui = validateForgeUIBlock({ title: 'My Dash', blocks: [] });
  assert.equal(ui.version, 1);
  assert.equal(ui.title, 'My Dash');
  assert.deepEqual(ui.blocks, []);
});

test('validateForgeUIBlock tolerates missing title and missing blocks', () => {
  const ui = validateForgeUIBlock({ version: 1 });
  assert.equal(ui.title, '');
  assert.deepEqual(ui.blocks, []);
});

test('validateForgeUIBlock still rejects a non-object payload', () => {
  assert.throws(() => validateForgeUIBlock('not an object'), /forge-ui block must be an object/);
});

test('validateForgeDataBlock defaults missing version and infers format', () => {
  const block = validateForgeDataBlock({ id: 'rows', data: [{ a: 1 }] });
  assert.equal(block.version, 1);
  assert.equal(block.format, 'json');
  assert.equal(block.mode, 'replace');
});

test('validateForgeDataBlock still requires id', () => {
  assert.throws(() => validateForgeDataBlock({ version: 1, data: [] }), /forge-data\.id is required/);
});

test('rowsToCsv exports labeled recommendation rows', () => {
  const csv = rowsToCsv(
    [
      { site_id: 101, site_name: 'example.com', reason: 'Strong overlap', selected: true },
    ],
    [
      { key: 'site_id', label: 'Site ID' },
      { key: 'site_name', label: 'Site name' },
      { key: 'reason', label: 'Why recommended' },
    ],
  );

  assert.match(csv, /^Site ID,Site name,Why recommended/m);
  assert.match(csv, /101,example\.com,Strong overlap/);
});
