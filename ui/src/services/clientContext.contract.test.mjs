import assert from 'node:assert/strict';
import fs from 'node:fs';

const source = fs.readFileSync(new URL('./clientContext.js', import.meta.url), 'utf8');
assert.match(source, /import \* as ForgeComponents from 'forge\/components'/);
assert.match(source, /ForgeComponents\[responsiveResolverKey\]/);
assert.match(source, /return forgeFormFactorForWidth\(window\.innerWidth \|\| 0\)/);
console.log('Agently client context contract ✓ form-factor detection prefers Forge responsive foundations');
