/**
 * KnowledgeView — 知识库管理面板（覆盖层）。
 *
 * 功能：
 * 1. 知识库状态概览（文档数/索引大小/更新时间/源目录）
 * 2. 索引范围选择（专利法/审查指南/判例/自定义）
 * 3. 嵌入模型 / Rerank 模型设置（保存后重启应用生效）
 * 4. 本地 oMLX 服务状态（嵌入/Rerank 依赖）
 *
 * 模型设置持久化到 ~/.mady/knowledge-settings.json，应用启动时注入
 * OMLX_* 环境变量装配到知识库检索链路（见 desktop/app_knowledge_settings.go）。
 * 切换嵌入模型不会重建预构建的 knowledge.db 向量索引（BGE-M3 1024 维），
 * 维度不匹配时向量检索安全降级为关键词检索（FTS-only）。
 */

import React, { useEffect, useState } from 'react'
import { Database, Folder, KeyRound, Layers, Loader, Save, Server, X } from 'lucide-react'
import {
  useKnowledgeModelSettings,
  useKnowledgeStatus,
  useOmlxServiceStatus,
  useSaveKnowledgeModelSettings,
} from '@/queries/knowledge'
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

/** 设置输入框统一样式（模型设置区共用，避免三处重复的长类名）。 */
const inputCls =
  'w-full rounded-lg px-3 py-2 bg-mady-bg-primary border border-mady-border text-mady-body text-mady-text-primary placeholder-mady-text-tertiary outline-none focus:border-mady-accent focus:ring-2 focus:ring-mady-accent/20 transition-all duration-150 font-mono text-mady-small'

export const KnowledgeView: React.FC<KnowledgeViewProps> = ({ onClose }) => {
  const knowledgeQuery = useKnowledgeStatus()
  const data = knowledgeQuery.data ?? null
  const loading = knowledgeQuery.isLoading
  const [checked, setChecked] = useState<Set<string>>(
    () => new Set(SCOPE_OPTIONS.map((o) => o.key)),
  )

  // ── 模型设置（TanStack Query 接管加载/保存） ──
  const modelsQuery = useKnowledgeModelSettings()
  const saveModels = useSaveKnowledgeModelSettings()
  const omlxQuery = useOmlxServiceStatus()

  const [baseURL, setBaseURL] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [embedModel, setEmbedModel] = useState('')
  const [rerankEnabled, setRerankEnabled] = useState(false)
  const [rerankModel, setRerankModel] = useState('')
  const [saved, setSaved] = useState(false)

  // 后端设置加载后填入表单（仅生效一次；apiKey 为掩码，保持原值语义）
  useEffect(() => {
    if (!modelsQuery.data) return
    setBaseURL(modelsQuery.data.baseURL)
    setApiKey(modelsQuery.data.apiKey)
    setEmbedModel(modelsQuery.data.embedModel)
    setRerankEnabled(modelsQuery.data.rerankEnabled)
    setRerankModel(modelsQuery.data.rerankModel)
  }, [modelsQuery.data])

  const handleSave = () => {
    setSaved(false)
    saveModels.mutate(
      { baseURL, apiKey, embedModel, rerankModel, rerankEnabled },
      {
        onSuccess: () => setSaved(true),
      },
    )
  }

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
      <div className="w-[520px] max-h-[80vh] bg-mady-bg-primary rounded-2xl border border-mady-separator shadow-mady-modal overflow-y-auto">
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

          {/* ── 嵌入模型 / Rerank 模型设置 ────────── */}
          <section>
            <h3 className="text-mady-ui font-medium text-mady-text-primary mb-3">模型设置</h3>

            {/* 本地服务状态 */}
            <div className="flex items-center gap-2 rounded-lg bg-mady-bg-secondary px-3 py-2.5 mb-3">
              <Server size={13} className="text-mady-text-tertiary shrink-0" />
              {omlxQuery.isLoading ? (
                <span className="text-mady-small text-mady-text-tertiary">正在检测 oMLX 服务...</span>
              ) : omlxQuery.data ? (
                <>
                  <span
                    className={`w-2 h-2 rounded-full shrink-0 ${
                      omlxQuery.data.running
                        ? 'bg-mady-success'
                        : omlxQuery.data.installed
                          ? 'bg-mady-warning'
                          : 'bg-mady-danger'
                    }`}
                  />
                  <span className="text-mady-small text-mady-text-secondary">{omlxQuery.data.message}</span>
                </>
              ) : (
                <span className="text-mady-small text-mady-text-tertiary">服务状态不可用</span>
              )}
            </div>

            {modelsQuery.isLoading ? (
              <div className="flex items-center justify-center py-6 text-mady-text-tertiary">
                <Loader size={14} className="mr-2 animate-spin" />
                <span className="text-mady-ui">加载设置中...</span>
              </div>
            ) : (
              <div className="space-y-3">
                {/* 嵌入模型 */}
                <div className="rounded-lg bg-mady-bg-secondary p-3 space-y-2.5">
                  <div className="flex items-center gap-1.5 text-mady-caption text-mady-text-tertiary">
                    <Layers size={11} />
                    <span>嵌入模型（Embedding）</span>
                  </div>
                  <div>
                    <label className="block text-mady-caption text-mady-text-tertiary mb-1">服务地址（BaseURL）</label>
                    <input
                      type="text"
                      value={baseURL}
                      onChange={(e) => setBaseURL(e.target.value)}
                      placeholder="http://127.0.0.1:8000/v1"
                      className={inputCls}
                    />
                  </div>
                  <div>
                    <label className="flex items-center gap-1 text-mady-caption text-mady-text-tertiary mb-1">
                      <KeyRound size={10} />
                      <span>API Key（掩码表示保持原值，留空清除）</span>
                    </label>
                    <input
                      type="password"
                      value={apiKey}
                      onChange={(e) => setApiKey(e.target.value)}
                      placeholder="未配置，向量检索不可用"
                      className={inputCls}
                    />
                  </div>
                  <div>
                    <label className="block text-mady-caption text-mady-text-tertiary mb-1">嵌入模型名</label>
                    <input
                      type="text"
                      value={embedModel}
                      onChange={(e) => setEmbedModel(e.target.value)}
                      placeholder="bge-m3-mlx-8bit"
                      className={inputCls}
                    />
                  </div>
                  <p className="text-mady-caption text-mady-text-tertiary leading-relaxed">
                    切换嵌入模型后，预构建知识库的向量检索将降级为关键词检索（FTS）；
                    仅新写入的用户文档使用新模型。
                  </p>
                </div>

                {/* Rerank 模型 */}
                <div className="rounded-lg bg-mady-bg-secondary p-3 space-y-2.5">
                  <label className="flex items-center gap-2.5 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={rerankEnabled}
                      onChange={(e) => setRerankEnabled(e.target.checked)}
                      className="w-4 h-4 rounded border-mady-border text-mady-accent accent-mady-accent focus:ring-1 focus:ring-mady-accent focus:ring-offset-0"
                    />
                    <span className="text-mady-body text-mady-text-primary">启用 Rerank 重排</span>
                  </label>
                  <div>
                    <label className="block text-mady-caption text-mady-text-tertiary mb-1">重排模型名</label>
                    <input
                      type="text"
                      value={rerankModel}
                      onChange={(e) => setRerankModel(e.target.value)}
                      disabled={!rerankEnabled}
                      placeholder="Qwen3-Reranker-4B-4bit-MLX"
                      className={`${inputCls} disabled:opacity-50 disabled:cursor-not-allowed`}
                    />
                  </div>
                </div>

                {/* 保存 */}
                <div className="flex items-center justify-between gap-3 pt-1">
                  <div className="flex-1 min-w-0">
                    {saved && (
                      <p className="text-mady-caption text-mady-success">已保存 ✓ 重启应用后生效</p>
                    )}
                    {saveModels.isError && (
                      <p className="text-mady-caption text-mady-danger">
                        保存失败：{saveModels.error instanceof Error ? saveModels.error.message : '未知错误'}
                      </p>
                    )}
                    {!saved && !saveModels.isError && (
                      <p className="text-mady-caption text-mady-text-tertiary">保存后重启应用生效（与 AI 模型设置一致）</p>
                    )}
                  </div>
                  <button
                    onClick={handleSave}
                    disabled={saveModels.isPending}
                    className="flex items-center gap-1.5 shrink-0 rounded-lg px-3 py-2 bg-mady-accent text-mady-bg-primary text-mady-ui font-medium hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <Save size={13} />
                    {saveModels.isPending ? '保存中...' : '保存设置'}
                  </button>
                </div>
              </div>
            )}
          </section>
        </div>
      </div>
    </ModalShell>
  )
}
