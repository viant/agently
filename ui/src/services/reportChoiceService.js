import {sdkBaseURL} from '../endpoint.js';
import {APIRequestError} from './httpClient';

// The host selects the report's declared datasource and uses its ordinary MCP
// executor. Browser requests carry logical identities and inputs only.
export function createReportChoiceLoader({uri}, fetchImpl = fetch) {
  return async (dataSourceId, inputs, {signal} = {}) => {
    const response = await fetchImpl(`${sdkBaseURL}/api/datasources/${encodeURIComponent(dataSourceId)}/fetch`, {
      method: 'POST', credentials: 'include', signal,
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({owner: {uri}, inputs}),
    });
    if (!response.ok) {
      let payload = null;
      try { payload = await response.json(); }
      catch (error) { if (signal?.aborted) throw error; }
      throw new APIRequestError(`Report choices could not be loaded (${response.status}).`, {
        status: response.status, payload, method: 'POST',
        path: `/api/datasources/${encodeURIComponent(dataSourceId)}/fetch`,
        transient: response.status >= 500,
      });
    }
    const body = await response.json();
    return {data: body.rows, dataInfo: body.dataInfo, metrics: body.metrics};
  };
}
