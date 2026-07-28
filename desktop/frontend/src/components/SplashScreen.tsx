/**
 * SplashScreen — 桌面端启动加载层。
 *
 * 应用后端初始化期间显示，通过 mady:init-progress 事件接收实时进度文案，
 * 收到 mady:init-done 后淡出并展示主界面。
 */

import { useEffect, useState, useCallback } from 'react'
import { listenToWailsEvent } from '@/lib/wails'
import { useChatStore } from '@/stores/chat'

export function SplashScreen() {
  const [visible, setVisible] = useState(true)
  const [progress, setProgress] = useState('正在初始化引擎...')
  const [fadingOut, setFadingOut] = useState(false)

  const handleDone = useCallback(() => {
    setProgress('就绪')
    setFadingOut(true)
    // 淡出动画后隐藏
    setTimeout(() => {
      setVisible(false)
      useChatStore.setState({ ready: true })
    }, 600)
  }, [])

  useEffect(() => {
    // 订阅初始化进度
    const unsubProgress = listenToWailsEvent('mady:init-progress', (msg: string) => {
      if (typeof msg === 'string') setProgress(msg)
    })

    // 订阅初始化完成
    const unsubDone = listenToWailsEvent('mady:init-done', handleDone)

    // 兜底：5 秒后自动关闭（防止事件丢失导致永远卡在加载界面）
    const timeout = setTimeout(() => {
      if (!fadingOut) handleDone()
    }, 15000)

    return () => {
      unsubProgress()
      unsubDone()
      clearTimeout(timeout)
    }
  }, [handleDone, fadingOut])

  // 兜底：如果后端已快速就绪（store.ready 为 true），直接隐藏
  useEffect(() => {
    if (useChatStore.getState().ready) {
      setVisible(false)
    }
  }, [])

  if (!visible) return null

  return (
    <div
      className={`
        fixed inset-0 z-50 flex flex-col items-center justify-center
        bg-white dark:bg-gray-950
        transition-opacity duration-500 ease-in-out
        ${fadingOut ? 'opacity-0' : 'opacity-100'}
      `}
    >
      {/* Logo / 品牌标识 */}
      <div className="flex items-center gap-3 mb-8">
        <div className="w-10 h-10 rounded-xl bg-indigo-600 flex items-center justify-center">
          <span className="text-white text-lg font-bold">M</span>
        </div>
        <h1 className="text-3xl font-semibold text-gray-900 dark:text-gray-100 tracking-tight">
          Mady
        </h1>
      </div>

      {/* 进度文案 */}
      <p className="text-sm text-gray-500 dark:text-gray-400 mb-6 h-5 text-center transition-all duration-300">
        {progress}
      </p>

      {/* 加载指示器 */}
      <div className="flex items-center gap-1.5">
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            className="w-2 h-2 rounded-full bg-indigo-500 dark:bg-indigo-400 animate-bounce"
            style={{ animationDelay: `${i * 0.15}s`, animationDuration: '0.8s' }}
          />
        ))}
      </div>
    </div>
  )
}
