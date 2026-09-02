import { chatService } from './chatService';
import { scheduleService } from './scheduleService';
import { prepareAgentlyDataConnectorRequest } from './datasourceRequestContext';
import { reportingHostServices } from './reportingHostServices';
import {applyPermission} from './permissionService';

export const forgeHostServices = {
  chat: chatService,
  schedule: scheduleService,
  prepareDataConnectorRequest: prepareAgentlyDataConnectorRequest,
  applyPermission,
  ...reportingHostServices,
};
