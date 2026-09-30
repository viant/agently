import fs from 'node:fs';
import {describe, expect, it} from 'vitest';

const read = (path) => fs.readFileSync(new URL(path, import.meta.url), 'utf8');

describe('shared typography delivery', () => {
  it('loads generic typography inheritance once at the application entry point', () => {
    const main = read('./main.jsx');
    const typography = read('./typography.js');
    const packageJSON = JSON.parse(read('../package.json'));
    expect(main).toContain("import './typography.js'");
    expect(main).not.toContain('@fontsource-variable');
    expect(typography).not.toContain('@fontsource-variable');
    expect(Object.keys(packageJSON.dependencies).some(name => name.startsWith('@fontsource'))).toBe(false);
    expect(packageJSON.scripts['test:typography']).toBe('node scripts/test-typography-browser.mjs');
  });

  it('lets a workspace-selected family reach shell, controls, and portals', () => {
    const typography = read('./styles/typography.css');
    const shell = read('./styles/shell.css');
    const executionWorkspace = read('./styles/execution-workspace.css');
    expect(typography).not.toMatch(/--agently-font-workspace-primary\s*:/);
    expect(typography).toMatch(/\.agently-application :where\(button, input, textarea, select, optgroup, option\)[\s\S]*?font-family: inherit;/);
    expect(shell).toContain("--app-font-family: 'Avenir Next', 'SF Pro Display', 'Segoe UI', sans-serif;");
    expect(shell).toMatch(/\[data-testid="chat-composer"\][\s\S]*?font-family: var\(--app-font-family\) !important;/);
    expect(executionWorkspace).toMatch(/\.app-execution-toolbar h2[\s\S]*?font-family: var\(--app-font-family\);/);
  });
});
