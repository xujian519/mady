/**
 * 文档模板列表查询（TanStack Query）。
 *
 * 封装 App.ListDocTemplates（项目模板库），供 TemplatesView 使用
 * （见 mady-desktop-standards.md M-DSK-ST-002）。
 */
import { useQuery } from '@tanstack/react-query'
import { listDocTemplates, type DocTemplateEntry } from '@/lib/backend'

/** 模板列表 query key。 */
export const templateKeys = {
  all: ['templates'] as const,
}

/**
 * 查询文档模板列表。
 */
export function useDocTemplates() {
  return useQuery({
    queryKey: templateKeys.all,
    queryFn: () => listDocTemplates(),
    staleTime: 60_000,
    retry: 1,
  })
}

export type { DocTemplateEntry }
