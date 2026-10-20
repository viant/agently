import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
const css=readFileSync(new URL('./shell.css',import.meta.url),'utf8');
const block=selector=>{const escaped=selector.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');const match=css.match(new RegExp(`${escaped}\\s*\\{([^}]*)\\}`));assert.ok(match,`Missing layer rule: ${selector}`);return match[1];};
const fullscreen=block('.app-summary-surface-shell.is-workspace:not(.is-split)'),overlay=block('.agently-application .bp6-portal > .bp6-overlay');
const layer=body=>Number(body.match(/z-index:\s*(\d+)/)?.[1]);assert.equal(layer(fullscreen),100);assert.equal(layer(overlay),110);assert.ok(layer(overlay)>layer(fullscreen),'Body dialog portals must receive input above fullscreen workspace.');
assert.match(fullscreen,/position:\s*absolute/);assert.match(block('.app-chat-pane'),/position:\s*relative/);assert.doesNotMatch(overlay,/pointer-events:\s*none/);
console.log('Workspace focus stays within the chat pane and dialog portals remain above it.');
