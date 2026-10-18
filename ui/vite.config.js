import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// Two builds from the same source.
//
//   npm run build         the real one. An ES module app with the admin console and moderator dashboard in a
//                         separate file that is only downloaded when a moderator or admin opens it.
//                         scripts/site.mjs writes web/index.html and web/assets/ (hashed names, kept by browsers).
//
//   npm run build:full    everything in one script, the way the app was built before. The page tests use it,
//                         because the test browser (jsdom) cannot download modules on demand. The staff screens
//                         are the same code in both builds; only the way they are loaded differs.
export default defineConfig(({ mode }) => {
  const full = mode === 'full';
  return {
    plugins: [svelte()],
    resolve: full
      ? { alias: [{ find: /^(.*)\/staff-loader\.svelte\.js$/, replacement: '$1/staff-loader-static.svelte.js' }] }
      : {},
    build: full
      ? {
          lib: { entry: 'src/main.js', formats: ['iife'], name: 'LobbyApp', fileName: () => 'app.js', cssFileName: 'app' },
          outDir: 'dist-full',
          emptyOutDir: true,
          minify: true,
          sourcemap: false,
        }
      : {
          outDir: 'dist',
          emptyOutDir: true,
          minify: true,
          sourcemap: false,
          manifest: true,
          modulePreload: { polyfill: false },
          rollupOptions: {
            input: 'src/main.js',
            output: {
              format: 'es',
              entryFileNames: 'assets/app-[hash].js',
              chunkFileNames: 'assets/[name]-[hash].js',
              assetFileNames: 'assets/[name]-[hash][extname]',
            },
          },
        },
  };
});
