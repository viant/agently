import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { dispatchCallback } from './callbackService';

describe('callbackService.dispatchCallback', () => {
  let originalFetch;

  beforeEach(() => {
    originalFetch = globalThis.fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('POSTs to /v1/api/callbacks/dispatch with credentials and JSON body', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ tool: 'steward-SaveRecommendation', result: '{"ok":true}' }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    }));
    globalThis.fetch = fetchMock;

    const out = await dispatchCallback({
      eventName: 'spo_planner_submit',
      conversationId: 'conv-xyz',
      payload: { selectedRows: [{ site_id: 101, action: 'CUT' }] },
      context: { agencyId: 5337 },
    });

    expect(out.ok).toBe(true);
    expect(out.notFound).toBe(false);
    expect(out.tool).toBe('steward-SaveRecommendation');
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const [url, opts] = fetchMock.mock.calls[0];
    expect(url).toBe('/v1/api/callbacks/dispatch');
    expect(opts.method).toBe('POST');
    expect(opts.credentials).toBe('include');
    const body = JSON.parse(opts.body);
    expect(body.eventName).toBe('spo_planner_submit');
    expect(body.conversationId).toBe('conv-xyz');
    expect(body.payload.selectedRows).toEqual([{ site_id: 101, action: 'CUT' }]);
    expect(body.context).toEqual({ agencyId: 5337 });
    // Keys with undefined should have been stripped.
    expect(Object.prototype.hasOwnProperty.call(body, 'turnId')).toBe(false);
  });

  it('reports notFound when the server returns 404', async () => {
    globalThis.fetch = vi.fn(async () => new Response('no callback registered', { status: 404 }));
    const out = await dispatchCallback({ eventName: 'nope' });
    expect(out.ok).toBe(false);
    expect(out.notFound).toBe(true);
    expect(out.status).toBe(404);
  });

  it('reports notFound when the server returns 400 with "no callback registered"', async () => {
    globalThis.fetch = vi.fn(async () => new Response('no callback registered for event "x"', { status: 400 }));
    const out = await dispatchCallback({ eventName: 'x' });
    expect(out.ok).toBe(false);
    expect(out.notFound).toBe(true);
    expect(out.error).toContain('no callback registered');
  });

  it('reports a plain error when the server returns 400 with a different message', async () => {
    globalThis.fetch = vi.fn(async () => new Response('rendered payload is not valid JSON', { status: 400 }));
    const out = await dispatchCallback({ eventName: 'broken_template' });
    expect(out.ok).toBe(false);
    expect(out.notFound).toBe(false);
    expect(out.error).toContain('rendered payload is not valid JSON');
  });

  it('rejects an empty eventName before making a network call', async () => {
    const fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    const out = await dispatchCallback({});
    expect(out.ok).toBe(false);
    expect(out.error).toMatch(/eventName is required/i);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('handles network failures cleanly', async () => {
    globalThis.fetch = vi.fn(async () => { throw new Error('network down'); });
    const out = await dispatchCallback({ eventName: 'anything' });
    expect(out.ok).toBe(false);
    expect(out.notFound).toBe(false);
    expect(out.error).toContain('network down');
  });
});
