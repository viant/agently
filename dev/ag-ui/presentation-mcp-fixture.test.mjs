import {test} from 'node:test';
import assert from 'node:assert/strict';
import {feedResult} from './presentation-mcp-fixture.mjs';
test('synthetic feed updates keep identity without host or remote datasource authority',()=>{
 const initial=feedResult(1),updated=feedResult(2);
 const first=JSON.parse(initial.content[0].text),second=JSON.parse(updated.content[0].text);
 assert.equal(first.rows[0].id,second.rows[0].id);
 assert.equal(first.rows[0].value,'Initial fixture value');
 assert.equal(second.rows[0].value,'Updated fixture value');
 assert.equal(initial._meta,undefined);assert.equal(updated._meta,undefined);
});
