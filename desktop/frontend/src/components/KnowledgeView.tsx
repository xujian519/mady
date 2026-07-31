/**
 * KnowledgeView — 知识库管理面板（覆盖层）。
 *
 * 功能：
 * 1. 知识库状态概览（文档数/索引大小/更新时间/源目录）
 * 2. 重新索引按钮 + 进度指示
 * 3. 索引范围选择（专利法/审查指南/判例/自定义）
 * 数据从后端 knowledge 子系统加载。
 */

import React, { useState } from 'react'
import { Database, Folder, X, Loader } from 'lucide-react'
import { useKnowledgeStatus } from '@/queries/knowledge'
import { ModalShell } from './ModalShell'

interface KnowledgeViewProps {
  onClose: () => void
}

/** 索引范围选项。 */
const SCOPE_OPTIONS = [
  { key: 'patent-law', label: '专利法' },
  { key: 'exam-guide', label: '审查指南' },
  { key: 'precedent', label: '判例' },
  { key: 'custom', label: '自定义' },
] as const

export const KnowledgeView: React.FC<KnowledgeViewProps> = ({ onClose }) => {
  const knowledgeQuery = useKnowledgeStatus()
  const data = knowledgeQuery.data ?? null
  const loading = knowledgeQuery.isLoading
  const [checked, setChecked] = useState<Set<string>>(
    () => new Set(SCOPE_OPTIONS.map((o) => o.key)),
  )

  // 知识库状态由 TanStack Query 接管（useKnowledgeStatus，
  // 见 mady-desktop-standards.md M-DSK-ST-002）；加载失败静默降级为 null
  // F-I13：移除模拟「重新索引」（indexStatus/progress/timerRef/handleReindex）
  // ——后端无 reindex binding，前端自增进度是假象，状态展示改为说明文案。

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
    <ModalShell onClose={onClose} ariaLabel="知识库管理">
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

          {/* ── 索引状态（F-I13：移除无后端支撑的模拟「重新索引」，
              避免显示虚假进度；真实状态以启动时自动索引为准） ── */}
          <section>
            <h3 className="text-mady-ui font-medium text-mady-text-primary mb-3">索引状态</h3>
            <div className="rounded-lg bg-mady-bg-secondary px-3 py-2.5 text-mady-small text-mady-text-secondary">
              知识库在应用启动时自动建立索引。索引文件位于
              <code className="mx-1 px-1 py-0.5 rounded bg-mady-bg-tertiary text-mady-caption font-mono">~/.mady/knowledge</code>
              ；如需重建，请重启应用。
            </div>
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
    </ModalShell>
  )
}
