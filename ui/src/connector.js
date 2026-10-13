import { buildUISnapshot, ensureUIBridgeClientId } from 'forge/core';
import { MAIN_CHAT_WINDOW_ID, resolveConversationSelection } from './services/conversationWindow';

function firstString(...values) {
  for (const value of values) {
    const text = String(value || '').trim();
    if (text) return text;
  }
  return '';
}

function conversationIdFromWindow(win = null) {
  if (!win || typeof win !== 'object') return '';
  return firstString(
    win?.dataSources?.conversations?.form?.id,
    win?.parameters?.conversations?.form?.id,
    win?.parameters?.messages?.input?.parameters?.convID,
    win?.parameters?.conversationId
  );
}

export function snapshotConversationId(snapshot = null) {
  const windows = Array.isArray(snapshot?.windows) ? snapshot.windows : [];
  const mainWindow = windows.find((entry) => String(entry?.windowId || '').trim() === MAIN_CHAT_WINDOW_ID);
  const mainConversationId = conversationIdFromWindow(mainWindow);
  if (mainConversationId) return mainConversationId;
  return '';
}

function uiBridgeClientId() {
  try {
    return String(window.__forgeUIBridgeClientId || ensureUIBridgeClientId() || '').trim();
  } catch (_) {
    return '';
  }
}

export const connectorConfig = {
  workspace: { tabs: String(import.meta.env.VITE_WORKSPACE_TABS || 'auto').trim().toLowerCase() },
  mcpUI: {
    sandboxUrl: String(import.meta.env.VITE_MCP_APPS_SANDBOX_URL || '').trim(),
    legacyAdapter: String(import.meta.env.VITE_MCP_UI_LEGACY_ADAPTER || '').trim(),
    // Resource metadata is a request; deployments must opt in before an
    // MCP/UI iframe may become a conversation Workspace surface.
    allowWorkspacePlacement: String(import.meta.env.VITE_MCP_UI_WORKSPACE_PLACEMENT || '').trim().toLowerCase() === 'true',
  },
  window: {
    service: {
      endpoint: 'appAPI',
      uri: 'agently/forge/window',
      includeTargetContext: true
    }
  },
  navigation: {
    service: {
      endpoint: 'appAPI',
      uri: 'agently/forge/navigation',
      includeTargetContext: true
    }
  },
  uiBridge: {
    url: '/v1/ui/rpc',
    transport: 'http',
    // Agently refreshes its session with this event; Forge's default event
    // name is different, so an early unauthorized handshake needs this opt-in.
    authReadyEvents: ['agently:authorized', 'forge:authorized'],
    snapshotEvents: ['forge:conversation-select', 'forge:conversation-new', 'forge:conversation-active', 'agently:authorized', 'forge:authorized', 'popstate'],
    pollWhenHidden: true,
    // Keep background workspaces responsive without letting an immediately
    // completed/invalidated long-poll turn into a tight request loop.
    pollCycleDelayMs: 1000,
    startupReadyEvent: 'forge:conversation-active',
    startupReadyTimeoutMs: 1200,
    snapshotOptions: {
      includeCollection: true,
      includeInlineMetadata: true,
      includeFullWindowForm: true,
    },
    snapshotBuilder: () => {
      const snapshot = buildUISnapshot({ includeCollection: true, includeInlineMetadata: true, includeFullWindowForm: true });
      return {
        ...snapshot,
        clientId: uiBridgeClientId(),
        // The active route is the authoritative workspace scope. Restored form
        // state can briefly describe the previously selected conversation.
        conversationId: resolveConversationSelection(MAIN_CHAT_WINDOW_ID) || snapshotConversationId(snapshot),
      };
    },
  }
};
