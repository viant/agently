import React from 'react';

export const ConversationViewContext = React.createContext({
  showExecutionDetails: true,
  setShowExecutionDetails: () => {},
});
