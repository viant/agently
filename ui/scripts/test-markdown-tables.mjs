import assert from 'node:assert/strict';
import { looksLikePipeTable, parsePipeTable, findNextPipeTableBlock } from '../src/components/markdownTableUtils.js';

const cases = [
  {
    name: 'simple table detection',
    input: `
| A | B |
|---|---|
|  1|  2|
`,
    wantLooks: true,
    wantHeaders: ['A', 'B'],
    wantFirstRow: ['1', '2'],
  },
  {
    name: 'aligned table detection',
    input: `
| Name | Score | Note |
|:-----|:-----:|-----:|
| Foo  |  10   |  ok  |
`,
    wantLooks: true,
    wantHeaders: ['Name', 'Score', 'Note'],
    wantFirstRow: ['Foo', '10', 'ok'],
  },
];

for (const tc of cases) {
  assert.equal(looksLikePipeTable(tc.input), tc.wantLooks, tc.name + ' looksLikePipeTable');
  const { headers, rows } = parsePipeTable(tc.input);
  assert.deepEqual(headers, tc.wantHeaders, tc.name + ' headers');
  assert.deepEqual(rows[0], tc.wantFirstRow, tc.name + ' first row');
}

const mixed = `
### Heading
Some prose above.

| Metric | Legacy | DQL |
|-------:|:------:|:---:|
| A      |    1   |  1  |

More prose…
`;

const block = findNextPipeTableBlock(mixed, 0);
assert.ok(block && block.start >= 0 && block.end > block.start, 'find block in mixed content');
const tableBody = mixed.slice(block.start, block.end);
const p = parsePipeTable(tableBody);
assert.deepEqual(p.headers, ['Metric', 'Legacy', 'DQL']);
assert.deepEqual(p.rows[0], ['A', '1', '1']);

console.log('ok');

