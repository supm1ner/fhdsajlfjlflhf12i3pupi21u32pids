/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// Same-origin contract: in dev, Vite proxies the backend prefixes so the SPA and
// the API share an origin (required for SameSite=Lax cookies + double-submit CSRF).
// In prod the same prefixes are reverse-proxied by nginx (see frontend/nginx.conf).
// Build contract §7 + §6: backend listens on :8080, frontend dev server on :3000.
const BACKEND_TARGET = process.env.VITE_BACKEND_URL ?? 'http://localhost:8080';

const proxyPrefixes = ['/api', '/oauth', '/healthz', '/swagger'];

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    strictPort: true,
    proxy: Object.fromEntries(
      proxyPrefixes.map((prefix) => [
        prefix,
        {
          target: BACKEND_TARGET,
          changeOrigin: true,
        },
      ]),
    ),
  },
  preview: {
    port: 3000,
    strictPort: true,
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    css: true,
    include: ['src/**/*.test.{ts,tsx}'],
  },
});
