import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// Builds one script and one stylesheet. scripts/inline.mjs then puts both into web/index.html,
// so the Go server hands out a single file and the Docker build needs no Node.
export default defineConfig({
  plugins: [svelte()],
  build: {
    lib: { entry: 'src/main.js', formats: ['iife'], name: 'LobbyApp', fileName: () => 'app.js', cssFileName: 'app' },
    outDir: 'dist',
    emptyOutDir: true,
    minify: true,
    sourcemap: false,
  },
});
