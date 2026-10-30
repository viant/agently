import {applicationScope} from './applicationScope';
import { chatService } from './chatService';
import { scheduleService } from './scheduleService';
import { prepareAgentlyDataConnectorRequest } from './datasourceRequestContext';
import { reportingHostServices } from './reportingHostServices';
import {applyPermission} from './permissionService';
import {workspaceResources} from './workspaceResources';
import {createPublishedManagementHostWindow} from './publishedManagementHostWindow';
import {getAuthenticatedWindowCacheScope} from './agentlyClient';
import {recoverPublishedWindowBinding} from './publishedWindowBindingRecovery';

export const forgeHostServices = {
  applicationScope,
  createHostWindow:createPublishedManagementHostWindow,
  windowModelCacheScope:getAuthenticatedWindowCacheScope,
	resourceFS: workspaceResources,
  chat: chatService,
  schedule: scheduleService,
  prepareDataConnectorRequest: prepareAgentlyDataConnectorRequest,
  recoverWindowBinding: recoverPublishedWindowBinding,
  applyPermission,
  workspaceRefresh: {loadMetadata: ({load}) => load()},
  authorizationRefresh: {mode: 'opening'},
  ...reportingHostServices,
};
