/**
 * 知识库状态查询（TanStack Query）。
 *
 * 封装 App.GetKnowledgeStatus（知识库索引状态），
 * 供 KnowledgeView 使用（见 mady-desktop-standards.md M-DSK-ST-002）。
 */
import { useQuery } from '@tanstack/react-query'
import { getKnowledgeStatus, type KnowledgeStatus } from '@/lib/backend'

/** 知识库状态 query key。 */
export const knowledgeKeys = {
  all: ['knowledge'] as const,
}

/**
 * 查询知识库索引状态。
 * @param enabled 是否启用查询（默认 true）
 */
export function useKnowledgeStatus(enabled = true) {
  return useQuery({
    queryKey: knowledgeKeys.all,
    queryFn: () => getKnowledgeStatus(),
    enabled,
    staleTime: 30_000,
    retry: 1,
  })
}

export type { KnowledgeStatus }
