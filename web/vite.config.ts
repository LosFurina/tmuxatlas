import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    exclude: ['e2e/**', 'node_modules/**'],
    restoreMocks: true,
    clearMocks: true,
  },
  build: {
    outDir: '../pkg/server/dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks: {
          xterm: ['@xterm/xterm', '@xterm/addon-fit', '@xterm/addon-web-links', '@xterm/addon-clipboard'],
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:7654',
      '/ws': {
        target: 'ws://localhost:7654',
        ws: true,
      },
    },
  },
})
