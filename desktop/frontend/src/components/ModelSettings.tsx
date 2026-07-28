/**
 * ModelSettings — 模型与推理设置（§10.3）。
 *
 * 区域：
 * - 当前模型选择器（Pill 形状，radius/2xl 16px）
 * - 推理力度 Segmented（低/中/高/极高）
 * - 上下文窗口 Dropdown
 * - 温度 Slider（0–2，step 0.1）
 *
 * 所有设置通过 settingsStore 持久化到 localStorage。
 * 模型列表当前为 mock 数据（后端尚无 model/list 端点），
 * 后端就绪后替换 fetchModels() 体内的 MOCK_MODELS 为 API 调用即可。
 */

import React, { useState, useRef, useEffect } from 'react'
import { ChevronDown, Cpu, Thermometer, SlidersHorizontal, Brain } from 'lucide-react'
import { AnimatePresence, motion } from 'framer-motion'
import { useSettingsStore } from '@/stores/settings'

// ── 类型 ──────────────────────────────────────────

interface ModelOption {
  id: string
  name: string
  provider: string
  group: 'recommended' | 'all'
  capabilities: string[]
  reasoningLabel: string
}

/**
 * 获取可用模型列表。
 *
 * 当前返回 mock 数据。后端 model/list 端点就绪后，替换为：
 *   export async function fetchModels(): Promise<ModelOption[]> {
 *     const res = await callBinding(...)
 *     return res.map(...)
 *   }
 */
function fetchModels(): ModelOption[] {
  return [
    {
      id: 'gpt-5.2',
      name: 'GPT-5.2',
      provider: 'OpenAI',
      group: 'recommended',
      capabilities: ['256K 上下文', '多模态', '深度推理'],
      reasoningLabel: '高',
    },
    {
      id: 'gpt-5.1',
      name: 'GPT-5.1',
      provider: 'OpenAI',
      group: 'recommended',
      capabilities: ['128K 上下文', '多模态'],
      reasoningLabel: '中',
    },
    {
      id: 'deepseek-v4',
      name: 'DeepSeek V4',
      provider: 'DeepSeek',
      group: 'recommended',
      capabilities: ['1M 上下文', '代码优化'],
      reasoningLabel: '极高',
    },
    {
      id: 'kimi-k3',
      name: 'Kimi K3',
      provider: 'Moonshot',
      group: 'all',
      capabilities: ['128K 上下文', '文件分析'],
      reasoningLabel: '中',
    },
    {
      id: 'claude-5',
      name: 'Claude 5',
      provider: 'Anthropic',
      group: 'all',
      capabilities: ['200K 上下文', '多模态', '安全'],
      reasoningLabel: '高',
    },
  ]
}

const ALL_MODELS = fetchModels()

const EFFORT_OPTIONS = [
  { value: 'low', label: '低' },
  { value: 'medium', label: '中' },
  { value: 'high', label: '高' },
  { value: 'max', label: '极高' },
] as const

const CONTEXT_OPTIONS = ['32K', '64K', '128K', '256K', '1M'] as const

// ── Component ─────────────────────────────────────

export const ModelSettings: React.FC = () => {
  const store = useSettingsStore()
  const [dropdownOpen, setDropdownOpen] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)

  // 从 store 读取当前模型，按 id 匹配
  const selectedModel = ALL_MODELS.find((m) => m.id === store.modelId) ?? ALL_MODELS[0]

  // 点击外部关闭下拉
  useEffect(() => {
    if (!dropdownOpen) return
    const handleClick = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setDropdownOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [dropdownOpen])

  const recommended = ALL_MODELS.filter((m) => m.group === 'recommended')
  const allOthers = ALL_MODELS.filter((m) => m.group === 'all')

  return (
    <section>
      {/* 标题行 */}
      <div className="flex items-center gap-2 mb-3">
        <Brain size={14} className="text-mady-accent" />
        <h3 className="text-mady-ui font-medium">模型与推理</h3>
      </div>

      <div className="space-y-4">
        {/* ── 10.3.2 当前模型选择器 ── */}
        <div ref={dropdownRef} className="relative">
          <button
            onClick={() => setDropdownOpen(!dropdownOpen)}
            className="
              w-full flex items-center gap-3 px-3
              h-12 rounded-2xl border border-mady-border
              bg-mady-bg-tertiary
              hover:border-mady-accent/40 transition-colors duration-150
              text-left
            "
          >
            <Cpu size={16} className="text-mady-accent shrink-0" />
            <div className="flex-1 min-w-0">
              <span className="text-mady-body font-medium text-mady-text-primary">
                {selectedModel.name}
              </span>
            </div>
            <span className="font-mono text-mady-small text-mady-text-secondary">
              · {selectedModel.reasoningLabel}
            </span>
            <ChevronDown
              size={12}
              className={`text-mady-text-tertiary transition-transform duration-150 ${
                dropdownOpen ? 'rotate-180' : ''
              }`}
            />
          </button>

          <AnimatePresence>
            {dropdownOpen && (
              <motion.div
                initial={{ opacity: 0, y: -4, scaleY: 0.96 }}
                animate={{ opacity: 1, y: 0, scaleY: 1 }}
                exit={{ opacity: 0, y: -4, scaleY: 0.96 }}
                transition={{ duration: 0.15, ease: 'easeOut' }}
                className="
                  absolute top-full left-0 right-0 z-50 mt-1.5
                  bg-mady-bg-tertiary border border-mady-border
                  rounded-xl shadow-mady-floating
                  overflow-hidden
                "
              >
                {recommended.length > 0 && (
                  <div>
                    <div className="px-3 py-1.5 text-mady-caption font-medium text-mady-text-secondary tracking-wide uppercase">
                      推荐
                    </div>
                    {recommended.map((model) => (
                      <ModelRow
                        key={model.id}
                        model={model}
                        selected={model.id === store.modelId}
                        onSelect={() => {
                          store.update({ modelId: model.id })
                          setDropdownOpen(false)
                        }}
                      />
                    ))}
                  </div>
                )}
                {allOthers.length > 0 && (
                  <div>
                    <div className="border-t border-mady-separator" />
                    <div className="px-3 py-1.5 text-mady-caption font-medium text-mady-text-secondary tracking-wide uppercase">
                      全部
                    </div>
                    {allOthers.map((model) => (
                      <ModelRow
                        key={model.id}
                        model={model}
                        selected={model.id === store.modelId}
                        onSelect={() => {
                          store.update({ modelId: model.id })
                          setDropdownOpen(false)
                        }}
                      />
                    ))}
                  </div>
                )}
              </motion.div>
            )}
          </AnimatePresence>
        </div>

        {/* ── 10.3.3 推理设置 ── */}

        {/* 推理力度 Segmented */}
        <div>
          <label className="flex items-center gap-1.5 text-mady-caption text-mady-text-secondary mb-1.5">
            <SlidersHorizontal size={11} />
            推理力度
          </label>
          <div className="flex gap-1 p-1 rounded-lg bg-mady-bg-secondary border border-mady-border">
            {EFFORT_OPTIONS.map((opt) => (
              <button
                key={opt.value}
                onClick={() => store.update({ reasoningEffort: opt.value })}
                className={`
                  flex-1 px-2 py-1.5 rounded-md text-mady-small font-medium transition-all duration-100
                  ${
                    store.reasoningEffort === opt.value
                      ? 'bg-mady-accent text-white shadow-sm'
                      : 'text-mady-text-secondary hover:text-mady-text-primary'
                  }
                `}
              >
                {opt.label}
              </button>
            ))}
          </div>
        </div>

        {/* 上下文窗口 Dropdown */}
        <div>
          <label className="flex items-center gap-1.5 text-mady-caption text-mady-text-secondary mb-1.5">
            <Cpu size={11} />
            上下文窗口
          </label>
          <div className="relative">
            <select
              value={store.contextWindow}
              onChange={(e) => store.update({ contextWindow: e.target.value })}
              className="
                w-full appearance-none rounded-lg px-3 py-2 pr-8
                bg-mady-bg-secondary border border-mady-border
                text-mady-body text-mady-text-primary
                outline-none focus:border-mady-accent focus:ring-2 focus:ring-mady-accent/20
                transition-all duration-150
              "
            >
              {CONTEXT_OPTIONS.map((opt) => (
                <option key={opt} value={opt}>
                  {opt}
                </option>
              ))}
            </select>
            <ChevronDown
              size={12}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 text-mady-text-tertiary pointer-events-none"
            />
          </div>
        </div>

        {/* 温度 Slider */}
        <div>
          <label className="flex items-center gap-1.5 text-mady-caption text-mady-text-secondary mb-1.5">
            <Thermometer size={11} />
            温度
            <span className="text-mady-text-tertiary ml-auto font-mono text-mady-caption">
              {store.temperature.toFixed(1)}
            </span>
          </label>
          <div className="relative pt-1 pb-2">
            <div className="relative h-1 rounded-full bg-mady-border">
              <div
                className="absolute inset-y-0 left-0 rounded-full bg-mady-accent transition-[width] duration-100"
                style={{ width: `${(store.temperature / 2) * 100}%` }}
              />
            </div>
            <input
              type="range"
              min={0}
              max={2}
              step={0.1}
              value={store.temperature}
              onChange={(e) => store.update({ temperature: parseFloat(e.target.value) })}
              className="
                absolute inset-0 w-full h-full opacity-0 cursor-pointer
                m-0 p-0
              "
            />
            <div
              className="absolute top-1/2 -translate-y-1/2 w-4 h-4 rounded-full bg-white shadow-[0_1px_4px_rgba(0,0,0,0.3)] border border-mady-border pointer-events-none transition-transform duration-150 hover:scale-110"
              style={{
                left: `calc(${(store.temperature / 2) * 100}% - 8px)`,
              }}
            />
          </div>
          <div className="flex justify-between text-mady-caption text-mady-text-tertiary px-0.5">
            <span>精确</span>
            <span>0.7</span>
            <span>创意</span>
          </div>
        </div>
      </div>
    </section>
  )
}

// ── ModelRow（下拉菜单中的模型行） ──────────────────

interface ModelRowProps {
  model: ModelOption
  selected: boolean
  onSelect: () => void
}

const ModelRow: React.FC<ModelRowProps> = ({ model, selected, onSelect }) => (
  <button
    onClick={onSelect}
    className={`
      w-full flex items-center gap-3 px-3 py-2.5 text-left
      transition-colors duration-100
      ${selected ? 'bg-mady-bg-active' : 'hover:bg-mady-bg-hover'}
    `}
  >
    <Cpu
      size={14}
      className={`shrink-0 ${selected ? 'text-mady-accent' : 'text-mady-text-tertiary'}`}
    />
    <div className="flex-1 min-w-0">
      <div className="flex items-center gap-2">
        <span
          className={`text-mady-ui font-medium ${selected ? 'text-mady-accent' : 'text-mady-text-primary'}`}
        >
          {model.name}
        </span>
        <div className="flex gap-1">
          {model.capabilities.slice(0, 2).map((cap) => (
            <span
              key={cap}
              className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-mady-bg-hover text-mady-text-tertiary leading-none"
            >
              {cap}
            </span>
          ))}
        </div>
      </div>
      <span className="text-mady-caption text-mady-text-tertiary">{model.provider}</span>
    </div>
    <span className="font-mono text-mady-small text-mady-text-secondary">{model.reasoningLabel}</span>
  </button>
)
