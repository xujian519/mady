/**
 * DecisionSurface — 决策面板底部栏。
 *
 * 根据当前状态优先级切换显示内容，实现 Reasonix 风格决策模式：
 *   Tool Approval > Ask Question > Clear Context > Composer
 *
 * Composer 始终挂载（草稿不丢失），非激活时仅视觉隐藏。
 */

import React from 'react'
import { useChatStore } from '@/stores/chat'
import { ApprovalCard } from './ApprovalCard'
import { Composer } from './chat/Composer'

// ── 优先级排序 ────────────────────────────────────

enum SurfaceLevel {
  Approval = 4,
  Ask = 3,
  ClearContext = 2,
  Composer = 1,
}

// ── AskQuestionCard ───────────────────────────────

interface AskQuestionCardProps {
  question: string
  onSubmit: (answer: string) => void
  onCancel: () => void
}

const AskQuestionCard: React.FC<AskQuestionCardProps> = ({ question, onSubmit, onCancel }) => {
  const [answer, setAnswer] = React.useState('')

  return (
    <div className="border-t border-mady-separator bg-mady-bg-primary px-4 py-3">
      <div className="max-w-3xl mx-auto">
        <div className="rounded-xl border border-mady-accent/30 bg-mady-accent/5 px-4 py-3">
          <p className="text-mady-body font-medium text-mady-text-primary mb-2">{question}</p>
          <textarea
            value={answer}
            onChange={(e) => setAnswer(e.target.value)}
            placeholder="输入回答…"
            rows={2}
            className="w-full rounded-lg px-3 py-2 bg-mady-bg-primary border border-mady-border text-mady-body text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent resize-none mb-2"
          />
          <div className="flex gap-2">
            <button
              onClick={() => onSubmit(answer)}
              disabled={!answer.trim()}
              className="px-4 py-1.5 rounded-lg bg-mady-accent text-white text-mady-ui hover:bg-mady-accent-hover transition-colors disabled:opacity-50"
            >
              提交
            </button>
            <button
              onClick={onCancel}
              className="px-4 py-1.5 rounded-lg bg-mady-bg-secondary text-mady-text-secondary text-mady-ui hover:text-mady-text-primary transition-colors"
            >
              取消
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

// ── ClearContextCard ──────────────────────────────

interface ClearContextCardProps {
  onConfirm: () => void
  onDismiss: () => void
}

const ClearContextCard: React.FC<ClearContextCardProps> = ({ onConfirm, onDismiss }) => {
  return (
    <div className="border-t border-mady-separator bg-mady-bg-primary px-4 py-3">
      <div className="max-w-3xl mx-auto">
        <div className="rounded-xl border border-mady-warning/30 bg-mady-warning/5 px-4 py-3">
          <p className="text-mady-body text-mady-text-primary mb-3">确认清空当前上下文？这将移除所有消息历史。</p>
          <div className="flex gap-2">
            <button
              onClick={onConfirm}
              className="px-4 py-1.5 rounded-lg bg-mady-danger text-white text-mady-ui hover:opacity-90 transition-opacity"
            >
              清空
            </button>
            <button
              onClick={onDismiss}
              className="px-4 py-1.5 rounded-lg bg-mady-bg-secondary text-mady-text-secondary text-mady-ui hover:text-mady-text-primary transition-colors"
            >
              取消
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

// ── 主组件 ────────────────────────────────────────

export const DecisionSurface: React.FC = () => {
  const approvalPrompt = useChatStore((s) => s.approvalPrompt)

  // 计算当前活跃 Surface 优先级
  const activeLevel = (() => {
    if (approvalPrompt) return SurfaceLevel.Approval
    // Ask 和 ClearContext 暂未实现 store 状态，后续扩展
    return SurfaceLevel.Composer
  })()

  const isComposerVisible = activeLevel <= SurfaceLevel.Composer

  return (
    <>
      {/* 决策面板层 */}
      {approvalPrompt && <ApprovalCard prompt={approvalPrompt} />}

      {/* AskQuestion 占位（未来扩展） */}
      {/* {askQuestion && <AskQuestionCard ... />} */}

      {/* ClearContext 占位（未来扩展） */}
      {/* {clearContext && <ClearContextCard ... />} */}

      {/* Composer：始终挂载，非活跃时仅视觉隐藏 */}
      <div className={isComposerVisible ? '' : 'hidden'}>
        <Composer />
      </div>
    </>
  )
}

export { AskQuestionCard, ClearContextCard }
