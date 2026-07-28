/**
 * App — Mady 桌面端根组件。
 *
 * 架构：
 *   SplashScreen（加载层，启动时显示）
 *   ThemeProvider
 *     └── ChatView（主界面，初始化完成后显示）
 *
 * 启动时自动订阅 AGUI 事件流和 mady:init-* 初始化事件。
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
import { useTheme } from '@/theme/tokens'
import type { ThemePackId } from '@/theme/tokens'
import { ChatView } from '@/components/ChatView'
import { SplashScreen } from '@/components/SplashScreen'
import { saveWindowState } from '@/lib/backend'

/** 是否启用测试 API（__mady 全局接口）。生产构建默认为 false。 */
const ENABLE_TEST_API = import.meta.env.VITE_ENABLE_TEST_API === 'true'

function ThemeEventListener({ children }: { children: React.ReactNode }) {
  const { setThemePack } = useTheme()

  useEffect(() => {
    const handleThemePack = (e: Event) => {
      const detail = (e as CustomEvent).detail
      // 尝试匹配内置主题包 ID
      const validPacks: ThemePackId[] = ['professional', 'focus-blue', 'paper-warm', 'slate']
      const match = validPacks.find((p) => detail === p || detail === p.replace('-', ' '))
      if (match) {
        setThemePack(match)
      }
    }
    window.addEventListener('mady:set-theme-pack', handleThemePack)
    return () => window.removeEventListener('mady:set-theme-pack', handleThemePack)
  }, [setThemePack])

  return <>{children}</>
}

function App() {
  const ready = useChatStore((s) => s.ready)

  // 窗口关闭前保存几何信息
  useEffect(() => {
    const handleBeforeUnload = () => {
      saveWindowState(window.innerWidth, window.innerHeight).catch(() => {})
    }
    window.addEventListener('beforeunload', handleBeforeUnload)
    return () => window.removeEventListener('beforeunload', handleBeforeUnload)
  }, [])

  useEffect(() => {
    // 订阅 AGUI 事件流
    const unsubscribe = subscribeAguiEvents()

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
    <>
      {!ready && <SplashScreen />}
      <ThemeProvider>
        <ThemeEventListener>
          <ChatView />
        </ThemeEventListener>
      </ThemeProvider>
    </>
  )
}

export default App
