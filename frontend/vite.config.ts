import { defineConfig } from 'vite';
import type { Plugin } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { DEFAULT_THEME_FILE } from './src/lib/defaultTheme';

declare const process: { env: Record<string, string | undefined> };

const DEFAULT_THEME_PLACEHOLDER = '__DEFAULT_THEME__';

function paintDefaultTheme(): Plugin {
  return {
    name: 'spinoza-default-theme',
    transformIndexHtml(html) {
      const literal = JSON.stringify(DEFAULT_THEME_FILE).replace(/</g, '\\u003c');
      return html.replace(DEFAULT_THEME_PLACEHOLDER, literal);
    },
  };
}

export default defineConfig({
  define: {
    __SPINOZA_VERSION__: JSON.stringify(process.env.SPINOZA_VERSION ?? 'dev'),
  },
  plugins: [react(), tailwindcss(), paintDefaultTheme()],
  build: {
    outDir: '../web/dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          monaco: ['monaco-editor', 'monaco-yaml', '@monaco-editor/react'],
          xterm: ['@xterm/xterm', '@xterm/addon-fit'],
          graph: ['@xyflow/react', '@dagrejs/dagre'],
          charts: ['uplot'],
        },
      },
    },
  },
  server: {
    proxy: {
      '/ws': { target: 'ws://127.0.0.1:34115', ws: true },
      '/api': 'http://127.0.0.1:34115',
      '/healthz': 'http://127.0.0.1:34115',
    },
  },
});
