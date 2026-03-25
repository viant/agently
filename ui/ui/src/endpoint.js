/**
 * Single source of truth for all API base URLs.
 *
 * All browser-side URLs use relative paths so requests go through the
 * Vite dev proxy (localhost:5173 → backend) in development and stay
 * same-origin in production. This avoids CORS issues entirely.
 */

/**
 * SDK base URL — always relative.
 */
export const sdkBaseURL = '/v1';

/**
 * Forge SettingProvider endpoint map — also relative.
 */
export const endpoints = {
  appAPI: {
    baseURL: '/v1/api/',
    statusField: 'status',
    dataField: 'data'
  },
  dataAPI: {
    baseURL: '/',
    statusField: 'status',
    dataField: 'data'
  },
  agentlyAPI: {
    baseURL: '/',
    statusField: 'status',
    dataField: 'data'
  }
};
