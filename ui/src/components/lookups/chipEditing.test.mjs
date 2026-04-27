import assert from 'node:assert/strict';
import { applyResolvedChipToken, createEditingChipState, shouldSkipEditorSync } from './chipEditing.js';

const resolvedChip = createEditingChipState({
  raw: '@{order:7 "Order 7"}',
  name: 'order',
  id: '7',
  unresolved: false,
});
assert.deepEqual(resolvedChip, {
  raw: '@{order:7 "Order 7"}',
  name: 'order',
  value: '7',
  error: '',
});
console.log('createEditingChipState ✓ preserves resolved chip identity');

const unresolvedChip = createEditingChipState({
  raw: '@{order:? "?order"}',
  name: 'order',
  id: '?',
  unresolved: true,
});
assert.equal(unresolvedChip.value, '');
console.log('createEditingChipState ✓ clears value for unresolved chip');

const good = applyResolvedChipToken(
  'Troubleshoot @{order:7 "Order 7"} for pacing.',
  '@{order:7 "Order 7"}',
  '@{order:42 "Northwind - Retargeting"}'
);
assert.equal(good.ok, true);
assert.equal(good.nextStored, 'Troubleshoot @{order:42 "Northwind - Retargeting"} for pacing.');
console.log('applyResolvedChipToken ✓ replaces chip token in-place');

const missingLabel = applyResolvedChipToken(
  'Troubleshoot @{order:7 "Order 7"} for pacing.',
  '@{order:7 "Order 7"}',
  '@{order:42 ""}'
);
assert.equal(missingLabel.ok, false);
assert.equal(missingLabel.error, 'The lookup resolved, but no display label was emitted.');
console.log('applyResolvedChipToken ✓ rejects tokens without a display label');

const fallback = applyResolvedChipToken(
  'Troubleshoot @{order:7 "Order 7"} for pacing.',
  '@{order:missing "Missing"}',
  '@{order:42 "Northwind - Retargeting"}'
);
assert.equal(fallback.ok, true);
assert.equal(fallback.nextStored, '@{order:42 "Northwind - Retargeting"}');
console.log('applyResolvedChipToken ✓ falls back to fresh token when raw token is missing');

assert.equal(
  shouldSkipEditorSync({
    editingChip: {
      raw: '@{order:? "Order"}',
      name: 'order',
    },
    currentStored: 'Troubleshoot @{order:? "Order"} for pacing.',
    lastSyncedValue: 'Troubleshoot @{order:? "Order"} for pacing.',
    nextValue: 'Troubleshoot @{order:? "Order"} for pacing.',
    hasChipEditor: true,
    activeChipRaw: '@{order:? "Order"}',
  }),
  true
);
console.log('shouldSkipEditorSync ✓ preserves active chip editor while stored value is unchanged');

assert.equal(
  shouldSkipEditorSync({
    editingChip: {
      raw: '@{order:? "Order"}',
      name: 'order',
    },
    currentStored: 'Troubleshoot @{order:? "Order"} for pacing.',
    lastSyncedValue: 'Troubleshoot @{order:? "Order"} for pacing.',
    nextValue: 'Troubleshoot @{order:? "Order"} for pacing.',
    hasChipEditor: true,
    activeChipRaw: '@{campaign:? "Campaign"}',
  }),
  false
);
console.log('shouldSkipEditorSync ✓ resyncs when a different chip editor is mounted');

assert.equal(
  shouldSkipEditorSync({
    editingChip: null,
    currentStored: 'Troubleshoot @{order:42 "Northwind"} for pacing.',
    lastSyncedValue: 'Troubleshoot @{order:42 "Northwind"} for pacing.',
    nextValue: 'Troubleshoot @{order:42 "Northwind"} for pacing.',
    hasChipEditor: false,
  }),
  true
);
console.log('shouldSkipEditorSync ✓ skips when DOM and state already match');

assert.equal(
  shouldSkipEditorSync({
    editingChip: null,
    currentStored: 'Troubleshoot @{order:42 "Northwind"} for pacing.',
    lastSyncedValue: 'Troubleshoot @{order:42 "Northwind"} for pacing.',
    nextValue: 'Troubleshoot @{order:42 "Northwind"} for pacing.',
    hasChipEditor: true,
  }),
  false
);
console.log('shouldSkipEditorSync ✓ forces resync when chip editor DOM is still present');

console.log('\nCHIP EDITING TESTS PASSED');
