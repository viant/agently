import {describe, expect, it} from 'vitest';
import {configuredWindowAuthorizationRefresh} from './windowAuthorizationRefreshConfig';

describe('retired UI permission renewal option', () => {
  it('cannot enable permission lease renewal from operator configuration', () => {
    for (const mode of [undefined, '', 'disabled', 'seamless', 'SEAMLESS']) {
      expect(configuredWindowAuthorizationRefresh(mode)).toBeUndefined();
    }
  });
});
