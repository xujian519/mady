/**
 * McpView — MCP 服务器只读面板（T5.7，PilotDeck Settings → MCP Servers 对齐）。
 *
 * 展示 ~/.mady/mcp.json 与项目 .mcp.json 中配置的服务器：
 * - 名称 / 类型（stdio/http）/ 命令或 URL / 来源
 * - env 仅显示键名（不暴露值，防密钥泄露）
 * - 本期只读；增删改不在范围内（信任存储为安全敏感路径）
 */

import React, { useCallback } from 'react'
import { X, Loader2, AlertCircle, Server, RefreshCw, Globe, Terminal } from 'lucide-react'
import { useMcpServers } from '@/queries/mcp'
import { ModalShell } from './ModalShell'

interface McpViewProps {
  onClose: () => void
}

export const McpView: React.FC<McpViewProps> = ({ onClose }) => {
  const serversQuery = useMcpServers()
  const servers = serversQuery.data ?? []
  const loading = serversQuery.isLoading
  const error = serversQuery.isError
    ? (serversQuery.error instanceof Error ? serversQuery.error.message : String(serversQuery.error))
    : null
  const load = useCallback(() => { void serversQuery.refetch() }, [serversQuery])

  return (
    <ModalShell onClose={onClose} ariaLabel="MCP 服务器">
      <div className="w-[640px] max-w-[92vw] max-h-[80vh] bg-mady-bg-primary rounded-xl shadow-2xl border border-mady-separator flex flex-col overflow-hidden">
        {/* 标题栏 */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-mady-separator">
          <div className="flex items-center gap-2">
            <Server size={16} className="text-mady-accent" />
            <h2 className="text-mady-ui font-medium text-mady-text-primary">MCP 服务器</h2>
            <span className="text-mady-caption text-mady-text-tertiary">{servers.length} 个已配置</span>
          </div>
          <div className="flex items-center gap-1">
            <button
              onClick={() => void load()}
              className="p-1.5 rounded hover:bg-mady-bg-secondary text-mady-text-secondary"
              title="刷新"
            >
              <RefreshCw size={14} />
            </button>
            <button
              onClick={onClose}
              className="p-1.5 rounded hover:bg-mady-bg-secondary text-mady-text-secondary"
              title="关闭"
            >
              <X size={15} />
            </button>
          </div>
        </div>

        {/* 列表 */}
        <div className="flex-1 overflow-y-auto">
          {loading ? (
            <div className="flex items-center justify-center gap-2 py-12 text-mady-text-tertiary text-mady-ui">
              <Loader2 size={14} className="animate-spin" />
              加载中…
            </div>
          ) : error ? (
            <div className="flex flex-col items-center gap-2 py-12 px-6 text-center">
              <AlertCircle size={18} className="text-mady-warning" />
              <span className="text-mady-ui text-mady-text-secondary">{error}</span>
            </div>
          ) : servers.length === 0 ? (
            <div className="py-12 px-6 text-center text-mady-ui text-mady-text-tertiary">
              未配置 MCP 服务器
              <div className="text-mady-caption mt-1">
                在 ~/.mady/mcp.json 或项目 .mcp.json 中添加 mcpServers 配置
              </div>
            </div>
          ) : (
            servers.map((s) => (
              <div key={`${s.source}-${s.name}`} className="px-4 py-3 border-b border-mady-separator/50">
                <div className="flex items-center gap-2">
                  {s.type === 'http' ? (
                    <Globe size={14} className="text-mady-accent shrink-0" />
                  ) : (
                    <Terminal size={14} className="text-mady-accent shrink-0" />
                  )}
                  <span className="text-mady-ui font-medium text-mady-text-primary">{s.name}</span>
                  <span className="px-1.5 py-0.5 rounded bg-mady-accent-soft text-mady-accent text-mady-caption">
                    {s.type}
                  </span>
                  <span className="ml-auto text-mady-caption text-mady-text-tertiary">
                    {s.source === 'global' ? '全局' : '项目'}
                  </span>
                </div>
                <div className="mt-1.5 text-mady-caption text-mady-text-secondary font-mono truncate">
                  {s.type === 'http' ? s.url : [s.command, ...(s.args ?? [])].join(' ')}
                </div>
                {s.envKeys && s.envKeys.length > 0 && (
                  <div className="mt-1 flex flex-wrap gap-1">
                    {s.envKeys.map((k) => (
                      <span
                        key={k}
                        className="px-1.5 py-0.5 rounded bg-mady-bg-secondary text-mady-caption text-mady-text-tertiary font-mono"
                      >
                        {k}=•••
                      </span>
                    ))}
                  </div>
                )}
              </div>
            ))
          )}
        </div>
      </div>
    </ModalShell>
  )
}
