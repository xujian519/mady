/**
 * 通用工具。
 */

import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

/**
 * 合并 className（M-DSK-TW-004）：clsx 组合条件类 + tailwind-merge
 * 去重冲突的 Tailwind 类（如 bg-a/bg-b 只保留后者）。
 * 新增组件请使用本函数替代手写模板字符串拼接。
 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}
