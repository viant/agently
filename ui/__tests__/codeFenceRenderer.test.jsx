import React from 'react';
import { render, screen } from '@testing-library/react';
import CodeFenceRenderer from '../src/components/CodeFenceRenderer.jsx';

describe('CodeFenceRenderer – fenced code detection', () => {
  const fencedGo = [
    'Below is one idiomatic way to “filter in-place” without allocating a second backing array.\n',
    'We compact the slice by copying the kept elements toward the front and finally reslice',
    'to the portion that remains.\n\n',
    '```go\n',
    '// keep a short alias so we don’t type the long name repeatedly\n',
    'sids := profile.segmentId\n',
    '\n',
    '// writeIdx is the position where the next *kept* element will be stored\n',
    'writeIdx := 0\n',
    '\n',
    'for _, sid := range sids {\n',
    '    // build the bloom-filter key\n',
    '    bfKey := profile.uid + strconv.Itoa(sid)\n',
    '}\n',
    '```',
  ].join('\n');

  it('renders a single <pre><code> block for ```go fenced input (fallback path if editor unavailable)', () => {
    const { container } = render(<CodeFenceRenderer text={fencedGo} />);
    // Expect at least one pre>code block with our code inside
    const pres = container.querySelectorAll('pre');
    expect(pres.length).toBeGreaterThan(0);
    const hasSnippet = Array.from(container.querySelectorAll('code')).some(el =>
      el.textContent.includes('sids := profile.segmentId')
    );
    expect(hasSnippet).toBe(true);
  });

  it('renders code when a language label starts the block without triple backticks', () => {
    const noFence = [
      'go\n',
      'sids := profile.segmentId\n',
      'writeIdx := 0\n',
      'validSegments := sids[:writeIdx]',
    ].join('');
    const { container } = render(<CodeFenceRenderer text={noFence} />);
    const code = container.querySelector('pre code');
    expect(code).toBeTruthy();
    expect(code.textContent).toContain('validSegments := sids[:writeIdx]');
  });

  it('renders code when triple backticks are present but we split manually', () => {
    const manual = 'Prose before\n```\nline 1\nline 2\n```\nProse after';
    const { container } = render(<CodeFenceRenderer text={manual} />);
    const code = container.querySelector('pre code');
    expect(code).toBeTruthy();
    expect(code.textContent).toContain('line 1');
  });
});

