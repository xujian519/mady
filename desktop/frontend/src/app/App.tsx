/**
 * App — Mady 桌面端根组件。
 *
 * 架构：
 *   ThemeProvider
 *     └── ChatView（三栏布局）
 *     └── StatusBar（由 ChatView 内部渲染）
 *
 * 启动时自动订阅 AGUI 事件流。
 *
 * 测试接口门控：
 *   生产构建中可通过 VITE_ENABLE_TEST_API=true 环境变量启用，
 *   供 Playwright/Cypress E2E 测试使用。
 */

import { useEffect } from 'react'
import { useChatStore } from '@/stores/chat'
import { subscribeAguiEvents } from '@/agui-bridge/client'
import { useA2UIStore } from '@/a2ui-renderer/a2ui-store'
import { ThemeProvider } from '@/theme/provider'
import { ChatView } from '@/components/ChatView'

/** 是否启用测试 API（__mady 全局接口）。生产构建默认为 false。 */
const ENABLE_TEST_API = import.meta.env.VITE_ENABLE_TEST_API === 'true'

function App() {
  useEffect(() => {
    // 订阅 AGUI 事件流
    const unsubscribe = subscribeAguiEvents()

    // 标记就绪
    useChatStore.setState({ ready: true })

    return () => {
      unsubscribe()
    }
  }, [])

  // 暴露测试接口
  useEffect(() => {
    if (ENABLE_TEST_API) {
      const a2ui = useA2UIStore.getState()
      ;(window as any).__mady = {
        a2ui: {
          applyEnvelope: a2ui.applyEnvelope,
          getSurface: a2ui.getSurface,
        },
      }
    }
    return () => {
      if ((window as any).__mady) {
        delete (window as any).__mady
      }
    }
  }, [])

  return (
    <ThemeProvider>
      <ChatView />
    </ThemeProvider>
  )
}

export default App
