import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';

export default defineConfig({
  root: __dirname,
  plugins: [react()],
  resolve: { alias: { '@': path.resolve(__dirname, '../../src'), '@wailsjs': path.resolve(__dirname, '../../wailsjs') } },
  build: { outDir: '../../../build/bin/agent-build/timeline-heap-assets', emptyOutDir: true },
});
