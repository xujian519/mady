/**
 * 技能列表查询（TanStack Query）。
 *
 * 封装 App.ListSkills（技能发现路径扫描），供 SkillsView 使用
 * （见 mady-desktop-standards.md M-DSK-ST-002）。
 */
import { useQuery } from '@tanstack/react-query'
import { listSkills, type SkillEntry } from '@/lib/backend'

/** 技能列表 query key。 */
export const skillKeys = {
  all: ['skills'] as const,
}

/**
 * 查询可用技能列表。
 */
export function useSkills() {
  return useQuery({
    queryKey: skillKeys.all,
    queryFn: () => listSkills(),
    staleTime: 60_000,
    retry: 1,
  })
}

export type { SkillEntry }
