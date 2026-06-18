import { chatService } from './chatService';
import { scheduleService } from './scheduleService';
import { prepareAgentlyDataConnectorRequest } from './datasourceRequestContext';
import {
  getReportExportArtifact,
  getReportExportStatus,
  submitReportExportRequest,
} from './reportExportService';

export const forgeHostServices = {
  chat: chatService,
  schedule: scheduleService,
  prepareDataConnectorRequest: prepareAgentlyDataConnectorRequest,
  reportExport: {
    submitRequest: submitReportExportRequest,
    getStatus: getReportExportStatus,
    getArtifact: getReportExportArtifact,
  },
};
