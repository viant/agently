import assert from 'node:assert/strict';
import fs from 'node:fs';

const workspace = fs.readFileSync(new URL('./ConversationWorkspaceSurface.jsx', import.meta.url), 'utf8');
const dock = fs.readFileSync(new URL('./WorkspaceChatDock.jsx', import.meta.url), 'utf8');
const styles = fs.readFileSync(new URL('./workspaceChatDock.css', import.meta.url), 'utf8');
const shell = fs.readFileSync(new URL('../styles/shell.css', import.meta.url), 'utf8');

assert.match(workspace, /data-forge-component="workspace-surface"/);
assert.match(dock, /data-forge-component="chat-dock"/);
assert.match(styles, /var\(--forge-border-color/);
assert.match(styles, /var\(--forge-surface/);
assert.match(styles, /var\(--forge-spacing-2/);
assert.match(styles, /var\(--forge-radius-surface/);
assert.match(styles, /var\(--forge-overlay-shadow/);
const designSystemRules = Array.from(styles.matchAll(/\.agently-application\[data-agently-theme="design-system-2"\][^{]*\{[^}]*\}/g), match => match[0]).join('\n');
assert.match(designSystemRules, /--forge-border-color/);
assert.match(designSystemRules, /--forge-surface/);
assert.doesNotMatch(designSystemRules, /#d8dee8|#fff\b/);
for (const role of [
  '--forge-canvas',
  '--forge-surface',
  '--forge-border-color',
  '--forge-text',
  '--forge-interaction-color',
  '--forge-selected-background',
  '--forge-status-danger-background',
  '--forge-font-family',
  '--forge-type-body-size',
  '--forge-radius-surface',
]) assert.match(shell, new RegExp(`--app-[^:]+: var\\(${role.replace(/[.*+?^${}()|[\\]\\\\]/g, '\\\\$&')}`));

console.log('Agently surface contract ✓ shell surfaces consume the shared Forge semantic geometry');
