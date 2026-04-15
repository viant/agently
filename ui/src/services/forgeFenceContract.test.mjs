import test from 'node:test';
import assert from 'node:assert/strict';

import {
  applyForgeDataBlocks,
  createPlannerTableSubmitPayload,
  forgeFenceSample,
  rowsToCsv,
  validateForgeUIBlock,
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
