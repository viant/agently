import {resolve} from 'path'
import {defineConfig, loadEnv} from 'vite'
import react from '@vitejs/plugin-react'



// https://vitejs.dev/config/
export default defineConfig(({mode}) => {
    const env = loadEnv(mode, process.cwd(), '');
    const prodEnv = {AUTH_URL: '/', DATA_URL: '/', APPSERVER_URL: '/', ...env}
    return {
        base: '/',
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

