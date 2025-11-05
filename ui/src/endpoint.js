export const endpoints = {
    appAPI: {
        baseURL: process.env.APP_URL,
        output: {
            statusField: "status",
            dataField: "data"
        }
    },
    dataAPI: {
        baseURL: process.env.DATA_URL,
        output: {
            statusField: "status",
            dataField: "data"
        }
    },
    agentlyAPI: {
        baseURL: process.env.DATA_URL,
        output: {
            statusField: "status",
            dataField: "data"
        }
    }
};

// Lightweight header hook for Forge datasources hitting agentlyAPI.
// If a global AGENTLY_BEARER is present, attach Authorization automatically.
try {
  if (!window.__agentlyFetchWrapped) {
    const agentlyBase = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
    const origin = (typeof location !== 'undefined' && location.origin) ? location.origin : '';
    const origFetch = window.fetch.bind(window);
    window.fetch = async (input, init = {}) => {
      let url = '';
      try {
        url = typeof input === 'string' ? input : (input && input.url) || '';
        const method = (init && init.method) || 'GET';
        const isAbsolute = /^https?:\/\//i.test(url);
        const sameOrigin = isAbsolute ? (origin && url.startsWith(origin)) : true;
        const isAPI = url.startsWith('/v1/') || url.includes('/v1/');
        const matchesAgently = isAPI || (agentlyBase ? url.startsWith(agentlyBase) : sameOrigin);
        if (url && matchesAgently) {
          const token = window.AGENTLY_BEARER;
          if (token) {
            const hdrs = new Headers(init.headers || (typeof input !== 'string' ? input.headers : undefined) || {});
            if (!hdrs.has('Authorization')) hdrs.set('Authorization', `Bearer ${token}`);
            init.headers = hdrs;
          }
        }
        console.log('[agently.fetch] →', method, url);
      } catch (_) {}

      const resp = await origFetch(input, init);
      try {
        const ct = resp.headers.get('Content-Type') || '';
        const status = resp.status;
        if (!resp.ok) {
          // clone and log a small snippet to avoid consuming body
          const clone = resp.clone();
          const txt = await clone.text().catch(() => '');
          console.warn('[agently.fetch] ←', status, url, 'content-type:', ct, 'body:', (txt || '').slice(0, 220));
        } else {
          console.log('[agently.fetch] ←', status, url);
        }
      } catch (_) {}
      return resp;
    };
    window.__agentlyFetchWrapped = true;
  }
} catch (_) {}
