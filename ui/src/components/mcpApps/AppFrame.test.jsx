import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import AppFrame, { MCP_UI_SANDBOX } from './AppFrame.jsx';

describe('AppFrame', () => {
  it('renders sandboxed iframe with srcDoc', () => {
    const html = renderToStaticMarkup(<AppFrame title="Preview" srcDoc="<main>ok</main>" />);
    expect(html).toContain('iframe');
    expect(html).toContain(`sandbox="${MCP_UI_SANDBOX}"`);
    expect(html).toContain('title="Preview"');
    expect(html).toContain('srcDoc="&lt;main&gt;ok&lt;/main&gt;"');
  });

  it('prefers explicit src when provided', () => {
    const html = renderToStaticMarkup(<AppFrame title="Preview" src="/mcp-ui-forge-window.html?windowKey=order" sandbox="allow-scripts allow-same-origin" srcDoc="<main>ignored</main>" />);
    expect(html).toContain('src="/mcp-ui-forge-window.html?windowKey=order"');
    expect(html).toContain('sandbox="allow-scripts allow-same-origin"');
    expect(html).not.toContain('srcDoc=');
  });
});
