import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
const css=readFileSync(new URL('./shell.css',import.meta.url),'utf8');
const block=selector=>{const escaped=selector.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');const match=css.match(new RegExp(`${escaped}\\s*\\{([^}]*)\\}`));assert.ok(match,`Missing layer rule: ${selector}`);return match[1];};
const fullscreen=block('.app-summary-surface-shell.is-workspace:not(.is-split)'),overlay=block('.agently-application .bp6-portal > .bp6-overlay');
const layer=body=>Number(body.match(/z-index:\s*(\d+)/)?.[1]);assert.equal(layer(fullscreen),100);assert.equal(layer(overlay),110);assert.ok(layer(overlay)>layer(fullscreen),'Body dialog portals must receive input above fullscreen workspace.');
assert.match(fullscreen,/position:\s*fixed/);assert.doesNotMatch(overlay,/pointer-events:\s*none/);
console.log('Main host dialog portals are layered above fullscreen workspace without blocking their input.');
