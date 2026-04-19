import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

vi.mock('../../../../forge/src/components/table/basic/Toolbar.jsx', () => ({
  default: ({ toolbarItems = [] }) => React.createElement(
    'div',
    { 'data-testid': 'mock-toolbar' },
    toolbarItems.map((item) => item?.title || item?.id || '').join(', '),
  ),
}));

import { computeAbortVisibility, renderChatToolbar } from '../../../../forge/src/components/chatViewHelpers.js';

describe('chatViewHelpers', () => {
  it('computes abort visibility from selector + when', () => {
    const context = {
      signals: {
        form: {
          value: { running: true, status: 'active' },
        },
      },
      Context: () => null,
    };

    expect(computeAbortVisibility({
      chatCfg: { abortVisible: { selector: 'running' } },
      context,
    })).toBe(true);

    expect(computeAbortVisibility({
      chatCfg: { abortVisible: { selector: 'status', when: 'active' } },
      context,
    })).toBe(true);

    expect(computeAbortVisibility({
      chatCfg: { abortVisible: { selector: 'status', when: ['paused', 'done'] } },
      context,
    })).toBe(false);
  });

  it('renders toolbar descriptors and direct elements', () => {
    const element = React.createElement('div', { className: 'direct-toolbar' }, 'Direct');
    expect(renderToStaticMarkup(renderChatToolbar({
      effectiveToolbar: element,
      context: {},
    }))).toContain('Direct');

    const toolbarContext = {
      signals: {
        message: { value: null },
        control: { value: { inactive: false } },
      },
      handlers: {},
      Context: () => null,
    };
    const descriptorHtml = renderToStaticMarkup(renderChatToolbar({
      effectiveToolbar: { items: [{ id: 'refresh', title: 'Refresh' }] },
      context: toolbarContext,
    }));
    expect(descriptorHtml).toContain('Refresh');
  });
});
