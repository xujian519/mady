/**
 * 会话管理的 TanStack Query hooks。
 *
 * 每个 hook 封装对后端 Wails Binding 的调用，提供缓存/重试/失效。
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useChatStore } from '@/stores/chat'
import * as backend from '@/lib/backend'

// ── Query Keys ────────────────────────────────────

export const threadKeys = {
  all: ['threads'] as const,
  detail: (key: string) => ['threads', key] as const,
}

// ── Hooks ─────────────────────────────────────────

/**
 * 查询所有会话列表。
 */
export function useThreads() {
  const setThreads = useChatStore((s) => s.setThreads)

  return useQuery({
    queryKey: threadKeys.all,
    queryFn: async () => {
      const threads = await backend.listThreads()
      setThreads(threads.map((t) => ({
        key: t.key,
        title: t.title,
        updatedAt: t.updatedAt,
        messageN: t.messageN,
      })))
      return threads
    },
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
 */
export function useHealth() {
  return useQuery({
    queryKey: ['health'],
    queryFn: () => backend.health(),
    staleTime: 60_000,
    retry: 1,
  })
}
