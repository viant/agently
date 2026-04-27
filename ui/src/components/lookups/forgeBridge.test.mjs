// Pure-function test for forgeBridge. No DOM, no network.
import assert from 'node:assert/strict';
import { translateSchema, extractLookupBindings } from './forgeBridge.js';

const serverEmittedSchema = {
  properties: {
    advertiser_id: {
      type: 'integer',
      title: 'Advertiser',
      'x-ui-widget': 'lookup',
      'x-ui-lookup': {
        dataSource: 'advertiser',
        dialogId: 'advertiserPicker',
        title: 'Advertiser list',
        queryInput: 'q',
        resolveInput: 'id',
        inputs: [],
        outputs: [
          { location: 'id', name: 'advertiser_id' },
          { location: 'name', name: 'advertiser_name' },
        ],
        display: '${name} (#${id})',
      },
    },
    note: { type: 'string' },
  },
};

translateSchema(serverEmittedSchema);
assert.ok(
  serverEmittedSchema.properties.advertiser_id.lookup,
  'lookup metadata attached to property'
);
const lk = serverEmittedSchema.properties.advertiser_id.lookup;
assert.equal(lk.dialogId, 'advertiserPicker');
assert.equal(lk.dataSource, 'advertiser');
assert.equal(lk.title, 'Advertiser list');
assert.equal(lk.queryInput, 'q');
assert.equal(lk.resolveInput, 'id');
assert.equal(lk.outputs.length, 2);
console.log('translateSchema ✓ attaches item.lookup from x-ui-lookup');

const bindings = extractLookupBindings(serverEmittedSchema);
assert.deepEqual(bindings, [
  { property: 'advertiser_id', dataSource: 'advertiser' },
]);
console.log('extractLookupBindings ✓ yields property↔datasource pairs');

// Non-lookup field is left alone
assert.equal(serverEmittedSchema.properties.note.lookup, undefined);
console.log('non-lookup fields untouched ✓');

console.log('\nFORGE BRIDGE TESTS PASSED');
