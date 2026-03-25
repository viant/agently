import React from 'react';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { SettingProvider } from 'forge/core';
import 'forge/packs/blueprint/index.jsx';
import Root from './components/Root';
import OAuthCallback from './components/OAuthCallback';
import { endpoints } from './endpoint';
import { connectorConfig } from './connector';
import { chatService } from './services/chatService';
import { scheduleService } from './services/scheduleService';

const services = {
  chat: chatService,
  schedule: scheduleService
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
    <SettingProvider endpoints={endpoints} connectorConfig={connectorConfig} services={services}>
      <RouterProvider router={router} />
    </SettingProvider>
  );
}
