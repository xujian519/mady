/**
 * A2UI 函数注册表（15 个内置函数的聚合导出）。
 *
 * 合并 formatFunctions + validationFunctions 为单一注册表，
 * 供 dynamic.ts 的 callFunction 使用。
 */

import { formatFunctions } from './format'
import { validationFunctions } from './validate'

/** 完整 A2UI BasicCatalog 函数注册表。 */
export const functionRegistry: Record<string, (args: Record<string, unknown>) => unknown> = {
  ...formatFunctions,
  ...validationFunctions,
}
