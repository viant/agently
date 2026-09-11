import assert from 'node:assert/strict';
import {readRoute,windowURL} from './route.js';
const route=readRoute('?window=project-tasks&parameters=%7B%22projectId%22%3A2%7D');
assert.equal(route.windowKey,'project-tasks');assert.deepEqual(route.parameters,{projectId:2});
assert.throws(()=>readRoute('?parameters=[]'));
const url=windowURL('http://localhost/?window=projects&variant=empty&query=%7B%7D',{windowKey:'project-tasks',parameters:{projectId:2}});
const result=new URL(url,'http://localhost');assert.equal(result.searchParams.get('variant'),'empty');assert.equal(result.searchParams.has('query'),false);assert.deepEqual(readRoute(result.search).parameters,{projectId:2});
assert.equal(readRoute('', 'projects').windowKey,'projects');
console.log('Window preview routes passed');
