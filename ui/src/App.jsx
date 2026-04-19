import React from 'react';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { HotkeysProvider } from '@blueprintjs/core';
import { SettingProvider } from 'forge/core';
import 'forge/packs/blueprint/index.jsx';
import Root from './components/Root';
import OAuthCallback from './components/OAuthCallback';
import { endpoints } from './endpoint';
import { connectorConfig } from './connector';
import { chatService } from './services/chatService';
import { scheduleService } from './services/scheduleService';
import { redirectToLogin } from './services/httpClient';
import { buildWebClientContext } from './services/clientContext';
import * as chatStore from './services/chatStore';
import { installChatStoreMirror } from './services/chatRuntime';

// The live chat feed reads from chatStore on the real UI path. Make the
// transcript/SSE mirror explicit at bootstrap instead of relying on the
// lazy CommonJS fallback inside chatRuntime.
installChatStoreMirror(chatStore);

const services = {
  chat: chatService,
  schedule: scheduleService
};

const AgentlyAuthContext = React.createContext({
  authStates: {},
  defaultAuthProvider: 'oauth',
  handleUnauthorized: () => {}
});

const authContext = {
  authStates: {},
  defaultAuthProvider: 'oauth',
  handleUnauthorized: () => {
    redirectToLogin();
  }
};

const webClientContext = buildWebClientContext();
const targetContext = {
  platform: webClientContext.platform,
  formFactor: webClientContext.formFactor,
  capabilities: webClientContext.capabilities
};

const router = createBrowserRouter([
  { path: '/v1/api/auth/oauth/callback', element: <OAuthCallback /> },
  { path: '/', element: <Root /> },
  { path: '/ui', element: <Root /> },
  { path: '/v1/conversation/:id', element: <Root /> },
  { path: '/conversation/:id', element: <Root /> },
  { path: '/ui/conversation/:id', element: <Root /> }
]);

export default function App() {
  return (
    <AgentlyAuthContext.Provider value={authContext}>
      <SettingProvider endpoints={endpoints} connectorConfig={connectorConfig} authContext={AgentlyAuthContext} services={services} targetContext={targetContext}>
        <HotkeysProvider>
          <RouterProvider router={router} />
        </HotkeysProvider>
      </SettingProvider>
    </AgentlyAuthContext.Provider>
  );
}
