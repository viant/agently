import {resolve} from 'node:path';
import baseConfig from './vite.config.js';

export default async function themeProofConfig(env) {
  const base = typeof baseConfig === 'function' ? await baseConfig(env) : baseConfig;
  return {...base, build: {...base.build,
    outDir: resolve(import.meta.dirname, 'theme-proof-dist'),
    rollupOptions: {...base.build?.rollupOptions, input: {
      app: resolve(import.meta.dirname, 'index.html'),
      proof: resolve(import.meta.dirname, 'workspace-theme-proof.html'),
    }},
  }};
}
