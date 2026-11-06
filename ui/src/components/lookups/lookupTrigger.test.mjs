import assert from 'node:assert/strict';
import {filterLookupRegistry, findLookupTriggerStart, shouldClearSoleLookupTrigger} from './lookupTrigger.js';
import { it } from 'vitest';

it('lookupTrigger contract', () => {

  assert.equal(findLookupTriggerStart('/project'), 0);
  assert.equal(findLookupTriggerStart('open /project'), 5);
  assert.equal(findLookupTriggerStart('open (/order'), 6);
  assert.equal(findLookupTriggerStart('https://example.com/path'), -1);
  assert.equal(findLookupTriggerStart('folder/name'), -1);
  assert.equal(shouldClearSoleLookupTrigger({value: '/', key: 'Backspace', selectionStart: 1, selectionEnd: 1}), true);
  assert.equal(shouldClearSoleLookupTrigger({value: '/project', key: 'Backspace', selectionStart: 9, selectionEnd: 9}), false);
  assert.equal(shouldClearSoleLookupTrigger({value: '/', key: 'Escape', selectionStart: 1, selectionEnd: 1}), false);
  assert.equal(shouldClearSoleLookupTrigger({value: '/', key: 'Backspace', selectionStart: 0, selectionEnd: 0}), false);

  const registry = [
    {name: 'order', title: 'Work Order'},
    {name: 'project_list', title: 'Project List'},
    {name: 'project', title: 'Project'},
    {name: 'account', description: 'Record account selector'},
  ];

  assert.deepEqual(filterLookupRegistry(registry, 'proj').map((entry) => entry.name), ['project_list', 'project']);
  assert.deepEqual(filterLookupRegistry(registry, 'rec').map((entry) => entry.name), ['account']);
  assert.deepEqual(filterLookupRegistry(registry, 'work order').map((entry) => entry.name), ['order']);
  assert.deepEqual(filterLookupRegistry(registry, 'missing'), []);

});
