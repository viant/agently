import fs from 'node:fs';
import {describe, expect, it} from 'vitest';

const read = (path) => fs.readFileSync(new URL(path, import.meta.url), 'utf8');

describe('shared typography delivery', () => {
  it('loads the host registry once at the application entry point', () => {
    const main = read('./main.jsx');
    const typography = read('./typography.js');
    const packageJSON = JSON.parse(read('../package.json'));
    expect(main).toContain("import './typography.js'");
    expect(main).not.toContain('@fontsource-variable');
    expect(typography).toContain("@fontsource-variable/ibm-plex-sans/wght.css");
    expect(typography).toContain("@fontsource-variable/ibm-plex-sans/wght-italic.css");
    expect(packageJSON.scripts['test:typography']).toBe('node scripts/test-typography-browser.mjs');
    expect(read('../public/licenses/IBM-Plex-Sans-OFL-1.1.txt')).toContain('SIL OPEN FONT LICENSE Version 1.1');
  });

  it('maps the semantic primary role to IBM Plex Sans and reaches shell, controls, and portals', () => {
    const typography = read('./styles/typography.css');
    const shell = read('./styles/shell.css');
    const executionWorkspace = read('./styles/execution-workspace.css');
    expect(typography).toContain('--agently-font-product-primary: "IBM Plex Sans Variable", "IBM Plex Sans", system-ui, sans-serif;');
    expect(typography).toMatch(/\.agently-application :where\(button, input, textarea, select, optgroup, option\)[\s\S]*?font-family: inherit;/);
    expect(shell).toContain("--app-font-family: 'Avenir Next', 'SF Pro Display', 'Segoe UI', sans-serif;");
    expect(shell).toMatch(/\[data-testid="chat-composer"\][\s\S]*?font-family: var\(--app-font-family\) !important;/);
    expect(executionWorkspace).toMatch(/\.app-execution-toolbar h2[\s\S]*?font-family: var\(--app-font-family\);/);
  });
});
