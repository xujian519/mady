/**
 * KnowledgeView — 知识库管理面板（覆盖层）。
 *
 * 功能：
 * 1. 知识库状态概览（文档数/索引大小/更新时间/源目录）
 * 2. 重新索引按钮 + 进度指示
 * 3. 索引范围选择（专利法/审查指南/判例/自定义）
 * 数据从后端 knowledge 子系统加载。
 */

import React, { useState, useCallback, useEffect, useRef } from 'react'
import { Database, RefreshCw, CheckCircle2, Folder, X, Loader } from 'lucide-react'
import { getKnowledgeStatus, type KnowledgeStatus } from '@/lib/backend'

interface KnowledgeViewProps {
  onClose: () => void
}

type IndexStatus = 'idle' | 'indexing' | 'done'

/** 索引范围选项。 */
const SCOPE_OPTIONS = [
  { key: 'patent-law', label: '专利法' },
  { key: 'exam-guide', label: '审查指南' },
  { key: 'precedent', label: '判例' },
  { key: 'custom', label: '自定义' },
] as const

export const KnowledgeView: React.FC<KnowledgeViewProps> = ({ onClose }) => {
  const [indexStatus, setIndexStatus] = useState<IndexStatus>('idle')
  const [progress, setProgress] = useState(0)
  const [data, setData] = useState<KnowledgeStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [checked, setChecked] = useState<Set<string>>(
    () => new Set(SCOPE_OPTIONS.map((o) => o.key)),
  )
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // 从后端加载知识库状态
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    getKnowledgeStatus()
      .then((ks) => {
        if (!cancelled) {
          setData(ks)
          setLoading(false)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setData(null)
          setLoading(false)
        }
      })
    return () => { cancelled = true }
  }, [])

  // 清理定时器
  useEffect(() => {
    return () => {
      if (timerRef.current) clearInterval(timerRef.current)
    }
  }, [])

  const handleReindex = useCallback(() => {
    if (indexStatus === 'indexing') return

    setIndexStatus('indexing')
    setProgress(0)

    timerRef.current = setInterval(() => {
      setProgress((prev) => {
        const next = Math.min(prev + 4, 100)
        if (next >= 100) {
          if (timerRef.current) clearInterval(timerRef.current)
          timerRef.current = null
          setIndexStatus('done')
        }
        return next
      })
    }, 120) // 3s / (100/4) = ~120ms per tick
  }, [indexStatus])

  const toggleScope = (key: string) => {
    setChecked((prev) => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/20 backdrop-blur-sm">
      <div className="w-[480px] max-h-[80vh] bg-mady-bg-primary rounded-2xl border border-mady-separator shadow-xl overflow-y-auto">
        {/* 头部 */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-mady-separator">
          <div className="flex items-center gap-2">
            <Database size={16} className="text-mady-accent" />
            <h2 className="text-mady-heading font-semibold">知识库管理</h2>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg hover:bg-mady-bg-secondary text-mady-text-secondary"
          >
            <X size={16} />
          </button>
        </div>

        <div className="p-5 space-y-6">
          {/* ── 知识库概览 ───────────────────────── */}
          <section>
            <h3 className="text-mady-ui font-medium text-mady-text-primary mb-3">知识库概览</h3>
            {loading ? (
              <div className="flex items-center justify-center py-8 text-mady-text-tertiary">
                <Loader size={16} className="mr-2 animate-spin" />
                <span className="text-mady-ui">加载中...</span>
              </div>
            ) : (
              <>
                <div className="grid grid-cols-3 gap-2">
                  {/* 文档数 */}
                  <div className="bg-mady-bg-secondary rounded-lg p-3">
                    <p className="text-mady-caption text-mady-text-tertiary mb-0.5">文档</p>
                    <p className="text-mady-body font-semibold text-mady-text-primary">
                      {data ? `${data.docCount.toLocaleString()} 份` : '未知'}
                    </p>
                  </div>

                  {/* 索引大小 */}
                  <div className="bg-mady-bg-secondary rounded-lg p-3">
                    <p className="text-mady-caption text-mady-text-tertiary mb-0.5">索引大小</p>
                    <p className="text-mady-body font-semibold text-mady-text-primary">
                      {data ? `${data.indexSizeMB} MB` : '未知'}
                    </p>
                  </div>

                  {/* 最后更新 */}
                  <div className="bg-mady-bg-secondary rounded-lg p-3">
                    <p className="text-mady-caption text-mady-text-tertiary mb-0.5">最后更新</p>
                    <p className="text-mady-body font-semibold text-mady-text-primary">
                      {data?.lastUpdated || '无'}
                    </p>
                  </div>
                </div>

                {/* 源目录 */}
                {data && data.sourceDirs.length > 0 && (
                  <div className="mt-2 bg-mady-bg-secondary rounded-lg p-3 space-y-1.5">
                    <div className="flex items-center gap-1.5 text-mady-caption text-mady-text-tertiary">
                      <Folder size={11} />
                      <span>源目录</span>
                    </div>
                    {data.sourceDirs.map((dir) => (
                      <div key={dir} className="flex items-center gap-2 text-mady-small text-mady-text-secondary font-mono">
                        <span className="w-1.5 h-1.5 rounded-full bg-mady-accent-soft shrink-0" />
                        {dir}
                      </div>
                    ))}
                  </div>
                )}
              </>
            )}
          </section>

          {/* ── 重新索引 ─────────────────────────── */}
          <section>
            <div className="flex items-center justify-between mb-2">
              <h3 className="text-mady-ui font-medium text-mady-text-primary">重新索引</h3>
              <span
                className={`
                  text-mady-caption transition-colors
                  ${indexStatus === 'done'
                    ? 'text-mady-success'
                    : indexStatus === 'indexing'
                      ? 'text-mady-accent'
                      : 'text-mady-text-tertiary'
                  }
                `}
              >
                {indexStatus === 'idle' && '就绪'}
                {indexStatus === 'indexing' && '索引中...'}
                {indexStatus === 'done' && '索引完成 ✓'}
              </span>
            </div>

            {/* 进度条 */}
            <div className="w-full h-2 bg-mady-bg-secondary rounded-full overflow-hidden">
              <div
                className={`
                  h-full rounded-full transition-all duration-150 ease-out
                  ${indexStatus === 'done'
                    ? 'bg-mady-success'
                    : 'bg-mady-accent'
                  }
                `}
                style={{ width: `${progress}%` }}
              />
            </div>

            {/* 进度数值 */}
            {indexStatus !== 'idle' && (
              <p className="text-mady-caption text-mady-text-tertiary mt-1 text-right">{progress}%</p>
            )}

            {/* 按钮 */}
            <button
              onClick={handleReindex}
              disabled={indexStatus === 'indexing'}
              className={`
                mt-3 w-full flex items-center justify-center gap-2 px-3 py-2 rounded-lg
                text-mady-ui font-medium transition-colors
                ${indexStatus === 'done'
                  ? 'bg-mady-success/10 text-mady-success cursor-default'
                  : indexStatus === 'indexing'
                    ? 'bg-mady-accent/10 text-mady-accent cursor-not-allowed'
                    : 'bg-mady-accent text-white hover:bg-mady-accent-hover'
                }
              `}
            >
              {indexStatus === 'done' ? (
                <>
                  <CheckCircle2 size={14} />
                  已是最新
                </>
              ) : indexStatus === 'indexing' ? (
                <>
                  <RefreshCw size={14} className="animate-spin" />
                  索引中...
                </>
              ) : (
                <>
                  <RefreshCw size={14} />
                  开始索引
                </>
              )}
            </button>
          </section>

          {/* ── 索引范围 ─────────────────────────── */}
          <section>
            <h3 className="text-mady-ui font-medium text-mady-text-primary mb-3">索引范围</h3>
            <div className="space-y-2">
              {SCOPE_OPTIONS.map((opt) => (
                <label
                  key={opt.key}
                  className="flex items-center gap-3 px-3 py-2.5 rounded-lg bg-mady-bg-secondary hover:bg-mady-bg-tertiary cursor-pointer transition-colors"
                >
                  <input
                    type="checkbox"
                    checked={checked.has(opt.key)}
                    onChange={() => toggleScope(opt.key)}
                    className="w-4 h-4 rounded border-mady-border text-mady-accent accent-mady-accent focus:ring-1 focus:ring-mady-accent focus:ring-offset-0"
                  />
                  <span className="text-mady-body text-mady-text-primary">{opt.label}</span>
                </label>
              ))}
            </div>

            {/* 已选数量 */}
            <p className="text-mady-caption text-mady-text-tertiary mt-2">
              已选 {checked.size} / {SCOPE_OPTIONS.length} 个范围
            </p>
          </section>
        </div>
      </div>
    </div>
  )
}
