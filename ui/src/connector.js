import { buildUISnapshot } from 'forge/core';
import { getScopedConversationSelection, MAIN_CHAT_WINDOW_ID } from './services/conversationWindow';

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
    snapshotOptions: {
      includeCollection: true,
    },
    snapshotBuilder: () => ({
      conversationId: getScopedConversationSelection(MAIN_CHAT_WINDOW_ID),
      ...buildUISnapshot({ includeCollection: true }),
    }),
  }
};
