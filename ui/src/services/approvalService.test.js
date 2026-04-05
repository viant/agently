import { describe, expect, it } from 'vitest';

import { approvalService } from './approvalService';

describe('approvalService', () => {
  it('normalizes env selection against original request order', async () => {
    const result = await approvalService.normalizeSelection({
      editedFields: { names: ['PATH', 'HOME'] },
      originalArgs: { names: ['HOME', 'SHELL', 'PATH'] }
    });

    expect(result).toEqual({
      editedFields: {
        names: ['HOME', 'PATH']
      }
    });
  });

  it('falls back to original request when edited selection is empty', async () => {
    const result = await approvalService.filterEnvNames({
      editedFields: { names: [] },
      originalArgs: { names: ['HOME', 'SHELL'] }
    });

    expect(result).toEqual({
      editedFields: {
        names: ['HOME', 'SHELL']
      }
    });
  });
});
