/**
 * 记忆管理的 TanStack Query hooks（阶段 4：记忆面板）。
 *
 * 与线程管理同构：Query 管 server-state（记忆列表/检索结果），
 * 变更动作后失效重取。
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as backend from '@/lib/backend'

export const memoryKeys = {
  all: ['memories'] as const,
  search: (q: string) => ['memories', 'search', q] as const,
}

/**
 * 查询全部三层记忆列表。
 */
export function useMemories(limit = 100) {
  return useQuery({
    queryKey: memoryKeys.all,
    queryFn: () => backend.listMemories(limit),
    staleTime: 30_000,
    retry: 2,
  })
}

/**
 * 语义检索记忆（搜索框输入时触发）。
 */
export function useMemorySearch(query: string, enabled = true) {
  return useQuery({
    queryKey: memoryKeys.search(query),
    queryFn: () => backend.recallMemories(query),
    enabled: enabled && query.trim().length > 0,
    staleTime: 15_000,
  })
}

/**
 * 手动写入一条长期记忆。
 */
export function useRememberMemory() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (content: string) => backend.rememberMemory(content),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: memoryKeys.all })
    },
  })
}

/**
 * 按 ID 删除记忆。
 */
export function useForgetMemory() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => backend.forgetMemory(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: memoryKeys.all })
    },
  })
}
