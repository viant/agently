import { createRoot } from 'react-dom/client';
import App from './App.jsx';
import './index.css';

// Browser-side network request logger (dev only)
if (import.meta.env.DEV) {
  const _fetch = window.fetch;
  window.fetch = async function (input, init) {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input?.url ?? String(input);
    const method = init?.method || (input instanceof Request ? input.method : 'GET');
    const t0 = performance.now();
    console.group(`[net] ${method} ${url}`);
    try {
      const res = await _fetch(input, init);
      const ms = (performance.now() - t0).toFixed(0);
      console.log(`← ${res.status} ${res.statusText} (${ms}ms)`);
      console.groupEnd();
      return res;
    } catch (err) {
      const ms = (performance.now() - t0).toFixed(0);
      console.error(`← ERROR (${ms}ms)`, err);
      console.groupEnd();
      throw err;
    }
  };
}

createRoot(document.getElementById('root')).render(<App />);
