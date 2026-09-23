import { describe, expect, it } from 'vitest';

import { formatWindowMetadataError, windowMetadataFetchKey, withWindowPermissionDeadline } from '../../../../forge/src/components/WindowContent.jsx';

describe('Forge window permission feedback', () => {
  it('shows an actionable timeout instead of a permanent loading state', () => {
    expect(formatWindowMetadataError({ status: 504 })).toBe('Permission check timed out. Please try again.');
  });

  it('does not restart metadata fetch for equal reconstructed window state', () => {
    const original = windowMetadataFetchKey({ windowId: 'advertisers-1', windowKey: 'advertiserList',
      conversationId: 'conv-1', parameters: { AgencyId: [42] } }, { platform: 'web' });
    const reconstructed = windowMetadataFetchKey({ windowId: 'advertisers-1', windowKey: 'advertiserList',
      conversationId: 'conv-1', parameters: { AgencyId: [42] } }, { platform: 'web' });
    const changed = windowMetadataFetchKey({ windowId: 'advertisers-1', windowKey: 'advertiserList',
      conversationId: 'conv-1', parameters: { AgencyId: [43] } }, { platform: 'web' });
    expect(reconstructed).toBe(original);
    expect(changed).not.toBe(original);
  });

  it('fails closed when the permission request never settles', async () => {
    await expect(withWindowPermissionDeadline(() => new Promise(() => {}), 5))
      .rejects.toMatchObject({ status: 504 });
    await expect(withWindowPermissionDeadline(() => Promise.resolve('permitted'), 50))
      .resolves.toBe('permitted');
  });
});
