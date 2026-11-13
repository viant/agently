import fs from 'node:fs';
import {describe, expect, it} from 'vitest';

const read = (path) => fs.readFileSync(new URL(path, import.meta.url), 'utf8');

describe('shared response color delivery', () => {
  it('uses application semantic roles for message chrome', () => {
    const shell = read('./styles/shell.css');
    const bubbleMessage = read('./components/chat/BubbleMessage.jsx');

    expect(shell).toMatch(/\.app-bubble-user\s*{[^}]*background: var\(--app-user-message-background, var\(--forge-table-header-surface, #eef4ff\)\);[^}]*border-color: var\(--app-user-message-border, var\(--forge-control-border, #c8d8fb\)\);[^}]*color: var\(--app-user-message-text, var\(--forge-text, var\(--app-text\)\)\);/s);
    expect(shell).toMatch(/\.app-bubble-assistant\s*{[^}]*background: var\(--app-surface\);[^}]*color: var\(--app-text\);/s);
    expect(shell).toMatch(/\.app-rich-prose \.agently-entity-chip\s*{[^}]*background: var\(--app-selected-background\);[^}]*color: var\(--app-accent-strong\);/s);
    expect(shell).not.toMatch(/\.app-bubble-user\s*{[^}]*background: var\(--app-selected-background\)/s);
    expect(bubbleMessage).toContain("isUser ? 'var(--app-accent)' : 'var(--app-muted)'");
  });

  it('uses the same roles for rich response content', () => {
    const richContent = read('./styles/rich-content.css');

    expect(richContent).toMatch(/\.app-rich-prose a,[\s\S]*?color: var\(--app-accent\);/);
    expect(richContent).toMatch(/\.app-rich-table th\s*{[^}]*background: var\(--app-selected-background\);[^}]*color: var\(--app-accent-strong\);/s);
    expect(richContent).toMatch(/\.app-rich-chart\s*{[^}]*border: 1px solid var\(--app-border\);[^}]*background: var\(--app-surface\);/s);
    expect(richContent).toMatch(/\.app-rich-mermaid-error\s*{[^}]*background: var\(--app-status-danger-background\);[^}]*color: var\(--app-status-danger-foreground\);/s);
  });

  it('maps execution states and primary actions to Forge semantic roles', () => {
    const execution = read('./styles/execution-workspace.css');
    expect(execution).toMatch(/tone-live[\s\S]*?background: var\(--forge-status-success-foreground/);
    expect(execution).toMatch(/tone-connecting[\s\S]*?background: var\(--forge-status-warning-foreground/);
    expect(execution).toMatch(/tone-disconnected[\s\S]*?background: var\(--forge-status-danger-foreground/);
    expect(execution).toMatch(/\.app-execution-composer-actions button\s*{[^}]*color: var\(--forge-button-text/);
    expect(execution).toMatch(/\.app-execution-composer-actions button\s*{[^}]*background: var\(--forge-button-bg/);
    expect(execution).toMatch(/\.app-execution-error\s*{[^}]*background: var\(--forge-status-danger-background/);
  });

  it('maps conversation usage roles, data series, and errors to Forge semantics', () => {
    const usage = read('./styles/conversation-usage.css');
    expect(usage).toMatch(/\.conversation-usage-model-heading \.conversation-usage-role\.is-intake[^}]*background: var\(--forge-status-warning-background[^}]*color: var\(--forge-status-warning-foreground/s);
    expect(usage).toMatch(/\.conversation-usage-model-heading \.conversation-usage-role\.is-sidecar[^}]*background: var\(--forge-status-success-background[^}]*color: var\(--forge-status-success-foreground/s);
    expect(usage).toMatch(/\.conversation-usage-model-bar span\.is-input[^}]*background: var\(--forge-data-categorical-1/);
    expect(usage).toMatch(/\.conversation-usage-model-bar span\.is-output[^}]*background: var\(--forge-data-categorical-2/);
    expect(usage).toMatch(/\.conversation-usage-error[^}]*border: 1px solid var\(--forge-status-danger-border[^}]*background: var\(--forge-status-danger-background[^}]*color: var\(--forge-status-danger-foreground/s);
  });

  it('keeps Design System 2.0 status rules semantic while Baseline retains its original colors', () => {
    const files = [
      './styles/shell.css',
      './styles/iteration.css',
      './styles/progress.css',
      './styles/detail-panel.css',
      './styles/steer.css',
      './components/executionDetails.css',
    ];
    const directLegacyRole = /(?:color|background|border(?:-[a-z-]+)?|--app-[a-z-]+)\s*:\s*var\(--agently-theme-status-/;
    for (const file of files) {
      const scoped = Array.from(read(file).matchAll(/\.agently-application\[data-agently-theme="design-system-2"\][^{]*\{[^}]*\}/g), match => match[0]).join('\n');
      expect(scoped).toContain('design-system-2');
      expect(scoped).not.toMatch(directLegacyRole);
    }
    expect(read('./styles/shell.css')).toMatch(/\.app-sidebar-error\s*\{\s*color: var\(--agently-theme-status-danger-foreground/);
  });
});
