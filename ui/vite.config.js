import { resolve } from 'path';
import path from 'path';
import fs from 'fs';
import { isIP } from 'node:net';
import {fileURLToPath} from 'node:url';
import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

const NET_LOG = '/tmp/browser-network.log';
const LOCAL_DEV_HOST = '::';
const LOCAL_DEV_PORT = 5173;

export const isLoopbackPeer = (address = '') => {
  if (address === '::1') return true;
  const ipv4 = address.startsWith('::ffff:') ? address.slice(7) : address;
  return isIP(ipv4) === 4 && ipv4.startsWith('127.');
};

export function loopbackOnlyDevPlugin() {
  return {
    name: 'loopback-only-dual-stack',
    configureServer(server) {
      if (server.config.server.host !== LOCAL_DEV_HOST) return;
      server.middlewares.use((req, res, next) => {
        if (isLoopbackPeer(req.socket.remoteAddress)) return next();
        res.statusCode = 403;
        res.end('Local development access only');
      });
      server.httpServer?.prependListener('upgrade', (_req, socket) => {
        if (!isLoopbackPeer(socket.remoteAddress)) socket.destroy();
      });
    }
  };
}

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

export const resolveProxyTarget = (env, options = {}) => {
  const requireExplicit = options?.requireExplicit === true;
  const candidates = [
    env.APPSERVER_URL,
    env.DATA_URL,
    env.AUTH_URL,
    env.APP_URL
  ];
  for (const candidate of candidates) {
    const value = String(candidate || '').trim();
    if (!value) continue;
    if (/^https?:\/\//i.test(value)) {
      return value.replace(/\/+$/, '');
    }
  }
  if (requireExplicit) {
    throw new Error(
      'Missing backend proxy target for Vite dev server. ' +
      'Set APPSERVER_URL (recommended) or DATA_URL/AUTH_URL/APP_URL to the live Agently backend, ' +
      'for example APPSERVER_URL=http://127.0.0.1:9191 npm run dev -- --host 127.0.0.1 --port 5173'
    );
  }
  return '';
};

export const resolveDevHost = (env = {}) => {
  return String(env.VITE_HOST || env.HOST || LOCAL_DEV_HOST).trim() || LOCAL_DEV_HOST;
};

export const resolveDevPort = (env = {}) => {
  return Number(env.VITE_PORT || env.PORT || LOCAL_DEV_PORT) || LOCAL_DEV_PORT;
};

export const isForgeReportingModule = (id = '', sourceRoot = '') => {
  const root = resolve(sourceRoot).replaceAll('\\', '/').replace(/\/+$/, '');
  const moduleId = String(id).replaceAll('\\', '/');
  return moduleId.includes(`${root}/src/reporting/`)
    || moduleId.includes(`${root}/src/components/dashboard/`);
};

export default defineConfig(({ mode, command }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const safeEnv = pickEnv(env, ['AUTH_URL', 'DATA_URL', 'APP_URL', 'APPSERVER_URL', 'VITE_FORGE_LOG_LEVEL']);
  const prodEnv = { AUTH_URL: '/', DATA_URL: '/', APPSERVER_URL: '/', ...safeEnv };
  const proxyTarget = command === 'serve'
    ? resolveProxyTarget(env, { requireExplicit: true })
    : resolveProxyTarget(env);
  const devHost = resolveDevHost(env);
  const devPort = resolveDevPort(env);
  const uiRoot = fileURLToPath(new URL('.', import.meta.url));
  const forgeRoot = resolve(env.FORGE_SOURCE_ROOT || resolve(uiRoot, '../../forge'));
  const coreRoot = resolve(env.AGENTLY_CORE_SOURCE_ROOT || resolve(uiRoot, '../../agently-core'));
  const forgeNodeModules = resolve(uiRoot, 'node_modules/forge');
  const appNodeModules = resolve(uiRoot, 'node_modules');
  const sharedDependency = name => fs.existsSync(resolve(appNodeModules, name))
    ? resolve(appNodeModules, name) : resolve(forgeRoot, 'node_modules', name);
  const forgeExports = JSON.parse(fs.readFileSync(resolve(forgeRoot, 'package.json'), 'utf8')).exports || {};
  const forgePublicAliases = Object.entries(forgeExports).flatMap(([key, entry]) => {
    const target = typeof entry === 'string' ? entry : entry.default;
    if (!key.startsWith('./') || !target?.startsWith('./src/')) return [];
    const escape = value => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    const extension = path.extname(target);
    const wildcard = '(.+?)' + (extension ? '(?:' + escape(extension) + ')?' : '');
    const suffix = key.slice(2).split('*').map(escape).join(wildcard);
    return [{find: new RegExp('^forge/' + suffix + '$'), replacement: resolve(forgeRoot, target).replace('*', '$1')}];
  });

  return {
    base: '/',
    resolve: {
      // Resolve package-local dependencies correctly with both npm and pnpm.
      // Shared React/Forge identities remain controlled by aliases and dedupe.
      preserveSymlinks: false,
      dedupe: [
        'react',
        'react-dom',
        '@uiw/react-codemirror',
        '@codemirror/state',
        '@codemirror/view',
        '@codemirror/language'
      ],
      alias: [...forgePublicAliases, ...Object.entries({
        'agently-core-ui-sdk': resolve(coreRoot, 'sdk/ts/src'),
        forge: resolve(forgeRoot, 'src'),
        react: resolve(appNodeModules, 'react'),
        'react-dom': resolve(appNodeModules, 'react-dom'),
        '@preact/signals-react': resolve(appNodeModules, '@preact/signals-react'),
        'react-rnd': sharedDependency('react-rnd'),
        're-resizable': sharedDependency('re-resizable'),
        'react-draggable': sharedDependency('react-draggable'),
        '@phosphor-icons/react': resolve(appNodeModules, '@phosphor-icons/react'),
        '@uiw/react-codemirror': resolve(uiRoot, 'node_modules/@uiw/react-codemirror'),
        '@codemirror/state': resolve(uiRoot, 'node_modules/@codemirror/state'),
        '@codemirror/view': resolve(uiRoot, 'node_modules/@codemirror/view'),
        '@codemirror/language': resolve(uiRoot, 'node_modules/@codemirror/language')
      }).map(([find, replacement]) => ({find, replacement}))]
    },
    define: {
      'process.env': prodEnv,
      global: 'window'
    },
    plugins: [loopbackOnlyDevPlugin(), react(), networkLoggerPlugin()],
    server: {
      host: devHost,
      port: devPort,
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
        allow: [uiRoot, forgeRoot, resolve(forgeRoot, 'src'), coreRoot, resolve(coreRoot, 'sdk/ts/src')],
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
    test: {
      // Prebundle linked CJS UI dependencies with the same React runtime as
      // the app; native Node require would resolve Forge's second React copy.
      deps: { optimizer: { web: { enabled: true, include: ['react', 'react/jsx-runtime', 'react/jsx-dev-runtime', 'react-dom', '@blueprintjs/core', 'react-rnd', 're-resizable', 'react-draggable'] } } },
      exclude: [
        '**/node_modules/**',
        '**/dist/**',
        '**/.cache/**',
        'vite.config.test.mjs',
        'src/services/forgeFenceContract.test.mjs',
        'src/components/lookups/chipEditing.test.mjs',
        'src/components/lookups/forgeBridge.test.mjs',
        'src/components/lookups/tokens.test.mjs',
        'src/styles/workspaceDialogLayer.test.mjs'
      ]
    },
    build: {
      sourcemap: false,
      assetsInlineLimit: 4096,
      cssCodeSplit: true,
      brotliSize: false,
      chunkSizeWarningLimit: 1200,
      rollupOptions: {
        input: { main: resolve(uiRoot, 'index.html'), mcpAppSandbox: resolve(uiRoot, 'mcp-app-sandbox.html') },
        output: {
          manualChunks(id) {
            if (id.includes('/node_modules/mermaid/')) return 'mermaid';
            if (id.includes('/node_modules/@codemirror/') || id.includes('/node_modules/codemirror/')) return 'code-editor';
            if (isForgeReportingModule(id, forgeRoot)) return 'forge-reporting';
            return undefined;
          }
        }
      }
    }
  };
});
