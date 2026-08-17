import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

declare const process: { env: Record<string, string | undefined> };

export default defineConfig({
  define: {
    __SPINOZA_VERSION__: JSON.stringify(process.env.SPINOZA_VERSION ?? 'test'),
  },
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    testTimeout: 20000,
    setupFiles: ['./test/setup.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'text-summary', 'json', 'json-summary', 'html', 'lcov'],
      reportsDirectory: './coverage',
      include: ['src/**/*.{ts,tsx}'],
      exclude: [
        'node_modules/',
        'test/',
        '**/*.d.ts',
        '**/*.config.*',
        'src/main.tsx',
        'src/lib/monaco.ts',
        'src/lib/chart.ts',
        'src/lib/terminal.ts',
        'src/lib/types.ts',
      ],
      reportOnFailure: true,
      thresholds: {
        lines: 100,
        functions: 100,
        statements: 100,
        branches: 90,
      },
    },
  },
});
