import { chatService } from './chatService';
import { scheduleService } from './scheduleService';
import { prepareAgentlyDataConnectorRequest } from './datasourceRequestContext';
import { reportingHostServices } from './reportingHostServices';
import {applyPermission} from './permissionService';
import {workspaceResources} from './workspaceResources';

export const forgeHostServices = {
	resourceFS: workspaceResources,
  chat: chatService,
  schedule: scheduleService,
  prepareDataConnectorRequest: prepareAgentlyDataConnectorRequest,
  applyPermission,
  workspaceRefresh: {loadMetadata: ({load}) => load()},
  authorizationRefresh: {mode: 'opening'},
  ...reportingHostServices,
};
