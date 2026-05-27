import { describe, expect, it } from 'vitest';
import { appRoutePaths } from './appRoutePaths.js';

describe('appRoutePaths', () => {
  it('includes lookup preview and callback routes', () => {
    expect(Array.isArray(appRoutePaths)).toBe(true);
    expect(appRoutePaths).toContain('/lookup-chip-preview');
    expect(appRoutePaths).toContain('/ui/lookup-chip-preview');
    expect(appRoutePaths).toContain('/v1/api/auth/oauth/callback');
  });
});
