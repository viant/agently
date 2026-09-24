import fs from 'node:fs';
import {describe, expect, it} from 'vitest';

const read = (path) => fs.readFileSync(new URL(path, import.meta.url), 'utf8');

describe('shared response color delivery', () => {
  it('uses application semantic roles for message chrome', () => {
    const shell = read('./styles/shell.css');
    const bubbleMessage = read('./components/chat/BubbleMessage.jsx');

    expect(shell).toMatch(/\.app-bubble-user\s*{[^}]*background: var\(--app-selected-background\);[^}]*color: var\(--app-text\);/s);
    expect(shell).toMatch(/\.app-bubble-assistant\s*{[^}]*background: var\(--app-surface\);[^}]*color: var\(--app-text\);/s);
    expect(shell).toMatch(/\.app-rich-prose \.agently-entity-chip\s*{[^}]*background: var\(--app-selected-background\);[^}]*color: var\(--app-accent-strong\);/s);
    expect(shell).not.toMatch(/\.app-bubble-(?:user|assistant)\s*{[^}]*--forge-/s);
    expect(bubbleMessage).toContain("isUser ? 'var(--app-accent)' : 'var(--app-muted)'");
  });

  it('uses the same roles for rich response content', () => {
    const richContent = read('./styles/rich-content.css');

    expect(richContent).toMatch(/\.app-rich-prose a,[\s\S]*?color: var\(--app-accent\);/);
    expect(richContent).toMatch(/\.app-rich-table th\s*{[^}]*background: var\(--app-selected-background\);[^}]*color: var\(--app-accent-strong\);/s);
    expect(richContent).toMatch(/\.app-rich-chart\s*{[^}]*border: 1px solid var\(--app-border\);[^}]*background: var\(--app-surface\);/s);
    expect(richContent).toMatch(/\.app-rich-mermaid-error\s*{[^}]*background: var\(--app-status-danger-background\);[^}]*color: var\(--app-status-danger-foreground\);/s);
  });
});
