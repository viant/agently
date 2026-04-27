import assert from 'node:assert/strict';
import { appRoutePaths } from './appRoutePaths.js';

assert.ok(Array.isArray(appRoutePaths), 'appRoutePaths should be an array');
assert.ok(appRoutePaths.includes('/lookup-chip-preview'));
assert.ok(appRoutePaths.includes('/ui/lookup-chip-preview'));
assert.ok(appRoutePaths.includes('/v1/api/auth/oauth/callback'));

console.log('appRoutePaths ✓ includes lookup preview and callback routes');
