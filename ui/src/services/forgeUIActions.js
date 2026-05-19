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
  const callbackType = String(detail?.callback?.type || '').trim().toLowerCase();
  const plannerSubmit = detail?.plannerSubmit && typeof detail.plannerSubmit === 'object' ? detail.plannerSubmit : null;
  if (callbackType === 'llm_event' && plannerSubmit) {
    return [
      `Planner submit event: ${eventName}`,
      tableId ? `tableId=${tableId}` : '',
      plannerSubmit?.domain ? `domain=${plannerSubmit.domain}` : '',
      plannerSubmit?.submitIntent ? `submitIntent=${plannerSubmit.submitIntent}` : '',
      '',
      JSON.stringify({
        eventName,
        tableId,
        plannerSubmit,
        selectedRows: detail?.selectedRows || [],
      }, null, 2),
    ].filter(Boolean).join('\n');
  }
  const summaryPayload = plannerSubmit
    ? {
        eventName,
        tableId,
        plannerSubmit,
        selectedRows: detail?.selectedRows || [],
      }
    : detail;
  return [
    `Forge UI callback: ${eventName}`,
    tableId ? `tableId=${tableId}` : '',
    plannerSubmit?.domain ? `domain=${plannerSubmit.domain}` : '',
    plannerSubmit?.submitIntent ? `submitIntent=${plannerSubmit.submitIntent}` : '',
    `selected=${selectedCount}`,
    `unselected=${unselectedCount}`,
    `changed=${changedCount}`,
    '',
    JSON.stringify(summaryPayload, null, 2),
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

function extractCallbackContext(detail = {}) {
  const explicit = detail?.callbackContext ?? detail?.context ?? detail?.callback?.context;
  if (!explicit || typeof explicit !== 'object' || Array.isArray(explicit)) {
    return undefined;
  }
  const cleaned = {};
  Object.entries(explicit).forEach(([key, value]) => {
    const normalizedKey = String(key || '').trim();
    if (!normalizedKey) return;
    if (value === undefined) return;
    cleaned[normalizedKey] = value;
  });
  return Object.keys(cleaned).length > 0 ? cleaned : undefined;
}

function normalizeStageName(value = '') {
  const raw = String(value || '').trim();
  if (!raw) return '';
  return raw.replace(/_/g, ' ').replace(/\b\w/g, (m) => m.toUpperCase());
}

function summarizeLifecycleTransition(lifecycle = null, mode = 'success', reason = '') {
  if (!lifecycle || typeof lifecycle !== 'object' || Array.isArray(lifecycle)) {
    return '';
  }
  const current = normalizeStageName(lifecycle.currentStage || '');
  const validation = normalizeStageName(lifecycle.validationStage || '');
  const success = normalizeStageName(lifecycle.successStage || '');
  const followUp = normalizeStageName(lifecycle.followUpStage || '');
  if (mode === 'blocked') {
    const stage = validation || current || 'Validate';
    const suffix = String(reason || '').trim();
    return suffix
      ? `Recommendation lifecycle blocked at ${stage}: ${suffix}`
      : `Recommendation lifecycle blocked at ${stage}.`;
  }
  if (!current && !success && !followUp) {
    return '';
  }
  let summary = `Recommendation lifecycle advanced: ${current || 'Review'} -> ${success || 'Execute'}.`;
  if (followUp) {
    summary += ` Next stage: ${followUp}.`;
  }
  return summary;
}

function buildPlannerLLMSubmit(detail = {}) {
  const plannerSubmit = detail?.plannerSubmit && typeof detail.plannerSubmit === 'object' ? detail.plannerSubmit : null;
  const guidedTool = String(plannerSubmit?.toolGuidance?.tool || '').trim();
  const guidedToolBundle = String(plannerSubmit?.toolGuidance?.toolBundle || '').trim();
  return {
    content: 'Execute the planner submit event using the structured plannerSubmitEvent context. If plannerSubmitEvent.plannerSubmit.toolGuidance.tool is present, attempt that guided tool or its review flow before answering. Do not summarize selected rows in prose unless execution is blocked after attempting the guided path.',
    displayQuery: String(detail?.callbackContext?.displayQuery || detail?.context?.displayQuery || 'Submitted planner changes.').trim(),
    tools: guidedTool ? [guidedTool] : undefined,
    toolBundles: guidedToolBundle ? [guidedToolBundle] : undefined,
    context: {
      plannerSubmitEvent: {
        eventName: String(detail?.eventName || detail?.callback?.eventName || '').trim(),
        tableId: String(detail?.tableId || '').trim() || undefined,
        selectionField: String(detail?.selectionField || '').trim() || undefined,
        columns: Array.isArray(detail?.columns) ? detail.columns : undefined,
        plannerSubmit: plannerSubmit || undefined,
        selectedRows: Array.isArray(detail?.selectedRows) ? detail.selectedRows : [],
      },
    },
  };
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
    const callbackType = String(detail?.callback?.type || '').trim().toLowerCase();

    if (callbackType === 'llm_event') {
      const message = buildPlannerLLMSubmit(detail);
      try {
        await submitMessage({ context, message });
      } catch (err) {
        console.error('forge-ui-action llm_event submit failed', err);
      }
      return;
    }

    const dispatch = await resolveDispatch();
    if (eventName && typeof dispatch === 'function') {
      const input = {
        eventName,
        conversationId: extractConversationId(context),
        turnId: String(detail?.turnId || '').trim() || undefined,
        payload: {
          selectedRows: detail?.selectedRows,
          ...(detail?.plannerSubmit ? {} : {
            unselectedRows: detail?.unselectedRows,
            changedRows: detail?.changedRows,
          }),
          tableId: detail?.tableId,
          plannerSubmit: detail?.plannerSubmit || undefined,
        },
        context: extractCallbackContext(detail),
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
        const stageMessage = summarizeLifecycleTransition(input.context?.stageLifecycle, 'success');
        const message = [
          `Forge UI callback dispatched: ${eventName}${result?.tool ? ` → ${result.tool}` : ''}`,
          stageMessage,
        ].filter(Boolean).join(' ');
        try {
          await submitMessage({ context, message });
        } catch (err) {
          console.error('forge-ui-action confirmation submit failed', err);
        }
        return;
      }

      if (result?.blocked) {
        const stageMessage = summarizeLifecycleTransition(input.context?.stageLifecycle, 'blocked', result?.error || '');
        const message = [
          `Forge UI callback blocked: ${result?.error || eventName}`,
          stageMessage,
        ].filter(Boolean).join(' ');
        try {
          await submitMessage({ context, message });
        } catch (err) {
          console.error('forge-ui-action blocked confirmation submit failed', err);
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
