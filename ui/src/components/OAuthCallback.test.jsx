import { describe, expect, it, vi } from 'vitest';

import { completeOAuthReturn, exchangeOAuthCallback } from './OAuthCallback.jsx';

describe('OAuthCallback helpers', () => {
  it('posts callback codes as a JSON callback exchange', async () => {
    const fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ status: 'ok' }),
    });
    const targetWindow = {
      fetch,
    };

    await expect(exchangeOAuthCallback(targetWindow, 'code-123', 'state-456')).resolves.toEqual({ status: 'ok' });
    expect(fetch).toHaveBeenCalledWith('/v1/api/auth/oauth/callback?format=json', expect.objectContaining({
      method: 'POST',
      credentials: 'include',
      headers: expect.objectContaining({
        'Content-Type': 'application/json',
        Accept: 'application/json',
      }),
      body: JSON.stringify({ code: 'code-123', state: 'state-456' }),
    }));
  });

  it('redirects the current tab for same-tab auth completion', () => {
    const replace = vi.fn();
    const targetWindow = {
      opener: null,
      location: {
        origin: 'http://127.0.0.1:9191',
        replace,
      },
    };

    completeOAuthReturn(targetWindow, '/conversation/conv-123');

    expect(replace).toHaveBeenCalledWith('/conversation/conv-123');
  });
});
