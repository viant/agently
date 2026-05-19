import { buildUISnapshot } from 'forge/core';
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
    return String(window.__forgeUIBridgeClientId || '').trim();
  } catch (_) {
    return '';
  }
}

export const connectorConfig = {
  window: {
    service: {
      endpoint: 'appAPI',
      uri: 'agently/forge/window'
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
    startupReadyEvent: 'forge:conversation-active',
    startupReadyTimeoutMs: 1200,
    snapshotOptions: {
      includeCollection: true,
      includeInlineMetadata: true,
    },
    snapshotBuilder: () => {
      const snapshot = buildUISnapshot({ includeCollection: true, includeInlineMetadata: true });
      return {
        ...snapshot,
        clientId: uiBridgeClientId(),
        conversationId: snapshotConversationId(snapshot) || resolveConversationSelection(MAIN_CHAT_WINDOW_ID),
      };
    },
  }
};
