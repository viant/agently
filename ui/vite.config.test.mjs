import assert from 'node:assert/strict';

import { resolveProxyTarget } from './vite.config.js';

assert.equal(
  resolveProxyTarget({ APPSERVER_URL: 'http://127.0.0.1:9191' }, { requireExplicit: true }),
  'http://127.0.0.1:9191'
);

assert.equal(
  resolveProxyTarget({ DATA_URL: 'http://localhost:8080/' }),
  'http://localhost:8080'
);

assert.throws(
  () => resolveProxyTarget({}, { requireExplicit: true }),
  /Missing backend proxy target for Vite dev server/
);

console.log('vite.config proxy target contract ✓ explicit backend target required for dev');
