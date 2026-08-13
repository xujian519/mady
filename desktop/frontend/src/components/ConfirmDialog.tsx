/**
 * ConfirmDialog — 轻量确认/提示对话框（M-DSK-IX-007：确认对话框仅用于破坏性操作）。
 *
 * WKWebView 不支持同步 window.confirm / window.alert（静默失败，confirm 恒 false、
 * alert 不显示），所有破坏性操作确认与错误提示统一走本组件
 * （2026-08-04 审阅修复 M-11）。
 *
 * 用法：open 控制显隐；alertOnly=true 时仅「确定」按钮（提示模式）；
 * danger=true 时确认按钮使用危险色。
 */

import React from 'react'
import { ModalShell } from './ModalShell'

interface ConfirmDialogProps {
  open: boolean
  title: string
  message: string
  /** 提示模式（仅确定按钮，无取消）。默认 confirm 模式。 */
  alertOnly?: boolean
  confirmLabel?: string
  /** 确认按钮使用危险色（删除等破坏性操作）。 */
  danger?: boolean
  onConfirm?: () => void
  onCancel?: () => void
}

export const ConfirmDialog: React.FC<ConfirmDialogProps> = ({
  open,
  title,
  message,
  alertOnly = false,
  confirmLabel = '确定',
  danger = false,
  onConfirm,
  onCancel,
}) => {
  if (!open) return null
  return (
    <ModalShell onClose={() => onCancel?.()} ariaLabel={title}>
      <div className="w-[320px] rounded-xl border border-mady-border bg-mady-bg-primary shadow-mady-modal">
        <div className="px-4 pt-3.5 pb-1">
          <h2 className="text-mady-ui font-medium text-mady-text-primary">{title}</h2>
          <p className="mt-1.5 text-mady-small text-mady-text-secondary leading-relaxed whitespace-pre-line">
            {message}
          </p>
        </div>
        <div className="flex justify-end gap-2 px-4 py-3">
          {!alertOnly && (
            <button
              type="button"
              onClick={onCancel}
              className="px-3 py-1.5 rounded-lg text-mady-small text-mady-text-secondary hover:bg-mady-bg-hover transition-colors"
            >
              取消
            </button>
          )}
          <button
            type="button"
            autoFocus
            onClick={onConfirm ?? onCancel}
            className={`
              px-3 py-1.5 rounded-lg text-mady-small font-medium transition-colors
              ${danger
                ? 'bg-mady-danger/90 text-white hover:bg-mady-danger'
                : 'bg-mady-accent text-white hover:opacity-90'}
            `}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </ModalShell>
  )
}
