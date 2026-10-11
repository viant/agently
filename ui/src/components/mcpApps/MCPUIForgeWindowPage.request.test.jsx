import React from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('forge/components', () => ({ WindowContent: () => null }));
vi.mock('forge/core', () => ({ useSetting: () => ({ targetContext: { platform: 'web', formFactor: 'desktop' } }) }));
vi.mock('../../services/mcpApps/forgeGuestBridge.js', () => ({ installForgeGuestBridge: () => () => {} }));
vi.mock('./MCPUIVerifierRouteDebug.jsx', () => ({ default: () => null }));

import MCPUIForgeWindowPage from './MCPUIForgeWindowPage.jsx';

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

async function openStandaloneWindow(response) {
  const effects = [];
  const setState = vi.fn();
  vi.spyOn(React, 'useMemo').mockImplementation((compute) => compute());
  vi.spyOn(React, 'useState').mockReturnValue([{ loading: true }, setState]);
  vi.spyOn(React, 'useEffect').mockImplementation((effect) => { effects.push(effect); });
  vi.stubGlobal('window', { location: { search: new URLSearchParams({
    windowKey: 'advertiser', windowParams: JSON.stringify({ AdvertiserId: [111298] }),
  }).toString() } });
  const fetch = vi.fn().mockResolvedValue(response);
  vi.stubGlobal('fetch', fetch);
  MCPUIForgeWindowPage();
  effects.forEach((effect) => effect());
  await vi.waitFor(() => expect(setState).toHaveBeenCalled());
  return { fetch, setState };
}

describe('standalone Forge window permission request', () => {
  it('sends the authored selection into server admission before rendering', async () => {
    const { fetch, setState } = await openStandaloneWindow({
      ok: true, json: async () => ({ data: { namespace: 'Advertiser' } }),
    });
    const [request, options] = fetch.mock.calls[0];
    const url = new URL(request, 'http://localhost');
    expect(url.pathname).toBe('/v1/api/agently/forge/window/advertiser');
    expect(url.searchParams.get('applyPermission')).toBe('true');
    expect(JSON.parse(url.searchParams.get('windowParams'))).toEqual({ AdvertiserId: [111298] });
    expect(url.searchParams.get('platform')).toBe('web');
    expect(url.searchParams.get('formFactor')).toBe('desktop');
    expect(options).toEqual({ method: 'GET', credentials: 'include', headers: { Accept: 'application/json' } });
    expect(setState).toHaveBeenCalledWith(expect.objectContaining({
      error: '', window: expect.objectContaining({ parameters: { AdvertiserId: [111298] } }),
    }));
  });

  it('keeps a server denial denied instead of constructing a selected window', async () => {
    const { setState } = await openStandaloneWindow({
      ok: false, status: 403, text: async () => 'resource not found or access denied',
    });
    expect(setState).toHaveBeenCalledWith({
      loading: false, error: 'resource not found or access denied', window: null,
    });
  });
});
