/**
 * TemplatesView — 专利模板库视图（覆盖层）。
 *
 * 分类展示各类专利撰写模板，支持搜索筛选和选择使用。
 * 模板数据从后端 doc-templates/ 目录加载。
 */

import React, { useState, useEffect } from 'react'
import { FileText, Search, X, Loader } from 'lucide-react'
import { useDocTemplates, type DocTemplateEntry } from '@/queries/templates'
import { ModalShell } from './ModalShell'

export interface TemplatesViewProps {
  /** 关闭回调。 */
  onClose: () => void
  /** 用户选择使用模板时的回调。 */
  onUseTemplate?: (template: { name: string; category: string; content: string }) => void
}

// ── 组件 ────────────────────────────────────────────

export const TemplatesView: React.FC<TemplatesViewProps> = ({
  onClose,
  onUseTemplate,
}) => {
  const [activeCategory, setActiveCategory] = useState<string>('claims')
  const [searchQuery, setSearchQuery] = useState('')
  const templatesQuery = useDocTemplates()
  const templates = templatesQuery.data ?? []
  const loading = templatesQuery.isLoading

  // 模板数据由 TanStack Query 接管（useDocTemplates，
  // 见 mady-desktop-standards.md M-DSK-ST-002）；失败静默降级为空列表

  // 从模板数据中提取所有分类
  const categories = React.useMemo(() => {
    const seen = new Set<string>()
    const result: { key: string; label: string }[] = []
    for (const t of templates) {
      if (!seen.has(t.category)) {
        seen.add(t.category)
        result.push({ key: t.category, label: t.categoryLabel || t.category })
      }
    }
    // 默认分类顺序优先
    const order = ['claims', 'specification', 'disclosure', 'oa-response', 'legal']
    result.sort((a, b) => {
      const ai = order.indexOf(a.key)
      const bi = order.indexOf(b.key)
      return (ai >= 0 ? ai : 99) - (bi >= 0 ? bi : 99)
    })
    return result
  }, [templates])

  // 如果当前 activeCategory 不在分类中，自动切换到第一个
  useEffect(() => {
    if (categories.length > 0 && !categories.find((c) => c.key === activeCategory)) {
      setActiveCategory(categories[0].key)
    }
  }, [categories, activeCategory])

  const filteredTemplates = templates.filter(
    (t) => t.category === activeCategory && (!searchQuery.trim() || t.name.toLowerCase().includes(searchQuery.toLowerCase())),
  )

  const handleUse = (template: DocTemplateEntry) => {
    onUseTemplate?.({
      name: template.name,
      category: template.category,
      content: template.content,
    })
  }

  return (
    <ModalShell onClose={onClose} ariaLabel="专利模板库">
      <div className="w-[640px] max-h-[80vh] bg-mady-bg-primary rounded-2xl border border-mady-separator shadow-xl flex flex-col overflow-hidden">
        {/* ── 头部 ── */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-mady-separator shrink-0">
          <div className="flex items-center gap-2">
            <FileText size={16} className="text-mady-accent" />
            <h2 className="text-mady-heading font-semibold">专利模板库</h2>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg hover:bg-mady-bg-secondary text-mady-text-secondary"
            aria-label="关闭"
          >
            <X size={16} />
          </button>
        </div>

        {/* ── 搜索 ── */}
        <div className="px-5 pt-4 pb-2 shrink-0">
          <div className="relative">
            <Search
              size={14}
              className="absolute left-3 top-1/2 -translate-y-1/2 text-mady-text-tertiary"
            />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="搜索模板..."
              className="w-full rounded-lg pl-9 pr-3 py-2 bg-mady-bg-secondary border border-mady-border text-mady-body text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent transition-colors"
            />
          </div>
        </div>

        {/* ── 分类标签 ── */}
        <div className="px-5 pb-3 shrink-0 overflow-x-auto">
          <div className="flex gap-1.5">
            {categories.map((cat) => (
              <button
                key={cat.key}
                onClick={() => setActiveCategory(cat.key)}
                className={[
                  'px-3 py-1.5 rounded-lg text-mady-ui whitespace-nowrap transition-colors',
                  activeCategory === cat.key
                    ? 'bg-mady-accent text-white font-medium'
                    : 'text-mady-text-secondary hover:bg-mady-bg-secondary hover:text-mady-text-primary',
                ].join(' ')}
              >
                {cat.label}
              </button>
            ))}
          </div>
        </div>

        {/* ── 模板网格 ── */}
        <div className="flex-1 overflow-y-auto px-5 pb-5">
          {loading ? (
            <div className="flex flex-col items-center justify-center py-12 text-mady-text-tertiary">
              <Loader size={24} className="mb-2 animate-spin opacity-40" />
              <p className="text-mady-ui">加载模板中...</p>
            </div>
          ) : filteredTemplates.length > 0 ? (
            <div className="grid grid-cols-2 gap-3">
              {filteredTemplates.map((template) => (
                <div
                  key={template.name}
                  className="rounded-xl border border-mady-separator bg-mady-bg-secondary p-4 flex flex-col"
                >
                  <div className="flex items-start gap-2 mb-2">
                    <FileText
                      size={14}
                      className="text-mady-accent shrink-0 mt-0.5"
                    />
                    <h3 className="text-mady-ui font-semibold text-mady-text-primary">
                      {template.name}
                    </h3>
                  </div>
                  <p className="text-mady-small text-mady-text-secondary mb-3 flex-1 line-clamp-3">
                    {template.description}
                  </p>
                  <button
                    onClick={() => handleUse(template)}
                    className="self-start px-3 py-1.5 rounded-lg text-mady-ui font-medium bg-mady-accent text-white hover:bg-mady-accent-hover transition-colors"
                  >
                    使用
                  </button>
                </div>
              ))}
            </div>
          ) : (
            <div className="flex flex-col items-center justify-center py-12 text-mady-text-tertiary">
              <FileText size={32} className="mb-2 opacity-40" />
              <p className="text-mady-ui">未找到匹配模板</p>
            </div>
          )}
        </div>
      </div>
    </ModalShell>
  )
}
