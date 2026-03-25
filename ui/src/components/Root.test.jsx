import { describe, expect, it } from 'vitest';
import { resolveInitialAuthState, resolveOAuthProviderLabel } from './Root.jsx';

describe('Root auth bootstrap', () => {
  it('requires sign-in when oauth provider is present and no user is authenticated', () => {
    expect(resolveInitialAuthState([{ type: 'bff', name: 'oauth' }], null)).toBe('required');
  });

  it('treats an authenticated user as ready even for oauth providers', () => {
    expect(resolveInitialAuthState([{ type: 'bff', name: 'oauth' }], { username: 'tzhao' })).toBe('ready');
  });

  it('allows local-only workspaces to continue without forcing the auth gate', () => {
    expect(resolveInitialAuthState([{ type: 'local', defaultUsername: 'devuser' }], null)).toBe('ready');
  });
});

describe('Root oauth label', () => {
  it('uses the oauth provider label when available', () => {
    expect(resolveOAuthProviderLabel([{ type: 'bff', label: 'Okta' }])).toBe('Okta');
  });

  it('falls back to generic sign-in text when label is missing or generic', () => {
    expect(resolveOAuthProviderLabel([{ type: 'bff', label: 'oauth' }])).toBe('');
    expect(resolveOAuthProviderLabel([{ type: 'bff' }])).toBe('');
  });
});
