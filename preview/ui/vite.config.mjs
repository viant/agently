import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
import {dirname,resolve} from 'node:path';
// Reuse agently/ui's dependency graph; Forge supplies only renderer libraries.
const here=dirname(fileURLToPath(import.meta.url));
const uiRoot=resolve(here,'../../ui');
const require=createRequire(resolve(uiRoot,'package.json'));
const reactModule=require('@vitejs/plugin-react');
const react=reactModule.default||reactModule;
const forgeRoot=resolve(here,'../../../forge');
export default {
 root:here,
 plugins:[react()],
 resolve:{dedupe:['react','react-dom','@preact/signals-react'],alias:[
  {find:'forge-runtime',replacement:resolve(forgeRoot,'index.js')},
  {find:'forge',replacement:resolve(forgeRoot,'src')},
  {find:'react',replacement:resolve(uiRoot,'node_modules/react')},
  {find:'react-dom',replacement:resolve(uiRoot,'node_modules/react-dom')},
  {find:'@preact/signals-react',replacement:resolve(uiRoot,'node_modules/@preact/signals-react')},
  {find:'@blueprintjs/core',replacement:resolve(uiRoot,'node_modules/@blueprintjs/core')},
  {find:'@blueprintjs/icons',replacement:resolve(uiRoot,'node_modules/@blueprintjs/icons')},
 ]},
 build:{outDir:resolve(here,'dist'),emptyOutDir:true,rollupOptions:{input:{
  windowPreview:resolve(here,'window-preview.html'),
  reportPreview:resolve(here,'report-preview.html'),
 }}},
};
