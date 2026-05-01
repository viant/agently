// Direct-execution test mirroring lookups-test.mjs T12-T17 for the actual
// component module.  Run:
//   node src/components/lookups/tokens.test.mjs
// (uses Node's ESM resolution; no test runner required)
import assert from 'node:assert/strict';
import {
  serializeManualToken,
  serializeToken,
  parseTokens,
  parseAuthored,
  flatten,
  flattenStored,
  rehydrate,
} from './tokens.js';

const registry = [
  {
    name: 'advertiser',
    dataSource: 'advertiser',
    required: true,
    display: '${name} (#${id})',
    token: { store: '${id}', display: '${name}', modelForm: '${id}' },
  },
];

const adOrderRegistry = [
  {
    name: 'order',
    dataSource: 'orders',
    required: true,
    display: '${adOrderName}',
    token: { store: '${adOrderId}', display: '${adOrderName}', modelForm: '${adOrderId}', resolveInput: 'id' },
  },
];

// T12 — Rich token roundtrip
const picked = { id: 789, name: 'Globex', region: 'EMEA' };
const tok = serializeToken(registry[0], picked);
assert.equal(tok, '@{advertiser:789 "Globex"}');
const stored = `Analyze performance for ${tok} in Q4.`;
const parsed = parseTokens(stored);
assert.equal(parsed.length, 1);
assert.deepEqual(
  { name: parsed[0].name, id: parsed[0].id, label: parsed[0].label },
  { name: 'advertiser', id: '789', label: 'Globex' }
);
console.log('T12 ✓ parse(serialize(x)) = x');

// T15 — Rehydrate from stored text (no re-fetch)
const segs = rehydrate(stored);
assert.equal(segs.length, 3);
assert.equal(segs[0].kind, 'text');
assert.equal(segs[1].kind, 'chip');
assert.equal(segs[1].label, 'Globex');
assert.equal(segs[1].id, '789');
console.log('T15 ✓ rehydrate produces chip segment with id+label from storage');

// T17 — Authored /name parse → flatten
const authored = 'Analyze performance for /advertiser in Q4.';
const parts = parseAuthored(authored, registry);
assert.equal(parts.length, 3);
assert.equal(parts[1].kind, 'picker');
assert.throws(() => flatten(parts), /unresolved required/);

parts[1].resolved = picked;
assert.equal(flatten(parts), 'Analyze performance for 789 in Q4.');
console.log('T17 ✓ chip visible to user; id-only goes to LLM');

// flattenStored — stored rich string → LLM send-time string
const sent = flattenStored(stored, registry);
assert.equal(sent, 'Analyze performance for 789 in Q4.');
console.log('flattenStored ✓ labels stripped from stored form for LLM send');

// Escape handling in label
const weird = serializeToken(registry[0], {
  id: 1,
  name: 'Acme "Inc" & Co',
});
const backParsed = parseTokens(weird);
assert.equal(backParsed.length, 1);
assert.equal(backParsed[0].label, 'Acme "Inc" & Co');
console.log('escape ✓ label with quotes roundtrips');

const manualToken = serializeManualToken('order', '1232');
assert.equal(manualToken, '@{order:1232 "1232"}');
console.log('manual-token ✓ direct identity stores explicit id/label');

const adOrderToken = serializeToken(adOrderRegistry[0], {
  adOrderId: 2637583,
  adOrderName: 'Northwind - Retargeting',
});
const adOrderParsed = parseTokens(adOrderToken);
assert.equal(adOrderParsed.length, 1);
assert.equal(adOrderParsed[0].id, '2637583');
assert.equal(adOrderParsed[0].label, 'Northwind - Retargeting');
console.log('display-template ✓ adOrderName resolves into stored chip label');

// Authored-placeholder roundtrip: NamedLookupInput emits @{name:? "?name"};
// parseTokens must match it, rehydrate must flag it unresolved, flattenStored
// must throw for required bindings.  (tokens.js already imported at top.)
import { isUnresolvedToken } from './tokens.js';
const authoredStored = 'Analyze @{advertiser:? "?advertiser"} in Q4.';
const authoredParsed = parseTokens(authoredStored);
assert.equal(authoredParsed.length, 1);
assert.equal(authoredParsed[0].id, '?');
assert.ok(isUnresolvedToken(authoredParsed[0]));
console.log('authored-placeholder ✓ parser matches @{name:? "?name"}');

const rehydrated = rehydrate(authoredStored);
const chip = rehydrated.find((s) => s.kind === 'chip');
assert.ok(chip && chip.unresolved === true);
assert.equal(chip.label, '?advertiser');
console.log('authored-placeholder ✓ rehydrate flags chip unresolved');

assert.throws(
  () => flattenStored(authoredStored, registry),
  /unresolved required/
);
console.log('authored-placeholder ✓ flattenStored blocks required unresolved');

assert.equal(
  flattenStored(authoredStored, registry, { allowUnresolvedRequired: true }),
  'Analyze /advertiser in Q4.',
);
console.log('authored-placeholder ✓ flattenStored can preserve unresolved required token for submit-time fallback');

// Non-required unresolved passes through as /name literal.
const nonReqRegistry = [{ ...registry[0], required: false }];
assert.equal(
  flattenStored(authoredStored, nonReqRegistry),
  'Analyze /advertiser in Q4.'
);
console.log('authored-placeholder ✓ non-required unresolved passes through');

console.log('\nALL FRONTEND TOKEN TESTS PASSED');
