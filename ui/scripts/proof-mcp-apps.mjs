// Source-only, isolated two-origin browser proof. No backend or workspace config.
import { build } from 'esbuild';
import { chromium } from 'playwright';
import { createServer } from 'node:http';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import assert from 'node:assert/strict';
const uiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
async function bundle(source) {
  const result = await build({ absWorkingDir: uiRoot, stdin: { contents: source, resolveDir: uiRoot, sourcefile: 'mcp-app-proof.js' }, bundle: true, format: 'esm', platform: 'browser', write: false });
  return result.outputFiles[0].text;
}
const guest = await bundle(`import { App } from '@modelcontextprotocol/ext-apps';
 const app = new App({name:'generic-browser-fixture',version:'1'}, {}, {autoResize:false});
 let input, partial;
 app.ontoolinput = value => { input = value.arguments; };
 app.ontoolinputpartial = value => { partial = value.arguments; };
 app.onteardown = async () => ({});
 app.ontoolresult = async result => {
   let evalBlocked=false, parentBlocked=false, networkBlocked=false;
   try { new Function('return 1')(); } catch { evalBlocked=true; }
   try { window.parent.parent.document.body; } catch { parentBlocked=true; }
   try { await fetch('https://undeclared.invalid/resource'); } catch { networkBlocked=true; }
   await app.callServerTool({ name:'refresh_fixture', arguments:{input,partial,hostMeta:result._meta,evalBlocked,parentBlocked,networkBlocked} });
 };
 await app.connect();`);
const sandboxJS = await bundle("import './src/services/mcpApps/sandboxProxy.js';");
let hostJS;
const hostServer = createServer((req, res) => {
  res.setHeader('Content-Type', req.url === '/host.js' ? 'application/javascript' : 'text/html');
  res.end(req.url === '/host.js' ? hostJS : '<!doctype html><iframe id="app" sandbox="allow-scripts allow-same-origin"></iframe><script type="module" src="/host.js"></script>');
});
const sandboxServer = createServer((req, res) => {
  res.setHeader('Content-Type', req.url === '/proxy.js' ? 'application/javascript' : 'text/html');
  res.setHeader('Cache-Control', 'no-store');
  res.end(req.url === '/proxy.js' ? sandboxJS : '<!doctype html><body><script type="module" src="/proxy.js"></script>');
});
const listen = server => new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
await listen(hostServer); await listen(sandboxServer);
const hostOrigin = `http://127.0.0.1:${hostServer.address().port}`;
const sandboxOrigin = `http://127.0.0.1:${sandboxServer.address().port}`;
const html = '<!doctype html><html><head><title>Generic fixture</title></head><body><p>Generic app</p><script type="module">' + guest.replaceAll('</script', '<\\/script') + '</script></body></html>';
hostJS = await bundle(`import { createStandardAppHost } from './src/services/mcpApps/standardHost.js';
 import { resolveSandboxUrl } from './src/services/mcpApps/standardDocument.js';
 const frame=document.querySelector('#app'); window.proof={};
 const proxy={ call: async (method,params) => { window.proof.request={method,params}; return {content:[{type:'text',text:'fixture updated'}]}; }, dispose:()=> {window.proof.disposed=true;} };
 const host=createStandardAppHost({proxy,target:frame.contentWindow,origin:${JSON.stringify(sandboxOrigin)},resource:{html:${JSON.stringify(html)},csp:{}},
 onReady:async()=>{ await host.sendPartialInput({query:'par'}); await host.sendInput({query:'complete'}); await host.sendResult({content:[{type:'text',text:'ready'}],_meta:{fixture:'host-only'}});},
 onError:error=>{window.proof.error=String(error);},onClosed:()=>{window.proof.closed=true;} });
 window.closeApp=()=>host.close(); await host.connect(); frame.src=resolveSandboxUrl(${JSON.stringify(sandboxOrigin + '/sandbox.html')});`);
let browser;
try {
  browser = await chromium.launch({ headless: true, ...(process.env.MCP_APPS_CHROME_EXECUTABLE ? { executablePath: process.env.MCP_APPS_CHROME_EXECUTABLE } : {}) });
  const page = await browser.newPage();
  await page.goto(hostOrigin);
  await page.waitForFunction(() => window.proof?.request || window.proof?.error);
  const proof = await page.evaluate(() => window.proof);
  assert.equal(proof.error, undefined);
  assert.equal(proof.request.method, 'tools/call');
  assert.equal(proof.request.params.name, 'refresh_fixture');
  assert.deepEqual(proof.request.params.arguments, { input: {query:'complete'}, partial: {query:'par'}, hostMeta:{fixture:'host-only'},evalBlocked:true,parentBlocked:true,networkBlocked:true });
  await page.evaluate(() => window.closeApp());
  assert.equal(await page.evaluate(() => window.proof.closed && window.proof.disposed), true);
  console.log('PASS: official App SDK + different-origin proxy + opaque view + CSP + inputs/result + tool call + acknowledged teardown');
} finally {
  await browser?.close();
  await Promise.all([new Promise(resolve=>hostServer.close(resolve)),new Promise(resolve=>sandboxServer.close(resolve))]);
}
