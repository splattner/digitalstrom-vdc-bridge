import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [tailwindcss(), react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8090',
        changeOrigin: true,
        ws: true,
      },
    },
  },
  build: {
    outDir: '../pkg/httpapi/webdist',
    emptyOutDir: true,
    // Use relative asset paths so the app works under any URL prefix,
    // including the Home Assistant ingress path (/api/hassio_ingress/<token>/).
    assetsDir: 'assets',
  },
  // Relative base so generated <script>/<link> tags use ./assets/... instead
  // of /assets/..., which would break behind the HA ingress proxy.
  base: './',
})
