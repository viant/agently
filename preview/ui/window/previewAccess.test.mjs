import assert from 'node:assert/strict';
import {collectPreviewAccessOptions,withPreviewPrincipal,withPreviewTableMode,withPreviewTableWidth,authoredPreviewTableMode,authoredPreviewTableWidth} from './previewAccess.js';
const metadata={authorizationSnapshot:{principal:{roles:['REVIEWER'],features:['EXPOSE_A']},resource:{capabilities:{write:false,read:true}}},view:{items:[{visibleWhen:{source:'authorization',field:'principal.roles',contains:'OPERATOR'}},{visibleWhen:{field:'principal.features',contains:'EXPOSE_B'}}]}};
assert.deepEqual(collectPreviewAccessOptions(metadata),{roles:['OPERATOR','REVIEWER'],features:['EXPOSE_A','EXPOSE_B']});
const updated=withPreviewPrincipal(metadata,{roles:[],features:['EXPOSE_B']});
assert.deepEqual(updated.authorizationSnapshot.principal.roles,[]);
assert.equal(updated.authorizationSnapshot.resource,metadata.authorizationSnapshot.resource);
assert.deepEqual(metadata.authorizationSnapshot.principal.roles,['REVIEWER']);
assert.equal(withPreviewPrincipal(updated,{roles:[],features:['EXPOSE_B']}),updated);
console.log('Preview option discovery, principal-only updates and stable reapplication passed.');

const compact={view:{content:{table:{columns:[{id:'id'}]}}},dataSource:{rows:[1,2,3]}};
const reserve=withPreviewTableMode(compact,'reserve10');
assert.equal(reserve.view.content.table.minRows,10);
assert.equal(reserve.dataSource,compact.dataSource);
assert.equal(withPreviewTableMode(reserve,'reserve10'),reserve);
assert.equal(withPreviewTableMode(reserve,'compact').view.content.table.minRows,0);
assert.equal(withPreviewTableMode(reserve,'compact').view.content.table.rowHeight,undefined);

const trailing=withPreviewTableWidth(reserve,'trailing-space');
assert.equal(trailing.view.content.table.fillRemainingWidth,true);
assert.equal(trailing.view.content.table.minRows,10);
assert.deepEqual(withPreviewTableWidth(trailing,'adaptive'),reserve);
assert.equal(withPreviewTableMode(withPreviewTableWidth(trailing,'adaptive'),'compact').view.content.table.minRows,0);

const authored={view:{content:{table:{columns:[],minRows:10,rowHeight:32}}}};
assert.equal(authoredPreviewTableMode(authored),'reserve10');
assert.equal(authoredPreviewTableMode(compact),'compact');
assert.equal(withPreviewTableMode(authored,null),authored);
const comparison=withPreviewTableMode(authored,'compact');
assert.equal(comparison.view.content.table.minRows,0);
assert.equal(comparison.view.content.table.rowHeight,undefined);
assert.equal(authored.view.content.table.minRows,10);
assert.equal(withPreviewTableMode(comparison,'compact'),comparison);
assert.equal(withPreviewTableMode(comparison,'reserve10').view.content.table.minRows,10);

const authoredWidth={view:{content:{table:{columns:[],fillRemainingWidth:true}}}};
assert.equal(authoredPreviewTableWidth(authoredWidth),'trailing-space');
assert.equal(withPreviewTableWidth(authoredWidth,null),authoredWidth);
assert.equal(withPreviewTableWidth(authoredWidth,'adaptive').view.content.table.fillRemainingWidth,undefined);
assert.equal(authoredWidth.view.content.table.fillRemainingWidth,true);
