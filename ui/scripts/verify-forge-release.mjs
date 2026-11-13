import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

const uiRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const forgeRoot = path.resolve(process.env.FORGE_SOURCE_ROOT || path.join(uiRoot, 'node_modules/forge'));
const forgePackagePath = path.join(forgeRoot, 'package.json');
assert.ok(fs.existsSync(forgePackagePath), `Forge package not found at ${forgeRoot}`);

const forgePackage = JSON.parse(fs.readFileSync(forgePackagePath, 'utf8'));
assert.equal(forgePackage.name, 'forge');
assert.ok(/^1\.1\./.test(forgePackage.version), `Forge 1.1.x is required, found ${forgePackage.version}`);
for (const exportPath of ['./patterns', './responsive']) {
  assert.ok(forgePackage.exports?.[exportPath], `Forge must export ${exportPath}`);
}

const releaseFiles = Array.isArray(forgePackage.files) ? forgePackage.files : [];
for (const requiredPattern of ['src', 'index.js']) {
  assert.ok(releaseFiles.includes(requiredPattern), `Forge release must include ${requiredPattern}`);
}
for (const excludedPattern of ['!src/**/*.test.*', '!src/**/testdata/**', '!src/**/*.browser.*']) {
  assert.ok(releaseFiles.includes(excludedPattern), `Forge release must exclude ${excludedPattern}`);
}

for (const relativePath of [
  'src/components/patterns/index.js',
  'src/components/patterns/SectionPattern.jsx',
  'src/components/Container.css',
  'src/components/responsive.js',
]) {
  assert.ok(fs.existsSync(path.join(forgeRoot, relativePath)), `Forge release is missing ${relativePath}`);
}

console.log(`Agently Forge release contract passed: forge@${forgePackage.version} at ${forgeRoot}`);
