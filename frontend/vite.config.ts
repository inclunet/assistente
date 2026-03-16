import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';
import fs from 'node:fs';

const keepDistSentinel = () => {
  return {
    name: 'keep-dist-sentinel',
    closeBundle: async () => {
      const outDir = path.resolve(__dirname, 'dist');
      const keepPath = path.join(outDir, 'keep.txt');
      const content =
        'Este arquivo existe apenas para que `go test`/`go vet` não falhem quando `frontend/dist` ainda não foi buildado.\n\n' +
        '- O `//go:embed all:frontend/dist` exige pelo menos 1 arquivo embeddable.\n' +
        '- O build do Vite normalmente apaga o conteúdo do diretório e recria os artefatos.\n';

      await fs.promises.mkdir(outDir, { recursive: true });
      await fs.promises.writeFile(keepPath, content, 'utf8');
    },
  };
};

// https://vitejs.dev/config/
export default defineConfig({
  root: __dirname,
  plugins: [react(), keepDistSentinel()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@wailsjs': path.resolve(__dirname, './wailsjs'),
    },
    dedupe: ['react', 'react-dom'],
  },
  server: {
    port: 5173,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 5000,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return;

          if (id.includes('monaco-editor') || id.includes('@monaco-editor')) {
            return 'monaco';
          }

          if (id.includes('mermaid') || id.includes('cytoscape') || id.includes('katex')) {
            return 'mermaid';
          }

          if (
            id.includes('tiptap') ||
            id.includes('markdown-it') ||
            id.includes('react-markdown') ||
            id.includes('remark-') ||
            id.includes('rehype-') ||
            id.includes('react-syntax-highlighter')
          ) {
            return 'editor';
          }
        },
      },
    },
  },
})
