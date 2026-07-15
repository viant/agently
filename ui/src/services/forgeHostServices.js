import { chatService } from './chatService';
import { scheduleService } from './scheduleService';
import { prepareAgentlyDataConnectorRequest } from './datasourceRequestContext';
import { reportingHostServices } from './reportingHostServices';

export const forgeHostServices = {
  chat: chatService,
  schedule: scheduleService,
  prepareDataConnectorRequest: prepareAgentlyDataConnectorRequest,
  ...reportingHostServices,
};
