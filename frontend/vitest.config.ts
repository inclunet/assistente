import { defineConfig } from 'vitest/config';
import path from 'path';

export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@wailsjs': path.resolve(__dirname, './wailsjs'),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    clearMocks: true,
    setupFiles: ['src/vitest.setup.ts'],
    globals: true,
  },
});
