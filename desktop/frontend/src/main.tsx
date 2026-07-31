import React from 'react'
import ReactDOM from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MotionConfig } from 'framer-motion'
import App from './app/App'
import './styles/globals.css'

// 平台检测（Windows 适配，M-DSK-PKG-002 前端层）：
// 在 <html> 上设置 data-platform，供 globals.css 的平台覆盖块（标题栏高度/
// 字体栈/滚动条）使用。WKWebView 的 UA 恒含 "Macintosh"，mac 为默认分支。
const platformUA = navigator.userAgent.toLowerCase()
document.documentElement.dataset.platform = platformUA.includes('win')
  ? 'win32'
  : platformUA.includes('linux')
    ? 'linux'
    : 'mac'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 2,
      staleTime: 30_000,
    },
  },
})

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <QueryClientProvider client={queryClient}>
      {/* reducedMotion="user"：跟随系统「减弱动态效果」设置，
          framer-motion 动画自动降级为无位移淡入淡出 */}
      <MotionConfig reducedMotion="user">
        <App />
      </MotionConfig>
    </QueryClientProvider>
  </React.StrictMode>,
)
