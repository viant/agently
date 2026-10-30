import assert from 'node:assert/strict';
import {criticalModulePreload} from './criticalModulePreload.mjs';
const plugin=criticalModulePreload();
plugin.configResolved({base:'/'});
const context={filename:'/example/index.html',bundle:{
 renderer:{type:'chunk',isEntry:false,facadeModuleId:'/example/src/components/chat/RichContent.jsx',fileName:'assets/RichContent-current.js'},
 main:{type:'chunk',isEntry:true,facadeModuleId:'/example/src/main.jsx',fileName:'assets/main.js'},
 other:{type:'chunk',isEntry:false,facadeModuleId:'/example/src/other.jsx',fileName:'assets/other.js'},
}};
assert.deepEqual(plugin.transformIndexHtml.handler('',context),[{tag:'link',attrs:{rel:'modulepreload',crossorigin:'',href:'/assets/RichContent-current.js'},injectTo:'head'}]);
assert.deepEqual(plugin.transformIndexHtml.handler('',{filename:'/example/index.html'}),[]);
assert.deepEqual(plugin.transformIndexHtml.handler('',{...context,filename:'/example/mcp-app-sandbox.html'}),[]);
console.log('critical renderer preload checks passed');
