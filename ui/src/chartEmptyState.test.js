import { describe, expect, it } from 'vitest';

import { hasNonEmptySummary, resolveEmptyChartMessage } from 'forge/components/Chart.jsx';

describe('chart empty-state helpers', () => {
  it('detects when a metrics payload has a populated summary object', () => {
    expect(hasNonEmptySummary({
      lineLifetimeSummary: {
        spend: 6687.689,
      },
      linePerformanceTimeline: [],
    })).toBe(true);

    expect(hasNonEmptySummary({
      lineLifetimeSummary: null,
      linePeriodSummary: null,
      linePerformanceTimeline: [],
    })).toBe(false);
  });

  it('uses the richer empty-state message when summary totals exist', () => {
    expect(resolveEmptyChartMessage({
      lineLifetimeSummary: {
        spend: 6687.689,
      },
    })).toContain('Summary totals may still be available below.');

    expect(resolveEmptyChartMessage({
      lineLifetimeSummary: null,
      linePeriodSummary: null,
    })).toBe('No data for the selected period.');
  });
});
