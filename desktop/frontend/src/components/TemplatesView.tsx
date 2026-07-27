/**
 * TemplatesView — 专利模板库视图（覆盖层）。
 *
 * 分类展示各类专利撰写模板，支持搜索筛选和选择使用。
 */

import React, { useState } from 'react'
import { FileText, Search, X } from 'lucide-react'

// ── 类型定义 ────────────────────────────────────────

interface TemplateItem {
  /** 模板名称。 */
  name: string
  /** 简短描述。 */
  description: string
  /** 所属分类键值。 */
  category: string
  /** 模板内容（可包含占位符）。 */
  content: string
}

export interface TemplatesViewProps {
  /** 关闭回调。 */
  onClose: () => void
  /** 用户选择使用模板时的回调。 */
  onUseTemplate?: (template: { name: string; category: string; content: string }) => void
}

// ── 常量 ────────────────────────────────────────────

const CATEGORIES = [
  { key: 'claims', label: '权利要求书' },
  { key: 'specification', label: '说明书' },
  { key: 'abstract', label: '摘要' },
  { key: 'oa-response', label: 'OA答复函' },
  { key: 'pct', label: 'PCT申请' },
] as const

type CategoryKey = (typeof CATEGORIES)[number]['key']

// ── 模拟数据 ────────────────────────────────────────

const MOCK_TEMPLATES: Record<CategoryKey, TemplateItem[]> = {
  claims: [
    {
      name: '独立权利要求模板',
      description:
        '适用于发明专利和实用新型的独立权利要求撰写，含前序部分和特征部分的标准结构',
      category: 'claims',
      content: '一种……，其特征在于，……',
    },
    {
      name: '从属权利要求模板',
      description:
        '从属权利要求的标准撰写格式，含引用关系和附加技术特征',
      category: 'claims',
      content: '如权利要求1所述的……，其特征在于，……',
    },
    {
      name: 'Markush式权利要求模板',
      description:
        '化学领域Markush组表达方式的权利要求模板，适用于化合物通式保护',
      category: 'claims',
      content: '选自由……组成的组中的至少一种',
    },
  ],
  specification: [
    {
      name: '发明专利说明书模板',
      description:
        '发明专利说明书标准结构，含技术领域、背景技术、发明内容、附图说明、具体实施方式',
      category: 'specification',
      content: '【技术领域】\n……\n【背景技术】\n……',
    },
    {
      name: '实用新型说明书模板',
      description:
        '实用新型说明书标准结构，含结构说明、工作原理、有益效果等模块',
      category: 'specification',
      content: '【技术领域】\n本实用新型涉及……\n【背景技术】\n……',
    },
    {
      name: 'PCT申请说明书模板',
      description:
        'PCT国际申请说明书模板，符合PCT实施细则第5条和行政规程要求',
      category: 'specification',
      content: '[Technical Field]\n……\n[Background Art]\n……',
    },
  ],
  abstract: [
    {
      name: '标准摘要模板',
      description:
        '发明专利申请摘要的标准格式，限300字以内，含技术方案和主要用途',
      category: 'abstract',
      content: '本发明公开了一种……，包括……本发明……',
    },
    {
      name: '化学类摘要模板',
      description:
        '化学/生物领域专利摘要模板，含通式化合物、制备方法和医药用途',
      category: 'abstract',
      content: '本发明涉及通式(I)所示的化合物……',
    },
  ],
  'oa-response': [
    {
      name: '答复审查意见模板',
      description:
        '针对审查意见通知书的标准答复模板，含意见陈述和权利要求修改说明',
      category: 'oa-response',
      content:
        '尊敬的审查员：\n针对贵局于……发出的审查意见通知书，申请人现答复如下……',
    },
    {
      name: '修改权利要求书模板',
      description:
        '应审查意见修改权利要求书的规范格式，含修改对照说明',
      category: 'oa-response',
      content:
        '根据《专利法》第33条和《专利法实施细则》第51条的规定……',
    },
    {
      name: '意见陈述书模板',
      description:
        '意见陈述书标准格式，含驳回理由反驳和论证逻辑框架',
      category: 'oa-response',
      content:
        '申请人认真研究了审查意见通知书中的驳回理由……',
    },
  ],
  pct: [
    {
      name: 'PCT请求书模板',
      description:
        'PCT国际申请请求书（PCT/RO/101）填写指南和范例',
      category: 'pct',
      content: '本国际申请请求按照专利合作条约（PCT）处理……',
    },
    {
      name: 'PCT说明书模板',
      description:
        'PCT国际申请说明书撰写模板，含序列表和核苷酸/氨基酸序列公开要求',
      category: 'pct',
      content: '[Title of Invention]\n……\n[Technical Field]\n……',
    },
    {
      name: 'PCT权利要求书模板',
      description:
        'PCT国际阶段权利要求书格式规范，符合PCT细则第6条',
      category: 'pct',
      content: 'What is claimed is:\n1. ……',
    },
  ],
}

// ── 组件 ────────────────────────────────────────────

export const TemplatesView: React.FC<TemplatesViewProps> = ({
  onClose,
  onUseTemplate,
}) => {
  const [activeCategory, setActiveCategory] = useState<CategoryKey>('claims')
  const [searchQuery, setSearchQuery] = useState('')

  const templates = MOCK_TEMPLATES[activeCategory] ?? []
  const filteredTemplates = searchQuery.trim()
    ? templates.filter((t) =>
        t.name.toLowerCase().includes(searchQuery.toLowerCase()),
      )
    : templates

  const handleUse = (template: TemplateItem) => {
    console.log('使用模板:', template)
    onUseTemplate?.({
      name: template.name,
      category: template.category,
      content: template.content,
    })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/20 backdrop-blur-sm">
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
            {CATEGORIES.map((cat) => (
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
          {filteredTemplates.length > 0 ? (
            <div className="grid grid-cols-2 gap-3">
              {filteredTemplates.map((template, idx) => (
                <div
                  key={`${template.name}-${idx}`}
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
    </div>
  )
}
