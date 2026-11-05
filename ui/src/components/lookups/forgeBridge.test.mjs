// Pure-function test for forgeBridge. No DOM, no network.
import assert from 'node:assert/strict';
import { translateSchema, extractLookupBindings } from './forgeBridge.js';

const serverEmittedSchema = {
  properties: {
    record_id: {
      type: 'integer',
      title: 'Record',
      'x-ui-widget': 'lookup',
      'x-ui-lookup': {
        dataSource: 'record',
        dialogId: 'recordPicker',
        title: 'Record list',
        queryInput: 'q',
        resolveInput: 'id',
        inputs: [],
        outputs: [
          { location: 'id', name: 'record_id' },
          { location: 'name', name: 'record_name' },
        ],
        display: '${name} (#${id})',
      },
    },
    note: { type: 'string' },
  },
};

translateSchema(serverEmittedSchema);
assert.ok(
  serverEmittedSchema.properties.record_id.lookup,
  'lookup metadata attached to property'
);
const lk = serverEmittedSchema.properties.record_id.lookup;
assert.equal(lk.dialogId, 'recordPicker');
assert.equal(lk.dataSource, 'record');
assert.equal(lk.title, 'Record list');
assert.equal(lk.queryInput, 'q');
assert.equal(lk.resolveInput, 'id');
assert.equal(lk.outputs.length, 2);
console.log('translateSchema ✓ attaches item.lookup from x-ui-lookup');

const bindings = extractLookupBindings(serverEmittedSchema);
assert.deepEqual(bindings, [
  { property: 'record_id', dataSource: 'record' },
]);
console.log('extractLookupBindings ✓ yields property↔datasource pairs');

// Non-lookup field is left alone
assert.equal(serverEmittedSchema.properties.note.lookup, undefined);
console.log('non-lookup fields untouched ✓');

console.log('\nFORGE BRIDGE TESTS PASSED');
