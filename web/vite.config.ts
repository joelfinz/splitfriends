import { defineConfig, type Plugin } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { writeFileSync, mkdirSync } from 'node:fs';
import { execSync } from 'node:child_process';

// Keeps web/dist/.gitkeep present after every build so Go's `//go:embed all:dist` never fails.
function keepDist(): Plugin {
  return {
    name: 'keep-dist-gitkeep',
    closeBundle() {
      mkdirSync('dist', { recursive: true });
      writeFileSync('dist/.gitkeep', '');
    },
  };
}

function version(): string {
  try {
    return execSync('git rev-parse --short HEAD', { stdio: ['ignore', 'pipe', 'ignore'] }).toString().trim();
  } catch {
    return process.env.APP_VERSION ?? 'dev';
  }
}

export default defineConfig({
  plugins: [tailwindcss(), svelte(), keepDist()],
  define: {
    __APP_VERSION__: JSON.stringify(version()),
    __BUILD_TIME__: JSON.stringify(new Date().toISOString()),
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'es2022',
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
    },
  },
});
