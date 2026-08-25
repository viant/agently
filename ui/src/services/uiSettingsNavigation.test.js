import { describe, expect, it } from 'vitest';

import { safeUIReturnPath, uiSettingsHref, uiSettingsReturnHref } from './uiSettingsNavigation';

describe('UI Settings navigation', () => {
  it('preserves the active conversation route through settings', () => {
    const href = uiSettingsHref({
      pathname: '/conversation/conv-1',
      search: '?view=workspace',
      hash: '#latest',
    });
    expect(href).toBe('/ui/settings?returnTo=%2Fconversation%2Fconv-1%3Fview%3Dworkspace%23latest');
    expect(uiSettingsReturnHref({ search: href.slice(href.indexOf('?')) })).toBe('/conversation/conv-1?view=workspace#latest');
  });

  it('rejects external, protocol-relative, and recursive return routes', () => {
    expect(safeUIReturnPath('https://example.com')).toBe('/');
    expect(safeUIReturnPath('//example.com')).toBe('/');
    expect(safeUIReturnPath('/ui/settings?returnTo=x')).toBe('/');
  });
});
