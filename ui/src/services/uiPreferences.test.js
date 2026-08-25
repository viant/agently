import { beforeEach, describe, expect, it, vi } from 'vitest';

const values = new Map();
const localStorage = {
  getItem: (key) => values.get(key) ?? null,
  setItem: (key, value) => values.set(key, String(value)),
  removeItem: (key) => values.delete(key),
  clear: () => values.clear(),
};

describe('uiPreferences', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.stubGlobal('window', { localStorage, addEventListener: vi.fn() });
    vi.resetModules();
  });

  it('defaults Developer mode off and persists changes', async () => {
    const preferences = await import('./uiPreferences');
    expect(preferences.getDeveloperMode()).toBe(false);
    preferences.setDeveloperMode(true);
    expect(preferences.getDeveloperMode()).toBe(true);
    expect(localStorage.getItem(preferences.DEVELOPER_MODE_KEY)).toBe('true');
    preferences.resetUIPreferences();
    expect(preferences.getDeveloperMode()).toBe(false);
  });
});
