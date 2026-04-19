/**
 * Callback dispatch — POSTs forge submit events to agently-core's
 * workspace-declared callback router (`POST /v1/api/callbacks/dispatch`).
 *
 * When the workspace has a matching `callbacks/<eventName>.yaml`, the
 * server renders the payload template and invokes the mapped tool (e.g.
 * `steward-SaveRecommendation`). When no callback is registered for the
 * event, the server returns HTTP 400 / 404 and this helper reports
 * `notFound: true` so callers can fall back to another path
 * (conversational chat submit, toast-only, …).
 */

import { sdkBaseURL } from '../endpoint';

const DISPATCH_PATH = '/api/callbacks/dispatch';

/**
 * @typedef {Object} DispatchInput
 * @property {string} eventName       required — matches `<workspace>/callbacks/<id>.yaml`
 * @property {string} [conversationId] surfaced to the payload template as `.conversationId`
 * @property {string} [turnId]         surfaced to the payload template as `.turnId`
 * @property {Object} [payload]        forge submit body (selectedRows, changedRows, tableId, …)
 * @property {Object} [context]        free-form keys flattened into the template root
 *                                     (e.g. agencyId, advertiserId, campaignId, adOrderId)
 *
 * @typedef {Object} DispatchResult
 * @property {boolean} ok       true when dispatch + tool invocation succeeded
 * @property {boolean} notFound true when no callback was registered for eventName
 * @property {string}  [tool]   tool that was invoked (echoed from server)
 * @property {string}  [result] tool's textual return value (verbatim)
 * @property {string}  [error]  dispatch/tool error when ok===false && notFound===false
 * @property {number}  [status] HTTP status code on failure
 */

/**
 * Dispatches a forge submit event to the configured callback router.
 *
 * @param {DispatchInput} input
 * @returns {Promise<DispatchResult>}
 */
export async function dispatchCallback(input = {}) {
  const eventName = String(input?.eventName || '').trim();
  if (!eventName) {
    return { ok: false, notFound: false, error: 'eventName is required', status: 0 };
  }

  const body = {
    eventName,
    conversationId: String(input?.conversationId || '').trim() || undefined,
    turnId: String(input?.turnId || '').trim() || undefined,
    payload: input?.payload || undefined,
    context: input?.context || undefined,
  };
  // Strip undefined keys so the JSON stays compact and matches server-side
  // DisallowUnknownFields strictness.
  Object.keys(body).forEach((k) => body[k] === undefined && delete body[k]);

  let response;
  try {
    response = await fetch(`${sdkBaseURL}${DISPATCH_PATH}`, {
      method: 'POST',
      credentials: 'include',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
      },
      body: JSON.stringify(body),
    });
  } catch (err) {
    return { ok: false, notFound: false, error: err?.message || 'network error', status: 0 };
  }

  if (response.status === 404) {
    return { ok: false, notFound: true, status: 404 };
  }

  if (!response.ok) {
    let message = '';
    try {
      message = (await response.text()) || '';
    } catch (_) {
      message = '';
    }
    // Treat "no callback registered" 400 from the server as a not-found signal
    // so callers can fall back cleanly. The server returns a clean text body.
    const body = String(message || '');
    const isNotFound = /no callback registered/i.test(body);
    return {
      ok: false,
      notFound: isNotFound,
      error: body || `dispatch failed (HTTP ${response.status})`,
      status: response.status,
    };
  }

  let out = null;
  try {
    out = await response.json();
  } catch (_) {
    out = null;
  }
  return {
    ok: true,
    notFound: false,
    tool: out?.tool,
    result: out?.result,
  };
}
