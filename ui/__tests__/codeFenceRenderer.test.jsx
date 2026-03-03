/* @vitest-environment jsdom */
import React from 'react';
import { render } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import CodeFenceRenderer from '../src/components/CodeFenceRenderer.jsx';

if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

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

  it('does not render chart from legacy CHART_SPEC payload (chartType/x/series)', () => {
    const md = [
      '<!-- CHART_SPEC:v1 -->\n',
      '```json\n',
      '{\n',
      '  "chartType": "bar",\n',
      '  "title": "Legacy Chart",\n',
      '  "x": ["A", "B"],\n',
      '  "series": [\n',
      '    { "name": "hop_count", "data": [4, 3] }\n',
      '  ]\n',
      '}\n',
      '```',
    ].join('');
    const { container } = render(<CodeFenceRenderer text={md} />);
    expect(container.textContent).toContain('Legacy Chart');
    expect(container.textContent).toContain('"chartType": "bar"');
    expect(container.textContent).not.toContain('Table');
  });

  it('renders chart for canonical CHART_SPEC schema (chart + data)', () => {
    const md = [
      '```json\n',
      '{\n',
      '  "title": "Canonical Chart",\n',
      '  "chart": { "type": "bar", "x": { "key": "metric" }, "y": [{ "key": "value" }] },\n',
      '  "data": [{ "metric": "A", "value": 4 }]\n',
      '}\n',
      '```',
    ].join('');
    const { container } = render(<CodeFenceRenderer text={md} />);
    expect(container.textContent).toContain('Canonical Chart');
    expect(container.textContent).toContain('Table');
  });

  it('renders chart from top-level type+data shape', () => {
    const md = [
      '```json\n',
      '{\n',
      '  "title": "Spend vs Required",\n',
      '  "type": "bar",\n',
      '  "data": [\n',
      '    { "metric": "Avg daily spend", "value": 267.993 },\n',
      '    { "metric": "Required daily spend", "value": 201.4691 }\n',
      '  ]\n',
      '}\n',
      '```',
    ].join('');
    const { container } = render(<CodeFenceRenderer text={md} />);
    expect(container.textContent).toContain('Spend vs Required');
    expect(container.textContent).toContain('Table');
  });

  it('renders scatter chart from top-level type+x/y+data shape', () => {
    const md = [
      '```json\n',
      '{\n',
      '  "title": "Win rate vs shading ratio",\n',
      '  "type": "scatter",\n',
      '  "x": { "field": "shading_ratio_7d" },\n',
      '  "y": { "field": "win_rate_7d" },\n',
      '  "data": [\n',
      '    { "audience_id": 1, "win_rate_7d": 1.0, "shading_ratio_7d": 0.0048 },\n',
      '    { "audience_id": 2, "win_rate_7d": 0.9993, "shading_ratio_7d": 0.0035 }\n',
      '  ]\n',
      '}\n',
      '```',
    ].join('');
    const { container } = render(<CodeFenceRenderer text={md} />);
    expect(container.textContent).toContain('Win rate vs shading ratio');
    expect(container.textContent).toContain('Table');
  });
});
