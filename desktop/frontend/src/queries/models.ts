/**
 * 模型列表查询（TanStack Query）。
 *
 * 封装 App.ListModels（agentconfig 聚合的可用模型列表），
 * 提供缓存 / 重试 / 失效。供 ModelSettings 使用
 * （见 mady-desktop-standards.md M-DSK-ST-002）。
 */
import { useQuery } from '@tanstack/react-query'
import { listModels, type ModelInfo } from '@/lib/backend'

/** 模型列表 query key。 */
export const modelKeys = {
  all: ['models'] as const,
}

/**
 * 查询可用模型列表。
 * @param enabled 是否启用查询（默认 true）
 */
export function useModels(enabled = true) {
  return useQuery({
    queryKey: modelKeys.all,
    queryFn: () => listModels(),
    enabled,
    staleTime: 60_000,
    retry: 1,
  })
}

/** 模型列表数据类型（供组件转换时引用）。 */
export type { ModelInfo }
