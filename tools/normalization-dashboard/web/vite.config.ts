import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  // allowedHosts가 없으면 quick tunnel 주소로 들어온 요청을 Vite가 Blocked request로 거절한다.
  server: { port: 5173, proxy: { '/api': 'http://127.0.0.1:8787' }, allowedHosts: ['.trycloudflare.com'] },
  test: { environment: 'node', include: ['src/**/*.test.ts'] },
})
