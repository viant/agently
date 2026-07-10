import assert from 'node:assert/strict';

import { resolveDevHost, resolveDevPort, resolveProxyTarget } from './vite.config.js';

assert.equal(
  resolveProxyTarget({ APPSERVER_URL: 'http://127.0.0.1:9191' }, { requireExplicit: true }),
  'http://127.0.0.1:9191'
);

assert.equal(resolveDevHost({ HOST: '127.0.0.1' }), '127.0.0.1');
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
