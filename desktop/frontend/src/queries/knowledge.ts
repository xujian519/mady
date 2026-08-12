/**
 * 知识库状态查询（TanStack Query）。
 *
 * 封装 App.GetKnowledgeStatus（知识库索引状态），
 * 供 KnowledgeView 使用（见 mady-desktop-standards.md M-DSK-ST-002）。
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getKnowledgeModelSettings,
  getKnowledgeStatus,
  getOmlxServiceStatus,
  setKnowledgeModelSettings,
  type KnowledgeModelSettings,
  type KnowledgeStatus,
  type OmlxServiceStatus,
} from '@/lib/backend'

/** 知识库状态 query key。 */
export const knowledgeKeys = {
  all: ['knowledge'] as const,
  status: ['knowledge', 'status'] as const,
  models: ['knowledge', 'models'] as const,
  omlx: ['knowledge', 'omlx'] as const,
}

/**
 * 查询知识库索引状态。
 * @param enabled 是否启用查询（默认 true）
 */
export function useKnowledgeStatus(enabled = true) {
  return useQuery({
    queryKey: knowledgeKeys.status,
    queryFn: () => getKnowledgeStatus(),
    enabled,
    staleTime: 30_000,
    retry: 1,
  })
}

/**
 * 查询知识库嵌入/Rerank 模型设置（含默认值合并，apiKey 为掩码）。
 */
export function useKnowledgeModelSettings() {
  return useQuery({
    queryKey: knowledgeKeys.models,
    queryFn: () => getKnowledgeModelSettings(),
    staleTime: 30_000,
    retry: 1,
  })
}

/**
 * 保存知识库嵌入/Rerank 模型设置。成功后失效 models 缓存，
 * 下次打开面板读到最新配置。
 */
export function useSaveKnowledgeModelSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (s: KnowledgeModelSettings) => setKnowledgeModelSettings(s),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: knowledgeKeys.models })
    },
  })
}

/**
 * 查询本地 oMLX 推理服务状态（嵌入/Rerank 依赖）。
 */
export function useOmlxServiceStatus() {
  return useQuery({
    queryKey: knowledgeKeys.omlx,
    queryFn: () => getOmlxServiceStatus(),
    staleTime: 15_000,
    retry: 1,
  })
}

export type { KnowledgeModelSettings, KnowledgeStatus, OmlxServiceStatus }
