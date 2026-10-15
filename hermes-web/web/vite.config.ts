import { tanstackRouter } from '@tanstack/router-plugin/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  // The router plugin must come before react(). It generates src/routeTree.gen.ts from
  // src/routes/ and splits each route's component into its own lazily loaded chunk.
  plugins: [tanstackRouter({ target: 'react', autoCodeSplitting: true }), react()],
  server: {
    proxy: { "/api": "http://localhost:8080" },
  }
})
