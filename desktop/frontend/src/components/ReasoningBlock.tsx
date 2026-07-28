/**
 * ReasoningBlock — 推理折叠块（对齐设计规范第9.3.5章）。
 *
 * 规范定义：
 *   默认折叠状态，暗淡 italic 摘要文字
 *   左侧 2px 灰色边框，展开时内容追加
 *   Chevron 旋转 90deg 动画 150ms ease
 */

import React, { useState } from 'react'
import { Brain, ChevronRight } from 'lucide-react'

interface ReasoningBlockProps {
  /** 推理内容文本 */
  content: string
}

export const ReasoningBlock: React.FC<ReasoningBlockProps> = ({
  content,
}) => {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="px-4 py-1.5 max-w-[85%]">
      <div className="rounded-md bg-mady-bg-hover/30 border-l-2 border-mady-text-tertiary/30">
        {/* 折叠状态摘要行 */}
        <button
          onClick={() => setExpanded(!expanded)}
          className="w-full flex items-center gap-1.5 px-3 py-1.5 text-left cursor-pointer select-none hover:bg-mady-bg-hover/50 transition-colors duration-150"
        >
          <div
            className="transition-transform duration-150 ease-in-out"
            style={{ transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)' }}
          >
            <ChevronRight size={12} className="text-mady-text-tertiary" />
          </div>
          <Brain size={12} className="text-mady-text-tertiary shrink-0" />
          <span className="text-mady-caption italic text-mady-text-tertiary truncate flex-1">
            {content ? content.slice(0, 60) + (content.length > 60 ? '…' : '') : '推理中…'}
          </span>
        </button>

        {/* 展开推理内容 */}
        {expanded && content && (
          <div className="px-3 pb-2 text-mady-small italic leading-relaxed text-mady-text-secondary whitespace-pre-wrap border-t border-mady-border/30 mt-0 pt-2">
            {content}
          </div>
        )}
      </div>
    </div>
  )
}
