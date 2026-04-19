export function dispatchForgeUIAction(detail = {}) {
  if (typeof window === 'undefined') return;
  const payload = {
    ...detail,
    callback: detail?.callback || null,
  };
  window.lastForgeUIAction = payload;
  window.dispatchEvent(new CustomEvent('forge-ui-action', { detail: payload }));
}

export function subscribeForgeUIAction(callback) {
  if (typeof window === 'undefined' || typeof callback !== 'function') {
    return () => {};
  }
  const handler = (event) => callback(event?.detail || null, event);
  window.addEventListener('forge-ui-action', handler);
  return () => window.removeEventListener('forge-ui-action', handler);
}

function summarizeForgeUIAction(detail = {}) {
  const eventName = String(detail?.eventName || detail?.callback?.eventName || 'forge_ui_submit').trim();
  const tableId = String(detail?.tableId || '').trim();
  const selectedCount = Array.isArray(detail?.selectedRows) ? detail.selectedRows.length : 0;
  const unselectedCount = Array.isArray(detail?.unselectedRows) ? detail.unselectedRows.length : 0;
  const changedCount = Array.isArray(detail?.changedRows) ? detail.changedRows.length : 0;
  return [
    `Forge UI callback: ${eventName}`,
    tableId ? `tableId=${tableId}` : '',
    `selected=${selectedCount}`,
    `unselected=${unselectedCount}`,
    `changed=${changedCount}`,
    '',
    JSON.stringify(detail, null, 2),
  ].filter(Boolean).join('\n');
}

export function connectForgeUIActionsToChat(submitMessage, contextProvider) {
  if (typeof submitMessage !== 'function') return () => {};
  return subscribeForgeUIAction(async (detail) => {
    const context = typeof contextProvider === 'function' ? contextProvider() : contextProvider;
    if (!context) return;
    const message = summarizeForgeUIAction(detail);
    try {
      await submitMessage({ context, message });
    } catch (err) {
      console.error('forge-ui-action submit failed', err);
    }
  });
}

/**
 * Extracts the conversation id from a forge context without hard-wiring
 * the context shape. Looks at the common places the UI stashes it:
 *
 *   context.Context('conversations').handlers.dataSource.peekFormData().id
 *   context.conversationId
 *   context.windowState?.conversationId
 */
function extractConversationId(context) {
  if (!context) return '';
  try {
    const direct = String(context?.conversationId || '').trim();
    if (direct) return direct;
    const winCid = String(context?.windowState?.conversationId || '').trim();
    if (winCid) return winCid;
    const ctx = typeof context?.Context === 'function' ? context.Context('conversations') : null;
    const form = ctx?.handlers?.dataSource?.peekFormData?.();
    const fromForm = String(form?.id || '').trim();
    if (fromForm) return fromForm;
  } catch (_) {
    // Forge contexts can throw on unfamiliar shapes — fall through to ''.
  }
  return '';
}

/**
 * Reads an optional scope map (agencyId / advertiserId / campaignId /
 * adOrderId / audienceId) from the forge context so callbacks can carry
 * taxonomy identifiers into the payload template.
 */
function extractScope(context) {
  const out = {};
  if (!context) return out;
  const keys = ['agencyId', 'advertiserId', 'campaignId', 'adOrderId', 'audienceId'];
  for (const k of keys) {
    const v = context?.[k] ?? context?.windowState?.[k];
    if (v !== undefined && v !== null && String(v).trim() !== '') {
      out[k] = v;
    }
  }
  return out;
}

/**
 * Wires forge UI submit events to the workspace-declared callback router
 * with a conversational fallback.
 *
 * Flow per event:
 *   1. POST the event to `/v1/api/callbacks/dispatch` (see callbackService).
 *   2. On success → post a short confirmation message to chat so the
 *      submission is preserved in conversation history.
 *   3. On "no callback registered" (404 or clean 400 body) → fall back to
 *      the legacy chat-message path so the agent can handle the event
 *      conversationally.
 *   4. On any other error → surface via console + fall back to chat.
 *
 * @param {Function} submitMessage     chat submit function (same as
 *                                     connectForgeUIActionsToChat).
 * @param {Function|Object} contextProvider
 * @param {Function} [dispatchCallback] injectable override for testing; when
 *                                      omitted imports the real service.
 */
export function connectForgeUIActionsToCallbacksOrChat(submitMessage, contextProvider, dispatchCallback) {
  if (typeof submitMessage !== 'function') return () => {};
  const resolveDispatch = () => {
    if (typeof dispatchCallback === 'function') return dispatchCallback;
    return import('./callbackService').then((m) => m.dispatchCallback);
  };
  return subscribeForgeUIAction(async (detail) => {
    const context = typeof contextProvider === 'function' ? contextProvider() : contextProvider;
    if (!context) return;

    const eventName = String(detail?.eventName || detail?.callback?.eventName || '').trim();

    const dispatch = await resolveDispatch();
    if (eventName && typeof dispatch === 'function') {
      const input = {
        eventName,
        conversationId: extractConversationId(context),
        turnId: String(detail?.turnId || '').trim() || undefined,
        payload: {
          selectedRows: detail?.selectedRows,
          unselectedRows: detail?.unselectedRows,
          changedRows: detail?.changedRows,
          tableId: detail?.tableId,
        },
        context: extractScope(context),
      };

      let result;
      try {
        result = await dispatch(input);
      } catch (err) {
        console.error('forge-ui-action dispatch failed', err);
        result = { ok: false, notFound: false, error: err?.message || 'dispatch failed' };
      }

      if (result?.ok) {
        // Preserve audit trail — post a one-line confirmation.
        const message = `Forge UI callback dispatched: ${eventName}${result?.tool ? ` → ${result.tool}` : ''}`;
        try {
          await submitMessage({ context, message });
        } catch (err) {
          console.error('forge-ui-action confirmation submit failed', err);
        }
        return;
      }

      if (!result?.notFound) {
        console.error('forge-ui-action dispatch error:', result?.error || 'unknown');
        // Fall through to chat — preserves the user's submit rather than
        // dropping it silently.
      }
    }

    // Fallback: summarise + submit to chat (legacy conversational path).
    const message = summarizeForgeUIAction(detail);
    try {
      await submitMessage({ context, message });
    } catch (err) {
      console.error('forge-ui-action submit failed', err);
    }
  });
}
