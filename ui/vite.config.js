import {resolve} from 'path'
import path from 'path'
import {defineConfig, loadEnv} from 'vite'
import react from '@vitejs/plugin-react'



const pickEnv = (env, keys) => {
    const out = {}
    for (const key of keys) {
        if (Object.prototype.hasOwnProperty.call(env, key)) {
            out[key] = env[key]
        }
    }
    return out
}

// https://vitejs.dev/config/
export default defineConfig(({mode}) => {
    // SECURITY: Never inject the full process env into the client bundle.
    // Vite's `define` inlines values at build time and would leak any secrets
    // present in the shell environment (e.g. API keys) into built JS assets.
    const env = loadEnv(mode, process.cwd(), '');
    const safeEnv = pickEnv(env, ['AUTH_URL', 'DATA_URL', 'APP_URL', 'APPSERVER_URL', 'VITE_FORGE_LOG_LEVEL'])
    const prodEnv = {AUTH_URL: '/', DATA_URL: '/', APPSERVER_URL: '/', ...safeEnv}
    const uiRoot = __dirname;
    const forgeRoot = resolve(__dirname, '../../forge');
    const forgeNodeModules = resolve(uiRoot, 'node_modules/forge');
    return {
        base: '/',
        resolve: {
            // Prevent duplicated instances when using linked/local packages (e.g., forge)
            // Also preserve symlinks so Vite can watch local linked packages under node_modules.
            preserveSymlinks: true,
            dedupe: [
                'react',
                'react-dom',
                '@uiw/react-codemirror',
                '@codemirror/state',
                '@codemirror/view',
                '@codemirror/language',
                '@codemirror/highlight',
            ],
            alias: {
                '@uiw/react-codemirror': resolve(__dirname, 'node_modules/@uiw/react-codemirror'),
                '@codemirror/state': resolve(__dirname, 'node_modules/@codemirror/state'),
                '@codemirror/view': resolve(__dirname, 'node_modules/@codemirror/view'),
                '@codemirror/language': resolve(__dirname, 'node_modules/@codemirror/language'),
                '@codemirror/highlight': resolve(__dirname, 'node_modules/@codemirror/highlight'),
            }
        },
        // Keep default module resolution; forge package is resolved by workspace
        define: {
            'process.env': prodEnv,
            global: 'window'
        },
        plugins: [
            react(),

        ],
        server: {
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
                },
            },
            fs: {
                // Allow serving both the UI itself and the locally linked `forge` sources.
                // When `allow` is set, Vite replaces its default allow list.
                allow: [uiRoot, forgeRoot, resolve(forgeRoot, 'src')],
                strict: false,
            },
            // proxy: {
            //     // Proxy API calls to backend during dev so relative '/v1/*' works
            //     '/v1': {
            //         target: prodEnv.DATA_URL || 'http://localhost:8084/',
            //         changeOrigin: true,
            //         // do not rewrite; paths already include /v1
            //     },
            //     '/upload': {
            //         target: prodEnv.DATA_URL || 'http://localhost:8084/',
            //         changeOrigin: true,
            //     },
            //     '/download': {
            //         target: prodEnv.DATA_URL || 'http://localhost:8084/',
            //         changeOrigin: true,
            //     },
            // },
        },

        optimizeDeps: {
            include: [
                '@blueprintjs/core',
                '@blueprintjs/icons',
                '@blueprintjs/datetime',
                '@blueprintjs/select',
                '@blueprintjs/datetime2',
                '@phosphor-icons/react'
            ],
            exclude: ["forge"], // Prevents caching
        },

        build: {
            sourcemap: false,
            rollupOptions: {
                input: {
                    main: resolve(__dirname, 'index.html'),
                },
                output: {
                    // Provide global variables to use in the UMD build
                    // for externalized deps
                    globals: {
                        'process.env': prodEnv
                    },
                },
            },
            assetsInlineLimit: 100000000, // Increase limit to inline larger assets
            cssCodeSplit: false, // Ensure all CSS is inlined
            brotliSize: false, // Disable brotli size report to speed up builds
            chunkSizeWarningLimit: 2000 // Increase chunk size limit
        }
    }
})
