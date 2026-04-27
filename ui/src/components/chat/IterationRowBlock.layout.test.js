import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

const iterationBlockSpy = vi.fn(({ showToolFeedDetail = false }) => (
  React.createElement('div', {
    'data-show-tool-feed-detail': String(!!showToolFeedDetail),
  })
));

vi.mock('./IterationBlock.jsx', () => ({
  default: (props) => iterationBlockSpy(props),
}));

import IterationRowBlock from './IterationRowBlock.jsx';

describe('IterationRowBlock layout bridge', () => {
  it('keeps tool feed detail enabled on canonical iteration rows', () => {
    const iterationRow = { kind: 'iteration', renderKey: 'rk_iter_1' };
    const html = renderToStaticMarkup(React.createElement(IterationRowBlock, {
      iterationRow,
      context: {},
    }));

    expect(iterationBlockSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        canonicalRow: iterationRow,
        showToolFeedDetail: true,
      })
    );
    expect(html).toContain('data-show-tool-feed-detail="true"');
  });
});
