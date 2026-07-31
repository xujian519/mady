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

import React, { useEffect, useRef } from 'react'

interface ModalShellProps {
  onClose: () => void
  /** 对话框可访问名称（读屏播报）。 */
  ariaLabel: string
  children: React.ReactNode
}

export const ModalShell: React.FC<ModalShellProps> = ({ onClose, ariaLabel, children }) => {
  const panelRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    // 初始聚焦面板（触发焦点陷阱的同时让读屏进入对话框）
    panelRef.current?.focus()

    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
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
  }, [onClose])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/20 backdrop-blur-sm"
      onMouseDown={(e) => {
        // 点击遮罩（非面板）关闭
        if (e.target === e.currentTarget) onClose()
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
