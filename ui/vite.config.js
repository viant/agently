import {resolve} from 'path'
import {defineConfig, loadEnv} from 'vite'
import react from '@vitejs/plugin-react'



// https://vitejs.dev/config/
export default defineConfig(({mode}) => {
    const env = loadEnv(mode, process.cwd(), '');
    const prodEnv = {AUTH_URL: '/', DATA_URL: '/', APPSERVER_URL: '/', ...env}
    return {
        base: '/',
        resolve: {
            // Prevent duplicated instances when using linked/local packages (e.g., forge)
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
            'process.env': env,
            global: 'window'
        },
        plugins: [
            react(),

        ],
        server: {
            watch: {
                ignored: ["!../../forge/**"], // Watch UI folder
            },
            proxy: {
                // Proxy API calls to backend during dev so relative '/v1/*' works
                '/v1': {
                    target: prodEnv.DATA_URL || 'http://localhost:8081/',
                    changeOrigin: true,
                    // do not rewrite; paths already include /v1
                },
                '/upload': {
                    target: prodEnv.DATA_URL || 'http://localhost:8081/',
                    changeOrigin: true,
                },
                '/download': {
                    target: prodEnv.DATA_URL || 'http://localhost:8081/',
                    changeOrigin: true,
                },
            },
        },

        optimizeDeps: {
            include: [
                '@blueprintjs/core',
                '@blueprintjs/icons',
                '@blueprintjs/datetime',
                '@blueprintjs/select',
                '@blueprintjs/popover2',
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
