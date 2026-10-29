import fs from 'node:fs';
import {describe, expect, it} from 'vitest';

const read = (path) => fs.readFileSync(new URL(path, import.meta.url), 'utf8');

describe('section navigation design-system adoption', () => {
  it('keeps shared appearance semantic and rejects raw styling bypasses', () => {
    const source = read('./components/SectionNavigation.jsx');
    const styles = read('./components/sectionNavigation.css') + read('./components/shellHeader.css') + read('./components/chat/documentMessages.css') + read('./components/requestLifecycle.css') + read('./components/chat/turnFeedback.css');
    const bypasses = css => /(?:#[0-9a-f]{3,8}\b|\b(?:rgb|hsl)a?\(|!important|(?:^|[;{])\s*(?:font-size|font-weight|line-height|letter-spacing)\s*:\s*-?\d)/i.test(css);
    expect(bypasses(styles)).toBe(false);
    expect(bypasses('.example {color: #123456}')).toBe(true);
    expect(bypasses('.example {font-size: 13px}')).toBe(true);
    expect(bypasses('.example {color: var(--app-text)}')).toBe(false);
    expect(bypasses('.example {--app-navigation-touch-font-size: 16px}')).toBe(false);
    const bubble = read('./components/chat/BubbleMessage.jsx');
    expect(bubble).toContain("typographyProps('body')");
    expect(bubble).toContain('Your message');
    expect(read('./components/lookups/NamedLookupInput.jsx')).toContain('data-forge-type={typeRole}');
    expect(source).toContain("typographyProps('body')");
    expect(source).toContain('aria-current');
    expect(source).not.toMatch(/(?:IBM Plex|Steward|Viant|fontSize|fontFamily)/i);
    for (const file of ['./components/RunProgressView.jsx','./components/RequestReviewDialog.jsx','./components/RequestLifecycleSpecimen.jsx']) {
      const component=read(file);
      expect(component).not.toMatch(/(?:IBM Plex|Steward|Viant|#[0-9a-f]{3,8}\b|fontSize|fontWeight|fontFamily|lineHeight|letterSpacing)/i);
      expect(component).toContain('typeRole=');
    }
    expect(read('./components/Sidebar.jsx')).toContain("typographyProps('metadata')");
    const header = read('./components/ShellHeader.jsx');
    expect(header).toContain('typeRole="label"');
    expect(header).toContain("typographyProps('label')");
    for (const icon of ['menu', 'arrow-left', 'arrow-right']) expect(header).toContain(`lucide:${icon}`);
    expect(header).not.toMatch(/(?:IBM Plex|Steward|Viant|fontSize|fontFamily)/i);
    const surface = read('./components/ConversationWorkspaceSurface.jsx');
    expect(surface).toContain('aria-controls="conversation-workspace-viewer"');
    expect(surface).toContain('lucide:panel-right');
    expect(surface).toContain('lucide:maximize');
    expect(surface).toContain('WorkspaceRuntimeTabs');
    for (const file of ['./components/WorkspaceOutputState.jsx', './components/InlineReportWorkspace.jsx', './components/OutputHandoffSpecimen.jsx', './components/chat/WorkspaceAttachmentCard.jsx']) {
      const source = read(file);
      expect(source).not.toMatch(/(?:IBM Plex|Steward|Viant|#[0-9a-f]{3,8}\b|fontSize|fontWeight|fontFamily|lineHeight|letterSpacing)/i);
      expect(source).toMatch(/(?:typeRole=|typographyProps\(|WorkspaceOutputState)/);
    }

  });
});

describe('shared typography delivery', () => {
  it('loads generic typography inheritance once at the application entry point', () => {
    const main = read('./main.jsx');
    const typography = read('./typography.js');
    const packageJSON = JSON.parse(read('../package.json'));
    expect(main).toContain("import './typography.js'");
    expect(main).not.toContain('@fontsource-variable');
    expect(typography).not.toContain('@fontsource-variable');
    expect(Object.keys(packageJSON.dependencies).some(name => name.startsWith('@fontsource'))).toBe(false);
    expect(packageJSON.scripts['test:typography']).toBe('vitest run src/components/chat/HostedInlineReport.render.test.jsx src/services/feedForgeWiring.test.js --testTimeout=15000 && node scripts/test-typography-browser.mjs');
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

describe('role adoption in the shared settings pilot', () => {
  it('takes workspace-specific proof content and expectations from external fixtures', () => {
    for (const file of ['./typographyComponentsProof.jsx', '../scripts/test-typography-browser.mjs']) {
      expect(read(file)).not.toMatch(/(?:IBM Plex|Steward|Viant|media-plan|planDisplayName)/i);
    }
  });
  it('renders the actual authored pilot container and forwards document appearance', () => {
    const proof = read('./typographyComponentsProof.jsx');
    expect(proof).toContain('<Container container={fixture.pilot.container} context={context}/>');
    const host = read('./components/ToolFeedDetail.jsx');
    expect(host).toContain('appearance: ui.appearance');
    expect(host).toContain('data-forge-appearance={container.appearance}');
  });
  it('uses semantic roles and does not introduce inline typography', () => {
    const source = read('./components/WorkspaceAppearanceSettings.jsx');
    for (const role of ['section', 'body', 'label', 'metadata']) expect(source).toContain(`text('${role}')`);
    expect(source).not.toMatch(/(?:fontSize|fontWeight|lineHeight|letterSpacing|fontFamily)\s*:/);
    expect(source).toContain('typography.${role}.weight');
    expect(source).toContain('SelectControl');
    expect(source).not.toContain('<select');
  });
});
