import { buildEnvelope, MCPUI_METHODS } from './appproto.js';

export const MCPUI_FORGE_GUEST_KEY = '__mcpuiForgeGuest';
export const MCPUI_HOST_KEY = '__mcpuiHost';
export const MCPUI_HOST_READY_EVENT = 'mcpui-host-ready';
export const MCPUI_HOST_TEARDOWN_EVENT = 'mcpui-host-teardown';
export const MCPUI_GUEST_BRIDGE_READY_EVENT = 'mcpui-guest-bridge-ready';

function hostState(target = globalThis.window) {
  if (!target || typeof target !== 'object') return {};
  const value = target[MCPUI_HOST_KEY];
  return value && typeof value === 'object' ? value : {};
}

function matchesHostBinding(target = globalThis.window, params = {}) {
  const current = hostState(target);
  return String(current.windowId || '').trim() !== ''
    && String(current.windowId || '').trim() === String(params?.windowId || '').trim()
    && String(current.resourceUri || '').trim() !== ''
    && String(current.resourceUri || '').trim() === String(params?.resourceUri || '').trim();
}

function clearHostBinding(target = globalThis.window) {
  if (!target || typeof target !== 'object') return;
  target[MCPUI_HOST_KEY] = {};
  try {
    target.dispatchEvent?.(new CustomEvent(MCPUI_HOST_TEARDOWN_EVENT, { detail: {} }));
  } catch (_) {}
}

function requireActiveHostBinding(target = globalThis.window) {
  const current = hostState(target);
  if (String(current.windowId || '').trim() === '' || String(current.resourceUri || '').trim() === '') {
    throw new Error('host not ready');
  }
  return current;
}

export function buildForgeGuestEnvelope(method, target = globalThis.window, params = {}) {
  const host = requireActiveHostBinding(target);
  return buildEnvelope(method, {
    windowId: String(host.windowId || '').trim(),
    resourceUri: String(host.resourceUri || '').trim(),
    ...params,
  });
}

export function installForgeGuestBridge(target = globalThis.window) {
  if (!target || typeof target !== 'object') {
    return () => {};
  }
  const handler = (event) => {
    const data = event?.data || {};
    if (data?.method === MCPUI_METHODS.HOST_READY && data?.params && typeof data.params === 'object') {
      target[MCPUI_HOST_KEY] = {
        windowId: String(data.params.windowId || '').trim(),
        resourceUri: String(data.params.resourceUri || '').trim(),
        allowedTools: Array.isArray(data.params.allowedTools) ? data.params.allowedTools : [],
        allowedToolBundles: Array.isArray(data.params.allowedToolBundles) ? data.params.allowedToolBundles : [],
      };
      try {
        target.dispatchEvent?.(new CustomEvent(MCPUI_HOST_READY_EVENT, { detail: target[MCPUI_HOST_KEY] }));
      } catch (_) {}
      return;
    }
    if (data?.method === MCPUI_METHODS.TEARDOWN && data?.params && typeof data.params === 'object') {
      if (matchesHostBinding(target, data.params)) {
        clearHostBinding(target);
      }
      return;
    }
    if (data?.method === MCPUI_METHODS.TOOL_RESULT) {
      if (!matchesHostBinding(target, data?.params)) {
        return;
      }
      target[MCPUI_HOST_KEY] = {
        ...hostState(target),
        lastToolResult: data.params || null,
      };
    }
  };
  target.addEventListener?.('message', handler);
  const existingHost = hostState(target);
  if (existingHost.windowId || existingHost.resourceUri) {
    try {
      target.dispatchEvent?.(new CustomEvent(MCPUI_HOST_READY_EVENT, { detail: existingHost }));
    } catch (_) {}
  }
  target[MCPUI_FORGE_GUEST_KEY] = {
    message(content = '') {
      target.parent?.postMessage(buildForgeGuestEnvelope(MCPUI_METHODS.MESSAGE, target, {
        content: String(content || '').trim(),
      }), '*');
    },
    openLink(url = '') {
      target.parent?.postMessage(buildForgeGuestEnvelope(MCPUI_METHODS.OPEN_LINK, target, {
        url: String(url || '').trim(),
      }), '*');
    },
    toolCall(name = '', args = {}, assistantText = '') {
      target.parent?.postMessage(buildForgeGuestEnvelope(MCPUI_METHODS.TOOLS_CALL, target, {
        name: String(name || '').trim(),
        arguments: args && typeof args === 'object' ? args : {},
        assistantText: String(assistantText || '').trim(),
      }), '*');
    },
  };
  try {
    target.dispatchEvent?.(new CustomEvent(MCPUI_GUEST_BRIDGE_READY_EVENT, {
      detail: { installed: true },
    }));
  } catch (_) {}
  return () => {
    target.removeEventListener?.('message', handler);
    try {
      delete target[MCPUI_FORGE_GUEST_KEY];
      delete target[MCPUI_HOST_KEY];
    } catch (_) {}
  };
}
