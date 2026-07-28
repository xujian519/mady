/**
 * McpServersSettings — MCP 服务器设置区段（对齐设计规范第10.5章）。
 *
 * 展示 MCP 服务器列表，包含名称、类型、状态指示。
 * 嵌入在 SettingsPanel 中使用。
 */

import React, { useCallback, useEffect, useState } from 'react'
import { listMcpServers, type McpServerEntry } from '@/lib/backend'
import { Loader2, AlertCircle, Server, RefreshCw, Globe, Terminal, Plug, CheckCircle } from 'lucide-react'

export const McpServersSettings: React.FC = () => {
  const [servers, setServers] = useState<McpServerEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setServers(await listMcpServers())
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void load() }, [load])

  return (
    <section>
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <Plug size={14} className="text-mady-accent" />
          <h3 className="text-mady-ui font-medium">MCP 服务器</h3>
          <span className="text-mady-caption text-mady-text-tertiary">
            {!loading && `${servers.length} 个`}
          </span>
        </div>
        <button
          onClick={() => void load()}
          className="flex items-center gap-1 px-2 py-1 rounded-md text-mady-caption text-mady-text-secondary hover:bg-mady-bg-hover transition-colors duration-150"
          disabled={loading}
        >
          <RefreshCw size={11} className={loading ? 'animate-spin' : ''} />
          刷新
        </button>
      </div>

      {loading ? (
        <div className="flex items-center justify-center gap-2 py-6 text-mady-text-tertiary text-mady-caption">
          <Loader2 size={12} className="animate-spin" />
          加载中…
        </div>
      ) : error ? (
        <div className="flex items-center gap-2 py-4 px-3 rounded-lg bg-mady-danger/5 border border-mady-danger/20 text-mady-ui text-mady-danger">
          <AlertCircle size={14} />
          <span>{error}</span>
        </div>
      ) : servers.length === 0 ? (
        <div className="py-6 px-3 text-center text-mady-ui text-mady-text-tertiary rounded-lg border border-dashed border-mady-border">
          <Server size={20} className="mx-auto mb-2 opacity-50" />
          <p>未配置 MCP 服务器</p>
          <p className="text-mady-caption mt-1">
            在 ~/.mady/mcp.json 或项目 .mcp.json 中添加配置
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {servers.map((s, i) => (
            <div
              key={`${s.source}-${s.name}-${i}`}
              className="rounded-lg border border-mady-border bg-mady-bg-secondary px-3 py-2.5"
            >
              <div className="flex items-center gap-2">
                {s.type === 'http' ? (
                  <Globe size={14} className="text-mady-accent shrink-0" />
                ) : (
                  <Terminal size={14} className="text-mady-accent shrink-0" />
                )}
                <span className="text-mady-ui font-medium text-mady-text-primary truncate flex-1">
                  {s.name}
                </span>
                {/* 状态指示：后端暂无实时状态，中性显示 */}
                <span className="flex items-center gap-1 text-mady-caption text-mady-text-tertiary">
                  <CheckCircle size={10} />
                  已配置
                </span>
                <span className="px-1.5 py-0.5 rounded bg-mady-accent-soft text-mady-accent text-mady-caption font-mono">
                  {s.type}
                </span>
                <span className="text-mady-caption text-mady-text-tertiary">
                  {s.source === 'global' ? '全局' : '项目'}
                </span>
              </div>
              <div className="mt-1 text-mady-caption text-mady-text-secondary font-mono truncate">
                {s.type === 'http' ? s.url : [s.command, ...(s.args ?? [])].join(' ')}
              </div>
              {s.envKeys && s.envKeys.length > 0 && (
                <div className="mt-1 flex flex-wrap gap-1">
                  {s.envKeys.map((k) => (
                    <span
                      key={k}
                      className="px-1.5 py-0.5 rounded bg-mady-bg-primary text-mady-caption text-mady-text-tertiary font-mono"
                    >
                      {k}=•••
                    </span>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </section>
  )
}
