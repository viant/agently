// Ensure cookies are sent to the backend for all requests by default.
// Forge data connectors use window.fetch; they don’t pass credentials by
// default in dev cross-port setups (5173 -> 8080). This shim adds
// credentials: 'include' for API URLs when the caller hasn’t specified it.

import {endpoints} from './endpoint';

if (typeof window !== 'undefined' && typeof window.fetch === 'function') {
  const origFetch = window.fetch.bind(window);
  const apiBases = [endpoints?.appAPI?.baseURL, endpoints?.agentlyAPI?.baseURL]
    .map(v => String(v || '').replace(/\/+$/, ''))
    .filter(v => v);

  window.fetch = (input, init) => {
    const url = (typeof input === 'string') ? input : (input && input.url) || '';
    const needsCreds = apiBases.some(b => b && typeof url === 'string' && url.startsWith(b)) ||
      (typeof url === 'string' && (url.startsWith('/v1/api/') || url.startsWith('/v1/workspace/')));
    const nextInit = init ? {...init} : {};
    if (needsCreds && !('credentials' in nextInit)) {
      nextInit.credentials = 'include';
    }
    return origFetch(input, nextInit);
  };
}

