import assert from 'node:assert/strict';

import { isForgeReportingModule, isLoopbackPeer, loopbackOnlyDevPlugin, resolveDevHost, resolveDevPort, resolveProxyTarget } from './vite.config.js';

for (const root of ['/tmp/forge', '/tmp/forge-opening-clean']) {
  assert.equal(isForgeReportingModule(`${root}/src/components/dashboard/ReportBuilder.jsx`, root), true);
  assert.equal(isForgeReportingModule(`${root}/src/reporting/reportFillModel.js`, root), true);
  assert.equal(isForgeReportingModule(`${root}/src/components/WindowContent.jsx`, root), false);
}

import createConfig from './vite.config.js';
import fs from 'node:fs';

assert.equal(
  resolveProxyTarget({ APPSERVER_URL: 'http://127.0.0.1:9191' }, { requireExplicit: true }),
  'http://127.0.0.1:9191'
);

assert.equal(resolveDevHost({ HOST: '127.0.0.1' }), '127.0.0.1');
assert.equal(resolveDevHost({}), '::');
for (const peer of ['::1', '127.0.0.1', '::ffff:127.0.0.1']) assert.equal(isLoopbackPeer(peer), true);
for (const peer of ['', '192.0.2.1', '::ffff:192.0.2.1', '127.invalid', '::']) assert.equal(isLoopbackPeer(peer), false);
let middleware, upgrade;
loopbackOnlyDevPlugin().configureServer({config:{server:{host:'::'}},middlewares:{use(fn){middleware=fn;}},httpServer:{prependListener(event,fn){assert.equal(event,'upgrade');upgrade=fn;}}});
let forwarded=false, ended=false, destroyed=false;
const response={statusCode:200,end(){ended=true;}};
middleware({socket:{remoteAddress:'192.0.2.1'}},response,()=>{forwarded=true;});
assert.equal(response.statusCode,403);assert.equal(ended,true);assert.equal(forwarded,false);
middleware({socket:{remoteAddress:'::ffff:127.0.0.1'}},response,()=>{forwarded=true;});assert.equal(forwarded,true);
upgrade({}, {remoteAddress:'192.0.2.1',destroy(){destroyed=true;}});assert.equal(destroyed,true);
assert.equal(resolveDevHost({ VITE_HOST: '0.0.0.0', HOST: '127.0.0.1' }), '0.0.0.0');
assert.equal(resolveDevPort({ PORT: '5175' }), 5175);
assert.equal(resolveDevPort({ VITE_PORT: '5176', PORT: '5175' }), 5176);

assert.equal(
  resolveProxyTarget({ DATA_URL: 'http://localhost:8080/' }),
  'http://localhost:8080'
);

assert.throws(
  () => resolveProxyTarget({}, { requireExplicit: true }),
  /Missing backend proxy target for Vite dev server/
);

console.log('vite.config proxy target contract ✓ explicit backend target required for dev');

const config = createConfig({mode:'production',command:'build'});
for (const name of ['forge/theme', 'forge/editor', 'forge/window-editor']) {
  const alias = config.resolve.alias.find(entry => entry.find instanceof RegExp && entry.find.test(name));
  assert.ok(alias, `missing public export ${name}`);
  assert.ok(fs.existsSync(alias.replacement), `public export ${name} must resolve to source`);
}
assert.ok(config.resolve.alias.some(entry => entry.find === 'forge'), 'legacy deep source imports must remain supported');
console.log('Forge public exports and legacy source aliases passed');
