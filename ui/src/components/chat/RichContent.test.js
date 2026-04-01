import { describe, expect, it } from 'vitest';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

import RichContent, { parseFences } from './RichContent';
import { renderMarkdownBlock } from 'agently-core-ui-sdk';

describe('RichContent fence parsing', () => {
  it('parses a closed fenced code block', () => {
    const parts = parseFences('Before\n```go\nfmt.Println("hi")\n```\nAfter');

    expect(parts).toHaveLength(3);
    expect(parts[1]).toMatchObject({
      kind: 'fence',
      lang: 'go',
      body: 'fmt.Println("hi")\n'
    });
  });

  it('parses an unterminated trailing fenced block for streaming content', () => {
    const parts = parseFences('```go\nfmt.Println("streaming")\nfor i := 0; i < 3; i++ {\n');

    expect(parts).toHaveLength(1);
    expect(parts[0]).toMatchObject({
      kind: 'fence',
      lang: 'go'
    });
    expect(parts[0].body).toContain('fmt.Println("streaming")');
    expect(parts[0].body).toContain('for i := 0; i < 3; i++ {');
  });

  it('parses compact json fence form used by some streamed outputs', () => {
    const parts = parseFences('<!-- CHART_SPEC:v1 -->\n```json{"version":"1.0"}\n```');

    expect(parts).toHaveLength(2);
    expect(parts[1]).toMatchObject({
      kind: 'fence',
      lang: 'json',
      body: '{"version":"1.0"}\n'
    });
  });

  it('renders markdown headings as heading tags', () => {
    const html = renderMarkdownBlock('## Cat Story\n\nA short paragraph.');
    expect(html).toContain('<h2>Cat Story</h2>');
    expect(html).toContain('A short paragraph.');
  });

  it('rewrites sandbox markdown links to generated file download URLs', () => {
    const html = renderToStaticMarkup(
      React.createElement(RichContent, {
        content: 'Created [mouse_story.pdf](sandbox:/mnt/data/mouse_story.pdf).',
        generatedFiles: [{ id: 'gf-123', filename: 'mouse_story.pdf', status: 'ready' }]
      })
    );

    expect(html).toContain('/v1/api/generated-files/gf-123/download');
    expect(html).not.toContain('sandbox:/mnt/data/mouse_story.pdf');
  });
});
