import test from 'node:test';
import assert from 'node:assert/strict';

import {
  applyForgeDataBlocks,
  createPlannerTableSubmitPayload,
  forgeFenceSample,
  rowsToCsv,
  validateForgeUIBlock,
  validateForgeDataBlock,
} from './forgeFenceContract.js';

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
