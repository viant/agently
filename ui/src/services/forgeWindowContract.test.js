import { describe, expect, it } from 'vitest';

import {
  getIncompleteWindowContractError,
  INCOMPLETE_FORGE_WINDOW_CONTRACT_MESSAGE,
} from './forgeWindowContract.js';

describe('forgeWindowContract', () => {
  it('flags a payload that declares report builder hooks without action code', () => {
    expect(getIncompleteWindowContractError({
      data: {
        reportBuilder: {
          hooks: {
            initializeState: 'Performance Metrics.stewardReportBuilder.initializeState',
            buildRequest: 'Performance Metrics.stewardReportBuilder.buildRequest',
          },
        },
      },
    })).toBe(INCOMPLETE_FORGE_WINDOW_CONTRACT_MESSAGE);
  });

  it('accepts report builder payloads that include action code', () => {
    expect(getIncompleteWindowContractError({
      reportBuilder: {
        hooks: {
          resolveLookup: 'Performance Metrics.stewardReportBuilder.resolveLookup',
        },
      },
      actions: {
        code: '(() => ({ stewardReportBuilder: { resolveLookup() { return null; } } }))()',
      },
    })).toBe('');
  });
});
