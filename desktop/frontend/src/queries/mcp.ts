/**
 * MCP 服务器列表查询（TanStack Query）。
 *
 * 封装 App.ListMcpServers（只读 MCP 配置），供 McpView /
 * McpServersSettings 共用，避免各自手写拉取与重复请求
 * （见 mady-desktop-standards.md M-DSK-ST-002）。
 */
import { useQuery } from '@tanstack/react-query'
import { listMcpServers, type McpServerEntry } from '@/lib/backend'

/** MCP 列表 query key。 */
export const mcpKeys = {
  all: ['mcp'] as const,
}

/**
 * 查询已配置的 MCP 服务器列表（只读）。
 */
export function useMcpServers() {
  return useQuery({
    queryKey: mcpKeys.all,
    queryFn: () => listMcpServers(),
    staleTime: 30_000,
    retry: 1,
  })
}

export type { McpServerEntry }
