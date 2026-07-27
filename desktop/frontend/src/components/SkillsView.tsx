/**
 * SkillsView — 技能管理面板（T5.6，PilotDeck Skills tab 对齐）。
 *
 * 左侧技能列表（名称 + 描述），右侧 SKILL.md 查看/编辑：
 * - 预览模式：MarkdownRenderer
 * - 编辑模式：CodeEditor（Cmd/Ctrl+S 保存）
 * - 未保存修改关闭时确认
 */

import React, { useCallback, useEffect, useState } from 'react'
import { X, Loader2, AlertCircle, Sparkles, Eye, Pencil, Save, RefreshCw } from 'lucide-react'
import { listSkills, readFile, writeFile, type SkillEntry } from '@/lib/backend'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'
import { CodeEditor } from '@/components/fileviewer/CodeEditor'

interface SkillsViewProps {
  onClose: () => void
}

export const SkillsView: React.FC<SkillsViewProps> = ({ onClose }) => {
  const [skills, setSkills] = useState<SkillEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<SkillEntry | null>(null)

  // SKILL.md 内容状态
  const [content, setContent] = useState('')
  const [draft, setDraft] = useState<string | null>(null)
  const [fileLoading, setFileLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [mode, setMode] = useState<'preview' | 'edit'>('preview')
  const dirty = draft !== null

  const loadSkills = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setSkills(await listSkills())
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadSkills()
  }, [loadSkills])

  const openSkill = useCallback(
    async (skill: SkillEntry) => {
      if (dirty && !window.confirm('有未保存的修改，确定切换？')) return
      setSelected(skill)
      setMode('preview')
      setDraft(null)
      setFileLoading(true)
      try {
        const fc = await readFile(skill.path)
        setContent(fc.text ?? '')
      } catch (err: unknown) {
        setContent('')
        setError(err instanceof Error ? err.message : String(err))
      } finally {
        setFileLoading(false)
      }
    },
    [dirty],
  )

  const handleSave = useCallback(async () => {
    if (!selected || draft === null) return
    setSaving(true)
    try {
      await writeFile(selected.path, draft)
      setContent(draft)
      setDraft(null)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSaving(false)
    }
  }, [selected, draft])

  const handleClose = useCallback(() => {
    if (dirty && !window.confirm('有未保存的修改，确定关闭？')) return
    onClose()
  }, [dirty, onClose])

  const text = draft ?? content

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/20 backdrop-blur-sm">
      <div className="w-[860px] max-w-[92vw] h-[600px] max-h-[85vh] bg-mady-bg-primary rounded-xl shadow-2xl border border-mady-separator flex flex-col overflow-hidden">
        {/* 标题栏 */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-mady-separator">
          <div className="flex items-center gap-2">
            <Sparkles size={16} className="text-mady-accent" />
            <h2 className="text-mady-ui font-medium text-mady-text-primary">技能管理</h2>
            <span className="text-mady-caption text-mady-text-tertiary">{skills.length} 个技能</span>
          </div>
          <div className="flex items-center gap-1">
            <button
              onClick={() => void loadSkills()}
              className="p-1.5 rounded hover:bg-mady-bg-secondary text-mady-text-secondary"
              title="刷新"
            >
              <RefreshCw size={14} />
            </button>
            <button
              onClick={handleClose}
              className="p-1.5 rounded hover:bg-mady-bg-secondary text-mady-text-secondary"
              title="关闭"
            >
              <X size={15} />
            </button>
          </div>
        </div>

        {/* 主体 */}
        <div className="flex-1 flex overflow-hidden">
          {/* 技能列表 */}
          <div className="w-64 shrink-0 border-r border-mady-separator overflow-y-auto">
            {loading ? (
              <div className="flex items-center justify-center gap-2 py-8 text-mady-text-tertiary text-mady-ui">
                <Loader2 size={14} className="animate-spin" />
                加载中…
              </div>
            ) : skills.length === 0 ? (
              <div className="py-8 px-4 text-center text-mady-caption text-mady-text-tertiary">
                项目 skills/ 目录下暂无技能
              </div>
            ) : (
              skills.map((s) => (
                <button
                  key={s.path}
                  onClick={() => void openSkill(s)}
                  className={`w-full text-left px-3 py-2 border-b border-mady-separator/50 transition-colors ${
                    selected?.path === s.path ? 'bg-mady-accent-soft' : 'hover:bg-mady-bg-secondary'
                  }`}
                >
                  <div className="text-mady-ui font-medium text-mady-text-primary truncate">{s.name}</div>
                  {s.description && (
                    <div className="text-mady-caption text-mady-text-tertiary truncate mt-0.5">
                      {s.description}
                    </div>
                  )}
                </button>
              ))
            )}
          </div>

          {/* SKILL.md 查看/编辑 */}
          <div className="flex-1 flex flex-col min-w-0">
            {error && (
              <div className="flex items-center gap-2 px-4 py-2 bg-mady-danger/10 text-mady-danger text-mady-caption">
                <AlertCircle size={12} />
                {error}
              </div>
            )}
            {!selected ? (
              <div className="flex-1 flex items-center justify-center text-mady-text-tertiary text-mady-ui">
                选择左侧技能查看或编辑 SKILL.md
              </div>
            ) : (
              <>
                <div className="flex items-center justify-between px-3 py-2 border-b border-mady-separator bg-mady-bg-secondary/30">
                  <span className="text-mady-caption text-mady-text-tertiary truncate">{selected.path}</span>
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => setMode((m) => (m === 'preview' ? 'edit' : 'preview'))}
                      className="p-1 rounded hover:bg-mady-bg-secondary text-mady-text-secondary"
                      title={mode === 'preview' ? '编辑' : '预览'}
                    >
                      {mode === 'preview' ? <Pencil size={13} /> : <Eye size={13} />}
                    </button>
                    {dirty && (
                      <button
                        onClick={() => void handleSave()}
                        disabled={saving}
                        className="p-1 rounded hover:bg-mady-bg-secondary text-mady-accent disabled:opacity-50"
                        title="保存 (⌘S)"
                      >
                        {saving ? <Loader2 size={13} className="animate-spin" /> : <Save size={13} />}
                      </button>
                    )}
                  </div>
                </div>
                <div className="flex-1 overflow-hidden">
                  {fileLoading ? (
                    <div className="h-full flex items-center justify-center gap-2 text-mady-text-tertiary text-mady-ui">
                      <Loader2 size={14} className="animate-spin" />
                      加载中…
                    </div>
                  ) : mode === 'preview' ? (
                    <div className="h-full overflow-y-auto p-4">
                      <MarkdownRenderer content={text} />
                    </div>
                  ) : (
                    <CodeEditor
                      value={text}
                      markdown
                      onChange={setDraft}
                      onSave={() => void handleSave()}
                    />
                  )}
                </div>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
