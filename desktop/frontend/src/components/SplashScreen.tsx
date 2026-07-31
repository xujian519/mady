/**
 * SplashScreen — 桌面端启动加载层。
 *
 * 应用后端初始化期间显示，通过 mady:init-progress 事件接收实时进度文案，
 * 收到 mady:init-done 后淡出并展示主界面。
 */

import { useEffect, useState, useCallback, useRef } from 'react'
import { listenToWailsEvent } from '@/lib/wails'
import { useChatStore } from '@/stores/chat'
import { AlertCircle } from 'lucide-react'

export function SplashScreen() {
  const [visible, setVisible] = useState(true)
  const [progress, setProgress] = useState('正在初始化引擎...')
  const [fadingOut, setFadingOut] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const doneRef = useRef(false)
  const fadingOutRef = useRef(false)
  const errorRef = useRef(false)

  const handleDone = useCallback(() => {
    if (doneRef.current) return // 去重守卫：即使收到重复事件也仅触发一次
    doneRef.current = true
    setProgress('就绪')
    setFadingOut(true)
    fadingOutRef.current = true
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

    // 订阅初始化失败（W4-T3 观察项闭环）：停住加载层并展示错误，
    // 不进入主界面；兜底计时器同时失效，避免错误态被强行跳过。
    const unsubError = listenToWailsEvent('mady:init-error', (msg: string) => {
      errorRef.current = true
      setError(typeof msg === 'string' && msg !== '' ? msg : '引擎初始化失败')
    })

    // 兜底：15 秒后自动关闭（防止事件丢失导致永远卡在加载界面）
    // 使用 fadingOutRef 避免 fadingOut 闭包过期；初始化失败时不自动关闭
    const timeout = setTimeout(() => {
      if (!errorRef.current && !fadingOutRef.current) handleDone()
    }, 15000)

    return () => {
      unsubProgress()
      unsubDone()
      unsubError()
      clearTimeout(timeout)
    }
  }, [handleDone])

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
        bg-mady-bg-primary
        transition-opacity duration-500 ease-in-out
        ${fadingOut ? 'opacity-0' : 'opacity-100'}
      `}
    >
      {/* 背景光晕装饰 */}
      <div
        className="absolute w-64 h-64 rounded-full opacity-20 blur-3xl pointer-events-none"
        style={{ background: 'var(--color-mady-accent)' }}
      />

      {/* Logo / 品牌标识 */}
      <div className="flex items-center gap-3 mb-8 relative z-10">
        <div
          className="w-12 h-12 rounded-2xl flex items-center justify-center shadow-lg"
          style={{
            background: 'linear-gradient(135deg, var(--color-mady-accent) 0%, var(--color-mady-accent-tertiary) 100%)',
            boxShadow: '0 8px 24px var(--color-mady-accent-glow)',
          }}
        >
          <span className="text-white text-xl font-bold tracking-tight">M</span>
        </div>
        <h1 className="text-3xl font-semibold text-mady-text-primary tracking-tight">
          Mady
        </h1>
      </div>

      {/* 进度文案 / 失败状态 */}
      {error ? (
        <div className="flex flex-col items-center mb-6 relative z-10 max-w-xs">
          <AlertCircle size={28} className="text-mady-danger mb-3" />
          <p className="text-sm text-mady-danger mb-1 text-center">引擎初始化失败</p>
          <p className="text-xs text-mady-text-secondary text-center break-all">{error}</p>
          <p className="text-xs text-mady-text-tertiary mt-3 text-center">
            请检查 AI 服务配置后重启应用
          </p>
        </div>
      ) : (
        <>
          <p className="text-sm text-mady-text-secondary mb-6 h-5 text-center transition-all duration-300 relative z-10">
            {progress}
          </p>

          {/* 加载指示器 — 波形脉冲 */}
          <div className="flex items-center gap-1.5 relative z-10">
            {[0, 1, 2].map((i) => (
              <div
                key={i}
                className="w-1.5 h-1.5 rounded-full animate-bounce"
                style={{
                  backgroundColor: 'var(--color-mady-accent)',
                  animationDelay: `${i * 0.15}s`,
                  animationDuration: '0.8s',
                }}
              />
            ))}
          </div>
        </>
      )}
    </div>
  )
}
