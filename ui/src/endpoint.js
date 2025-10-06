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
    window.fetch = (input, init = {}) => {
      try {
        const url = typeof input === 'string' ? input : (input && input.url) || '';
        const isAbsolute = /^https?:\/\//i.test(url);
        const sameOrigin = isAbsolute ? (origin && url.startsWith(origin)) : true;
        const matchesAgently = agentlyBase ? url.startsWith(agentlyBase) : sameOrigin;
        if (url && matchesAgently) {
          const token = window.AGENTLY_BEARER;
          if (token) {
            const hdrs = new Headers(init.headers || (typeof input !== 'string' ? input.headers : undefined) || {});
            if (!hdrs.has('Authorization')) hdrs.set('Authorization', `Bearer ${token}`);
            init.headers = hdrs;
          }
        }
      } catch (_) {}
      return origFetch(input, init);
    };
    window.__agentlyFetchWrapped = true;
  }
} catch (_) {}
