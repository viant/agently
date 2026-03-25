import { resolve } from 'path';
import path from 'path';
import fs from 'fs';
import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

const NET_LOG = '/tmp/browser-network.log';

function networkLoggerPlugin() {
  return {
    name: 'network-logger',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const start = Date.now();
        const origEnd = res.end.bind(res);
        res.end = function (...args) {
          const ms = Date.now() - start;
          const line = `${new Date().toISOString()} ${req.method} ${req.url} → ${res.statusCode} (${ms}ms)\n`;
          fs.appendFile(NET_LOG, line, () => {});
          return origEnd(...args);
        };
        next();
      });
    }
  };
}

const pickEnv = (env, keys) => {
  const out = {};
  for (const key of keys) {
    if (Object.prototype.hasOwnProperty.call(env, key)) {
      out[key] = env[key];
    }
  }
  return out;
};

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const safeEnv = pickEnv(env, ['AUTH_URL', 'DATA_URL', 'APP_URL', 'APPSERVER_URL', 'VITE_FORGE_LOG_LEVEL']);
  const prodEnv = { AUTH_URL: '/', DATA_URL: '/', APPSERVER_URL: '/', ...safeEnv };
  const proxyTarget = String(env.DATA_URL || env.APP_URL || 'http://localhost:8585').replace(/\/+$/, '');
  const uiRoot = __dirname;
  const forgeRoot = resolve(__dirname, '../../forge');
  const forgeNodeModules = resolve(uiRoot, 'node_modules/forge');
  const appNodeModules = resolve(uiRoot, 'node_modules');

  return {
    base: '/',
    resolve: {
      preserveSymlinks: true,
      dedupe: [
        'react',
        'react-dom',
        '@uiw/react-codemirror',
        '@codemirror/state',
        '@codemirror/view',
        '@codemirror/language'
      ],
      alias: {
        'agently-core-ui-sdk': resolve(__dirname, '../../agently-core/sdk/ts/src'),
        forge: resolve(forgeRoot, 'src'),
        react: resolve(appNodeModules, 'react'),
        'react-dom': resolve(appNodeModules, 'react-dom'),
        '@preact/signals-react': resolve(appNodeModules, '@preact/signals-react'),
        '@uiw/react-codemirror': resolve(__dirname, 'node_modules/@uiw/react-codemirror'),
        '@codemirror/state': resolve(__dirname, 'node_modules/@codemirror/state'),
        '@codemirror/view': resolve(__dirname, 'node_modules/@codemirror/view'),
        '@codemirror/language': resolve(__dirname, 'node_modules/@codemirror/language')
      }
    },
    define: {
      'process.env': prodEnv,
      global: 'window'
    },
    plugins: [react(), networkLoggerPlugin()],
    server: {
      host: 'localhost',
      port: 5173,
      watch: {
        followSymlinks: true,
        ignored: (watchPath) => {
          const normalized = String(watchPath || '');
          if (!normalized) return false;
          if (normalized.includes(`${path.sep}.git${path.sep}`)) return true;
          if (normalized.includes(`${path.sep}node_modules${path.sep}`)) {
            return !(normalized.startsWith(forgeNodeModules) || normalized.startsWith(forgeRoot));
          }
          return false;
        }
      },
      fs: {
        allow: [uiRoot, forgeRoot, resolve(forgeRoot, 'src')],
        strict: false
      },
      proxy: {
        '/v1': {
          target: proxyTarget,
          // Let the SPA handle the OAuth callback route so the OAuthCallback
          // component can exchange the code via POST. Without this bypass,
          // the proxy forwards the browser GET to the backend which consumes
          // the auth code and returns its own HTML, preventing the SPA from
          // completing the flow.
          bypass(req) {
            if (req.url && req.url.startsWith('/v1/api/auth/oauth/callback') && req.method === 'GET') {
              return req.url;
            }
          },
          configure: (proxy) => {
            proxy.on('proxyReq', (proxyReq, req) => {
              const host = req.headers.host || 'localhost:5173';
              proxyReq.setHeader('X-Forwarded-Host', host);
              proxyReq.setHeader('X-Forwarded-Proto', 'http');
            });
          }
        },
        '/healthz': proxyTarget,
        '/upload': proxyTarget,
        '/download': proxyTarget
      }
    },
    optimizeDeps: {
      include: ['@blueprintjs/core', '@blueprintjs/icons', '@phosphor-icons/react', '@preact/signals-react'],
      exclude: ['forge']
    },
    build: {
      sourcemap: false,
      assetsInlineLimit: 100000000,
      cssCodeSplit: true,
      brotliSize: false,
      chunkSizeWarningLimit: 1200,
      rollupOptions: {
        output: {
          manualChunks(id) {
            if (!id) return null;
            if (id.includes('/node_modules/')) {
              if (id.includes('/react/') || id.includes('/react-dom/') || id.includes('/scheduler/')) {
                return 'react-vendor';
              }
              if (id.includes('/mermaid/') || id.includes('/katex/')) {
                return 'diagram-vendor';
              }
              if (id.includes('/cytoscape') || id.includes('/dagre') || id.includes('/cose-base') || id.includes('/cytoscape-cose-bilkent')) {
                return 'diagram-vendor';
              }
              if (id.includes('/recharts/') || id.includes('/d3-')) {
                return 'chart-vendor';
              }
            }
            return null;
          }
        }
      }
    }
  };
});
