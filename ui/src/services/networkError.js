const CONNECTIVITY_MESSAGES = [
  'failed to fetch',
  'networkerror',
  'load failed',
  'network request failed',
  'fetch failed',
  'the internet connection appears to be offline'
];

export function isConnectivityError(err) {
  const message = String(err?.message || err || '').toLowerCase();
  if (!message) return false;
  return CONNECTIVITY_MESSAGES.some((pattern) => message.includes(pattern));
}

export const BACKEND_UNAVAILABLE_LABEL = 'Service temporarily unavailable. Reconnecting...';
