/**
 * 会话管理的 TanStack Query hooks。
 *
 * 每个 hook 封装对后端 Wails Binding 的调用，提供缓存/重试/失效。
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as backend from '@/lib/backend'

// ── Query Keys ────────────────────────────────────

export const threadKeys = {
  all: ['threads'] as const,
  detail: (key: string) => ['threads', key] as const,
}

// ── Hooks ─────────────────────────────────────────

/**
 * 查询所有会话列表。
 *
 * 会话列表的真相源是 TanStack Query（server-state），不再同步进 Zustand
 * （M-DSK-ST-001：Query 管 server-state，Zustand 管 client-state）。
 * 挂载点：App.tsx（常驻，任何视图切换都不丢数据）。
 */
export function useThreads() {
  return useQuery({
    queryKey: threadKeys.all,
    queryFn: () => backend.listThreads(),
    staleTime: 30_000,
    retry: 2,
  })
}

/**
 * 查询单个会话详情。
 */
export function useThreadDetail(key: string | null) {
  return useQuery({
    queryKey: threadKeys.detail(key ?? ''),
    queryFn: () => backend.getThread(key!),
    enabled: !!key,
    staleTime: 10_000,
  })
}

/**
 * 删除会话。
 */
export function useDeleteThread() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (key: string) => backend.deleteThread(key),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: threadKeys.all })
    },
  })
}

/**
 * 健康检查查询。
 * @param enabled 是否启用查询（ready 前不拉取）
 */
export function useHealth(enabled = true) {
  return useQuery({
    queryKey: ['health'],
    queryFn: () => backend.health(),
    enabled,
    staleTime: 60_000,
    retry: 1,
  })
}
