import { describe, expect, it } from 'vitest';

import {
  resolveHostedAuthState,
  resolveHostedOAuthProviderLabel,
} from './MCPUIForgeWindowPage.jsx';

describe('MCPUIForgeWindowPage auth helpers', () => {
  it('requires auth when an oauth-capable provider exists and no user is present', () => {
    expect(resolveHostedAuthState([
      { type: 'bff', name: 'oauth' },
    ], null)).toBe('required');
  });

  it('treats local-only auth as ready even without a current user payload', () => {
    expect(resolveHostedAuthState([
      { type: 'local', name: 'dev' },
    ], null)).toBe('ready');
  });

  it('returns ready when a user session is already present', () => {
    expect(resolveHostedAuthState([
      { type: 'bff', name: 'oauth' },
    ], { username: 'awitas' })).toBe('ready');
  });

  it('prefers explicit oauth provider labels and skips generic names', () => {
    expect(resolveHostedOAuthProviderLabel([
      { type: 'bff', name: 'oauth', label: 'Viant Login' },
    ])).toBe('Viant Login');

    expect(resolveHostedOAuthProviderLabel([
      { type: 'bff', name: 'oauth', label: 'oauth' },
    ])).toBe('');
  });
});
