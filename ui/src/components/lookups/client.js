// client.js — thin HTTP client for the three datasource/lookup endpoints.
// Uses the same session/cookie auth as every other agently API call, so the
// workspace's OAuth middleware (when enabled) applies uniformly. No separate
// bypass for the registry endpoint.
//
// Base URL is resolved from the agently `endpoint` module when available;
// for tests a caller may pass an explicit base.

import { sdkBaseURL } from '../../endpoint.js';

function baseURL() {
  // sdkBaseURL is "/v1"; all our routes are under "/v1/api/..." so we
  // return the root without the "/v1" prefix and let each call build its
  // own path. This matches how other client helpers in agently behave.
  return sdkBaseURL.replace(/\/v1$/, '');
}

/**
 * GET /v1/api/lookups/registry?context=<kind>:<id>
 * @param {string} contextKind  e.g. "template", "chat-composer"
 * @param {string} contextID    e.g. "site_list_planner"
 * @returns {Promise<Array<LookupRegistryEntry>>}
 */
export async function listLookupRegistry(contextKind, contextID) {
  const ctxParam = encodeURIComponent(`${contextKind}:${contextID}`);
  const res = await fetch(
    `${baseURL()}/v1/api/lookups/registry?context=${ctxParam}`,
    { credentials: 'include' }
  );
  if (res.status === 501) return [];
  if (!res.ok) {
    throw new Error(`lookup registry: ${res.status} ${res.statusText}`);
  }
  const body = await res.json();
  return body && Array.isArray(body.entries) ? body.entries : [];
}

/**
 * POST /v1/api/datasources/{id}/fetch
 *
 * Forwards both `inputs` AND optional `cache` hints so that `bypassCache`
 * / `writeThrough` behave the same way as the Go / iOS / Android clients.
 * Earlier versions dropped the cache hints silently; that drift is fixed.
 *
 * @param {string} id         datasource id
 * @param {object} inputs     caller inputs (merged with pinned server-side)
 * @param {object} [options]
 * @param {boolean} [options.bypassCache]   force a fresh backend call
 * @param {boolean} [options.writeThrough]  on hit, still trigger a background refresh
 * @returns {Promise<{rows: Array<object>, dataInfo?: object, cache?: object}>}
 */
export async function fetchDatasource(id, inputs, options) {
  const body = { inputs: inputs || {} };
  if (options && (options.bypassCache || options.writeThrough)) {
    body.cache = {};
    if (options.bypassCache) body.cache.bypassCache = true;
    if (options.writeThrough) body.cache.writeThrough = true;
  }
  const res = await fetch(
    `${baseURL()}/v1/api/datasources/${encodeURIComponent(id)}/fetch`,
    {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }
  );
  if (!res.ok) {
    throw new Error(`datasource fetch: ${res.status} ${res.statusText}`);
  }
  return res.json();
}

/**
 * DELETE /v1/api/datasources/{id}/cache[?inputsHash=…]
 */
export async function invalidateDatasourceCache(id, inputsHash) {
  const q = inputsHash
    ? `?inputsHash=${encodeURIComponent(inputsHash)}`
    : '';
  const res = await fetch(
    `${baseURL()}/v1/api/datasources/${encodeURIComponent(id)}/cache${q}`,
    { method: 'DELETE', credentials: 'include' }
  );
  if (!res.ok && res.status !== 404) {
    throw new Error(`datasource invalidate: ${res.status}`);
  }
}
