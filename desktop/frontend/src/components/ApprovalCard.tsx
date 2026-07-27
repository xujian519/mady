/**
 * ApprovalCard — 审批卡片组件。
 *
 * 通过 `agui:approval-prompt` 事件触发渲染。
 * 用户可选择批准/拒绝，结果通过 SendAction 回传。
 *
 * 与 TUI `approval_card.go` 行为一致。
 */

import React, { useState } from 'react'
import type { ApprovalPrompt } from '@/stores/chat'
import { useChatStore } from '@/stores/chat'
import { emitWailsEvent } from '@/lib/wails'
import { Shield, Check, X } from 'lucide-react'

interface ApprovalCardProps {
  prompt: ApprovalPrompt
}

export const ApprovalCard: React.FC<ApprovalCardProps> = ({ prompt }) => {
  const [reason, setReason] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const handleResponse = async (approved: boolean, value?: string) => {
    setSubmitting(true)
    try {
      emitWailsEvent('agui:approval-response', {
        id: prompt.id,
        approved,
        value,
        reason: reason || undefined,
      })
    } finally {
      setSubmitting(false)
      useChatStore.getState().setApprovalPrompt(null)
    }
  }

  return (
    <div className="max-w-[75%] rounded-xl border border-mady-warning/30 bg-mady-warning/5 px-4 py-3">
      {/* 头部 */}
      <div className="flex items-center gap-2 mb-2">
        <Shield size={16} className="text-mady-warning" />
        <span className="text-mady-ui font-medium text-mady-text-primary">
          {prompt.title || '审批请求'}
        </span>
      </div>

      {/* 描述 */}
      {prompt.description && (
        <p className="text-mady-body text-mady-text-secondary mb-3">
          {prompt.description}
        </p>
      )}

      {/* 自定义选项按钮 */}
      {prompt.options && prompt.options.length > 0 && (
        <div className="flex flex-wrap gap-2 mb-3">
          {prompt.options.map((opt) => (
            <button
              key={opt.value}
              onClick={() => handleResponse(true, opt.value)}
              className="px-3 py-1 rounded-lg bg-mady-accent text-white text-mady-ui hover:bg-mady-accent-hover transition-colors"
            >
              {opt.label}
            </button>
          ))}
        </div>
      )}

      {/* 理由输入 */}
      <textarea
        value={reason}
        onChange={(e) => setReason(e.target.value)}
        placeholder="添加备注（可选）…"
        rows={2}
        className="w-full rounded-lg px-3 py-2 bg-mady-bg-primary border border-mady-border text-mady-body text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent resize-none mb-3"
      />

      {/* 操作按钮 */}
      <div className="flex gap-2">
        <button
          onClick={() => handleResponse(true)}
          disabled={submitting}
          className="flex items-center gap-1.5 px-4 py-1.5 rounded-lg bg-mady-success text-white text-mady-ui hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          <Check size={14} />
          批准
        </button>
        <button
          onClick={() => handleResponse(false)}
          disabled={submitting}
          className="flex items-center gap-1.5 px-4 py-1.5 rounded-lg bg-mady-danger text-white text-mady-ui hover:opacity-90 transition-opacity disabled:opacity-50"
        >
          <X size={14} />
          拒绝
        </button>
      </div>
    </div>
  )
}
