import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, it, expect } from 'vitest';
import fs from 'node:fs';
import ElicitationMessage from './ElicitationMessage';

describe('ElicitationMessage', () => {
  it('renders Markdown rather than literal syntax', () => {
    const html = renderToStaticMarkup(<ElicitationMessage message={'## Review\n**Budget**\n- One\n- Two\n`field`'} />);
    expect(html).toContain('<h2>Review</h2>');
    expect(html).toContain('<strong>Budget</strong>');
    expect(html).toContain('<li>One</li>');
    expect(html).toContain('<code>field</code>');
  });
  it('handles empty and plain text messages', () => {
    expect(renderToStaticMarkup(<ElicitationMessage message="" />)).toBe('');
    expect(renderToStaticMarkup(<ElicitationMessage message="Confirm these values." />)).toContain('Confirm these values.');
  });
  it('uses the same presentation in both web elicitation surfaces', () => {
    for (const path of ['./ElicitationOverlay.jsx', './chat/ElicitationForm.jsx']) {
      expect(fs.readFileSync(new URL(path, import.meta.url), 'utf8')).toContain('<ElicitationMessage message={prompt} />');
    }
  });
});
