/**
 * ModalShell — 模态覆盖层基础封装（F-I8 / M-DSK-A11Y-003/006）。
 *
 * 统一提供：
 *  - role="dialog" + aria-modal="true"（读屏识别模态层级）
 *  - Esc 关闭
 *  - 初始聚焦面板 + Tab 焦点圈定（焦点陷阱）
 *  - 点击遮罩关闭
 *
 * 用法：各覆盖层（SettingsPanel/McpView/KnowledgeView/SkillsView/TemplatesView）
 * 将原 `fixed inset-0` 根 div 替换为本组件，面板内容作为 children 传入
 * （面板自带宽度/高度类）。
 */

import React, { useEffect, useRef, useState } from 'react'

// 对话框挂载序列号：Esc/遮罩只由「最后挂载（最顶层）」的实例响应。
// 避免嵌套模态（如 ConfirmDialog 叠在业务模态之上）时按 Esc 同时关掉多层
// ——旧实现两个 document keydown 监听器都响应，先注册的外层先触发（B-2）。
let modalSeq = 0

interface ModalShellProps {
  onClose: () => void
  /** 对话框可访问名称（读屏播报）。 */
  ariaLabel: string
  children: React.ReactNode
}

export const ModalShell: React.FC<ModalShellProps> = ({ onClose, ariaLabel, children }) => {
  const panelRef = useRef<HTMLDivElement>(null)
  // 惰性初始化：挂载序号越大越晚挂载（越顶层）。useState 初始化只执行一次。
  const [seq] = useState(() => ++modalSeq)

  useEffect(() => {
    // 初始聚焦面板（触发焦点陷阱的同时让读屏进入对话框）
    panelRef.current?.focus()

    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        // B-2：非最顶层实例不响应——嵌套时 Esc 只关最上层的对话框，
        // 用户需再按一次才关下层（一层层收，不静默丢决策）。
        if (seq !== modalSeq) return
        e.preventDefault()
        onClose()
        return
      }
      // Tab 焦点圈定：在面板内循环，不逃逸到背景
      if (e.key === 'Tab') {
        const panel = panelRef.current
        if (!panel) return
        const focusables = panel.querySelectorAll<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
        )
        if (focusables.length === 0) return
        const first = focusables[0]
        const last = focusables[focusables.length - 1]
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault()
          last.focus()
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault()
          first.focus()
        }
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose, seq])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/20 backdrop-blur-sm"
      onMouseDown={(e) => {
        // 点击遮罩（非面板）关闭；嵌套时最顶层先捕获（子面板在父之上），
        // 父层遮罩点击由最顶层拦截，不会穿透关闭下层（B-2）。
        if (e.target === e.currentTarget && seq === modalSeq) onClose()
      }}
    >
      <div
        ref={panelRef}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-label={ariaLabel}
        className="outline-none"
      >
        {children}
      </div>
    </div>
  )
}
