import React from 'react';
import "@blueprintjs/core/lib/css/blueprint.css";
import '@blueprintjs/table/lib/css/table.css';
import "@blueprintjs/datetime2/lib/css/blueprint-datetime2.css";

import {createBrowserRouter, RouterProvider} from 'react-router-dom';
import Root from './components/Root';
import {SettingProvider} from 'forge/core';
import {AuthProvider, AuthContext} from './AuthContext';

// Import your configurations
import {endpoints} from './endpoint';
import {connectorConfig} from "./connector.js";
import { chatService } from './services/chatService.js';

const router = createBrowserRouter([
    {
        path: '/',
        element: <Root/>,
        children: [],
    },
    {
        path: '/ui',
        element: <Root/>,
        children: [],
    },
]);

const services = {
  chat: chatService,
};

// We no longer pass a plain object as `authContext`. Instead we use a proper
// React context implemented in `AuthContext.jsx`.  `AuthProvider` supplies a
// mock value so that the rest of the application – and the underlying `forge`
// library – can retrieve `authStates` & `defaultAuthProvider` without
// crashing.

const authContext = AuthContext;

function App() {
  return (
    <AuthProvider>
      <SettingProvider
        endpoints={endpoints}
        connectorConfig={connectorConfig}
        services={services}
        authContext={authContext}
      >
        <RouterProvider router={router} />
      </SettingProvider>
    </AuthProvider>
  );
}

export default App;
