import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

declare const process: { env: Record<string, string | undefined> };

export default defineConfig({
  define: {
    __SPINOZA_VERSION__: JSON.stringify(process.env.SPINOZA_VERSION ?? 'dev'),
  },
  plugins: [react(), tailwindcss()],
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
