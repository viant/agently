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
      <SettingProvider endpoints={endpoints} connectorConfig={connectorConfig} authContext={AgentlyAuthContext} services={services}>
        <HotkeysProvider>
          <RouterProvider router={router} />
        </HotkeysProvider>
      </SettingProvider>
    </AgentlyAuthContext.Provider>
  );
}
