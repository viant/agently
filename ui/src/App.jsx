import React from 'react';
import "@blueprintjs/core/lib/css/blueprint.css";
import '@blueprintjs/table/lib/css/table.css';
import "@blueprintjs/datetime2/lib/css/blueprint-datetime2.css";

import {createBrowserRouter, RouterProvider} from 'react-router-dom';
import Root from './components/Root';
import {SettingProvider} from 'forge/core';
import {AuthProvider, AuthContext} from './AuthContext';

// Custom Forge widgets (file / URI reference)
import './widget/fileWidgetRegister.jsx';
// Patch Forge radio event adapter to accept value
import './widget/radioPatchRegister.jsx';

// Import your configurations
import {endpoints} from './endpoint';
import {connectorConfig} from "./connector.js";
import './setupFetchCredentials';
import { chatService } from './services/chatService.js';
import { modelService } from './services/modelService.js';
import { mcpService } from './services/mcpService.js';
import { agentService } from './services/agentService.js';
import { toolService } from './services/toolService.js';
import { toolRunnerService } from './services/toolRunnerService.js';
import { workflowRunnerService } from './services/workflowRunnerService.js';
import { workflowConversationService } from './services/workflowConversationService.js';
import { oauthService } from './services/oauthService.js';
import { scheduleService } from './services/scheduleService.js';
import { preferencesService } from './services/preferencesService.js';
import { guardrails } from './services/guardrails.js';
import CodeDiffDialog from './components/CodeDiffDialog.jsx';
import FileViewDialog from './components/FileViewDialog.jsx';

const router = createBrowserRouter([
    {
        path: '/',
        element: <Root/>,
        children: [],
    },
    {
        path: '/conversation/:id',
        element: <Root/>,
        children: [],
    },
    {
        path: '/ui',
        element: <Root/>,
        children: [],
    },
    {
        path: '/ui/conversation/:id',
        element: <Root/>,
        children: [],
    },
]);

const services = {
  chat: chatService,
  model: modelService,
  mcp: mcpService,
  agent: agentService,
  tool: toolService,
  toolRunner: toolRunnerService,
  workflowRunner: workflowRunnerService,
  workflowConversation: workflowConversationService,
  oauth: oauthService,
  schedule: scheduleService,
  preferences: preferencesService,
  guardrails,
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
        <>
          <RouterProvider router={router} />
          <CodeDiffDialog />
          <FileViewDialog />
        </>
      </SettingProvider>
    </AuthProvider>
  );
}

export default App;
