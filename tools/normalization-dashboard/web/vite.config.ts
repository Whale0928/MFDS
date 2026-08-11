import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  // allowedHosts가 없으면 quick tunnel 주소로 들어온 요청을 Vite가 Blocked request로 거절한다.
  // strictPort가 없으면 5173 선점 시 조용히 5174로 밀려 헬스체크와 터널이 엉뚱한 서버를 가리킨다.
  server: { port: 5173, strictPort: true, proxy: { '/api': 'http://127.0.0.1:8787' }, allowedHosts: ['.trycloudflare.com'] },
  test: { environment: 'node', include: ['src/**/*.test.{ts,tsx}'] },
})
