/**
 * ConclusionCard — 结论卡片。
 *
 * 展示 Agent 的结构化分析与结论。
 * 包含标题、主体（Markdown）、置信度、来源引用。
 *
 * 置信度标注遵循 tone-style-guide.md：
 * - 不使用绝对化表述
 * - 结论性表述附带置信度标注
 */

import React from 'react'
import { MarkdownRenderer } from './MarkdownRenderer'
import { ConfidenceBar } from './ConfidenceBar'
import { Lightbulb, ExternalLink } from 'lucide-react'

export interface ConclusionData {
  /** 结论标题。 */
  title: string
  /** 结论主体内容（Markdown）。 */
  content: string
  /** 置信度 (0-1)。 */
  confidence?: number
  /** 来源引用。 */
  sources?: ConclusionSource[]
}

export interface ConclusionSource {
  label: string
  url?: string
}

interface ConclusionCardProps {
  data: ConclusionData
}

export const ConclusionCard: React.FC<ConclusionCardProps> = ({ data }) => {
  return (
    <div className="max-w-[75%] rounded-xl border border-mady-accent/20 bg-mady-accent-soft/30 px-4 py-3">
      {/* 头部 */}
      <div className="flex items-center gap-2 mb-2">
        <Lightbulb size={16} className="text-mady-accent" />
        <span className="text-mady-ui font-medium text-mady-text-primary">
          {data.title || '分析结论'}
        </span>
      </div>

      {/* 主体 */}
      <div className="text-mady-body text-mady-text-primary mb-3">
        <MarkdownRenderer content={data.content} />
      </div>

      {/* 置信度 */}
      {data.confidence !== undefined && (
        <div className="mb-3">
          <ConfidenceBar
            level={data.confidence}
            label="可信度评估"
            description="基于现有证据的综合评估结果"
          />
        </div>
      )}

      {/* 来源 */}
      {data.sources && data.sources.length > 0 && (
        <div className="border-t border-mady-separator pt-2 mt-2">
          <span className="text-mady-caption text-mady-text-tertiary block mb-1">参考来源</span>
          <ul className="space-y-0.5">
            {data.sources.map((src, i) => (
              <li key={i}>
                {src.url ? (
                  <a
                    href={src.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 text-mady-caption text-mady-text-link hover:underline"
                  >
                    <ExternalLink size={10} />
                    {src.label}
                  </a>
                ) : (
                  <span className="text-mady-caption text-mady-text-secondary">{src.label}</span>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
