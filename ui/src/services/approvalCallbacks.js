import { executeApprovalCallbacks as sdkExecuteApprovalCallbacks } from 'agently-core-ui-sdk';
import { getWindowContext, selectedWindowId } from 'forge/core';

function resolveForgeContext(context = null) {
  if (context?.lookupHandler) return context;
  const windowId = String(selectedWindowId?.value || '').trim();
  if (!windowId) return null;
  return getWindowContext?.(windowId) || null;
}

export async function executeApprovalCallbacks({ meta = null, event = '', context = null, payload = {} } = {}) {
  const forgeContext = resolveForgeContext(context);
  return sdkExecuteApprovalCallbacks({
    meta,
    event,
    payload,
    resolveHandler: (handlerName) => forgeContext?.lookupHandler?.(handlerName) || null
  });
}
