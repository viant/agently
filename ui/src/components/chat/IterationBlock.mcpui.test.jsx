import { describe, expect, it, vi } from 'vitest';
import React from 'react';

vi.mock('../mcpApps/AppRenderer.jsx', () => ({
  default: ({ uri = '', title = '' }) => React.createElement('div', {
    'data-testid': 'mock-mcpui-renderer',
    'data-uri': uri,
    'data-title': title,
  }, `mcpui:${uri}`),
}));

import { buildIterationDataFromCanonicalRow } from './IterationBlock';

describe('IterationBlock MCP UI rendering', () => {
  it('preserves explicit uiResourceUri on mapped tool steps', () => {
    const canonicalRow = {
      id: 'row-1',
      kind: 'iteration',
      lifecycle: 'completed',
      rounds: [{
          pageId: 'page-1',
          status: 'completed',
          toolCalls: [{
            toolCallId: 'call-1',
            kind: 'tool',
            toolName: 'mcpuiverify:show_widget',
            status: 'completed',
            uiResourceUri: 'ui://mcpuiverify/demo/verify_widget',
            content: '{"server":"mcpuiverify","status":"ok","uri":"ui://mcpuiverify/demo/verify_widget"}',
          }],
        }],
    };

    const data = buildIterationDataFromCanonicalRow(canonicalRow);
    expect(Array.isArray(data.executionGroups)).toBe(true);
    expect(data.executionGroups[0].toolSteps[0].uiResourceUri).toBe('ui://mcpuiverify/demo/verify_widget');
    expect(data.executionGroups[0].toolSteps[0].toolName).toBe('mcpuiverify:show_widget');
  });
});
