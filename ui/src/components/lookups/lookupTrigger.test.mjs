import assert from 'node:assert/strict';
import {filterLookupRegistry, findLookupTriggerStart, shouldClearSoleLookupTrigger} from './lookupTrigger.js';

assert.equal(findLookupTriggerStart('/campaign'), 0);
assert.equal(findLookupTriggerStart('open /campaign'), 5);
assert.equal(findLookupTriggerStart('open (/order'), 6);
assert.equal(findLookupTriggerStart('https://example.com/path'), -1);
assert.equal(findLookupTriggerStart('folder/name'), -1);
assert.equal(shouldClearSoleLookupTrigger({value: '/', key: 'Backspace', selectionStart: 1, selectionEnd: 1}), true);
assert.equal(shouldClearSoleLookupTrigger({value: '/campaign', key: 'Backspace', selectionStart: 9, selectionEnd: 9}), false);
assert.equal(shouldClearSoleLookupTrigger({value: '/', key: 'Escape', selectionStart: 1, selectionEnd: 1}), false);
assert.equal(shouldClearSoleLookupTrigger({value: '/', key: 'Backspace', selectionStart: 0, selectionEnd: 0}), false);

const registry = [
  {name: 'order', title: 'Ad Order'},
  {name: 'campaign_list', title: 'Campaign List'},
  {name: 'campaign', title: 'Campaign'},
  {name: 'account', description: 'Advertiser account selector'},
];

assert.deepEqual(filterLookupRegistry(registry, 'camp').map((entry) => entry.name), ['campaign_list', 'campaign']);
assert.deepEqual(filterLookupRegistry(registry, 'advert').map((entry) => entry.name), ['account']);
assert.deepEqual(filterLookupRegistry(registry, 'ad order').map((entry) => entry.name), ['order']);
assert.deepEqual(filterLookupRegistry(registry, 'missing'), []);

console.log('lookupTrigger ✓ boundary-aware slash activation and ranked matching');
