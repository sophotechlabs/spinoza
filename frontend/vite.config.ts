import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: '../web/dist',
    emptyOutDir: false,
  },
  server: {
    proxy: {
      '/ws': { target: 'ws://127.0.0.1:34115', ws: true },
      '/healthz': 'http://127.0.0.1:34115',
    },
  },
});
