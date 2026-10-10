// Source-only browser interoperability proof. Uses disposable loopback servers
// and Playwright's isolated Chromium profile, never a live application session.
import assert from 'node:assert/strict';
import http from 'node:http';
import { createRequire } from 'node:module';
import { build } from 'esbuild';
const require = createRequire(import.meta.url);
const { chromium } = require('playwright');
const servers = [];
let browser;
async function serve(handler) {
  const server = http.createServer(handler);
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  servers.push(server);
  return `http://127.0.0.1:${server.address().port}`;
}
async function bundle(source) {
  const out = await build({ stdin: { contents: source, resolveDir: process.cwd(), sourcefile: 'mcp-apps-audit.js' }, bundle: true, write: false, format: 'iife', platform: 'browser', target: 'es2022', minify: true });
  return out.outputFiles[0].text.replaceAll('</script', '<\\/script');
}
const html = script => `<!doctype html><html><head><meta charset="utf-8"></head><body><script>${script}</script></body></html>`;
try {
  let blockedRequests = 0;
  let hostScript = '';
  const hostOrigin = await serve((req, res) => {
    if (req.url === '/blocked') blockedRequests++;
    res.setHeader('Content-Type', 'text/html'); res.end(html(hostScript));
  });
  const apiOrigin = await serve((_req, res) => { res.setHeader('Access-Control-Allow-Origin', '*'); res.setHeader('Content-Type', 'application/json'); res.end('{"allowed":true}'); });
  const proxyScript = await bundle(`import './src/services/mcpApps/sandboxProxy.js';`);
  const sandboxOrigin = await serve((_req, res) => { res.setHeader('Content-Type', 'text/html'); res.end(html(proxyScript)); });
  const viewScript = await bundle(`
    import { App } from '@modelcontextprotocol/ext-apps';
    void (async () => {
    window.audit = { events: [], done: false };
    const app = new App({name:'isolated-audit-view',version:'1'}, {}, {autoResize:false});
    app.ontoolinputpartial = () => window.audit.events.push('partial');
    app.ontoolinput = () => window.audit.events.push('input');
    app.ontoolresult = result => { window.audit.events.push('result'); window.audit.result = result.content[0].text; };
    app.onteardown = async () => { window.audit.teardown = true; return {}; };
    await app.connect();
    window.audit.tool = (await app.callServerTool({name:'fixture-refresh',arguments:{}})).content[0].text;
    window.audit.displayMode = (await app.requestDisplayMode({mode:'fullscreen'})).mode;
    window.audit.allowedNetwork = (await (await fetch(${JSON.stringify(apiOrigin)})).json()).allowed;
    try { await fetch(${JSON.stringify(hostOrigin + '/blocked')}); window.audit.deniedNetwork = false; }
    catch { window.audit.deniedNetwork = true; }
    try { void window.parent.document.body; window.audit.opaque = false; }
    catch { window.audit.opaque = true; }
    window.audit.done = true;
    })().catch(error => { window.audit.error = String(error); });
  `);
  const resource = { html: html(viewScript), csp: { connectDomains: [apiOrigin] } };
  hostScript = await bundle(`
    import { createStandardAppHost } from './src/services/mcpApps/standardHost.js';
    window.auditHost = { calls: 0, disposed: false };
    const frame = document.createElement('iframe');
    frame.setAttribute('sandbox','allow-scripts allow-same-origin'); document.body.appendChild(frame);
    const proxy = { call: async (method, params) => {
      if(method !== 'tools/call' || params.name !== 'fixture-refresh') throw Error('unrequested fixture method');
      window.auditHost.calls++; return {content:[{type:'text',text:'fixture-tool-result'}]};
    }, dispose: () => { window.auditHost.disposed = true; } };
    const host = createStandardAppHost({proxy, target:frame.contentWindow, origin:${JSON.stringify(sandboxOrigin)}, resource:${JSON.stringify(resource)}});
    window.auditClose = () => host.close();
    void (async () => {
      await host.connect();
      window.auditDelivery = Promise.all([host.sendPartialInput({query:'part'}),host.sendInput({query:'complete'}),host.sendResult({content:[{type:'text',text:'original-result'}]})]);
      frame.src = ${JSON.stringify(sandboxOrigin + '/#' + new URLSearchParams({ hostOrigin }).toString())};
    })();
  `);
  browser = await chromium.launch({ headless: true, ...(process.env.MCP_APPS_CHROME_EXECUTABLE ? { executablePath: process.env.MCP_APPS_CHROME_EXECUTABLE } : {}) });
  const page = await browser.newPage();
  await page.goto(hostOrigin);
  const view = await new Promise((resolve, reject) => {
    const deadline = Date.now() + 10000;
    const check = async () => {
      const candidate = page.frames().find(frame => frame.url() === 'about:srcdoc');
      if (candidate) { resolve(candidate); return; }
      if (Date.now() > deadline) { reject(Error('nested app view unavailable')); return; }
      setTimeout(check, 20);
    };
    void check();
  });
  await view.waitForFunction(() => window.audit?.done === true, { timeout: 10000 });
  await page.evaluate(() => window.auditDelivery);
  const state = await view.evaluate(() => window.audit);
  assert.deepEqual(state.events, ['partial', 'input', 'result']);
  assert.equal(state.result, 'original-result'); assert.equal(state.tool, 'fixture-tool-result');
  assert.equal(state.displayMode, 'inline'); assert.equal(state.allowedNetwork, true);
  assert.equal(state.deniedNetwork, true); assert.equal(state.opaque, true); assert.equal(blockedRequests, 0);
  await page.evaluate(() => window.auditClose());
  assert.equal(await view.evaluate(() => window.audit.teardown), true);
  assert.deepEqual(await page.evaluate(() => window.auditHost), { calls: 1, disposed: true });
  process.stdout.write('MCP Apps Chromium proof passed: official handshake, ordered data, tool proxy, display mode, opaque sandbox, declared/denied network, teardown.\n');
} finally {
  await browser?.close();
  await Promise.all(servers.map(server => new Promise(resolve => server.close(resolve))));
}
