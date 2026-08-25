import React from 'react';

export const ConversationViewContext = React.createContext({
  developerMode: false,
  showIntakeDetails: false,
  setShowIntakeDetails: () => {},
  toolFeedDock: 'inline',
  workspaceWindow: null,
  workspaceVisible: false,
  onOpenWorkspace: () => {},
});
