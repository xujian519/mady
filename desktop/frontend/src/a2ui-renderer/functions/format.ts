/**
 * A2UI 格式化函数集。
 *
 * 15 个内置函数中的 5 个：formatString / formatNumber / formatCurrency / formatDate / pluralize。
 * 所有函数以 Record<string, (args: Record<string, unknown>) => unknown> 导出。
 */

/** 格式化字符串模板。用 `{n}` 占位。 */
function _formatString(args: Record<string, unknown>): unknown {
  const template = args.template as string | undefined
  if (template === undefined) return ''
  return template.replace(/\{(\d+)\}/g, (_, idx) => {
    const v = args[`arg${idx}`] ?? args[String(idx)]
    return v !== undefined ? String(v) : ''
  })
}

/** 数字格式化。 */
function _formatNumber(args: Record<string, unknown>): unknown {
  const value = args.value as number | undefined
  if (value === undefined) return ''
  const decimals = args.decimals as number | undefined
  if (decimals !== undefined) {
    return value.toFixed(decimals)
  }
  return value.toLocaleString()
}

/** 货币格式化。 */
function _formatCurrency(args: Record<string, unknown>): unknown {
  const value = args.value as number | undefined
  if (value === undefined) return ''
  const currency = (args.currency as string) ?? 'CNY'
  try {
    return new Intl.NumberFormat('zh-CN', {
      style: 'currency',
      currency,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value)
  } catch {
    return `${currency} ${value.toFixed(2)}`
  }
}

/** 日期格式化。 */
function _formatDate(args: Record<string, unknown>): unknown {
  const value = args.value as string | undefined
  if (!value) return ''
  const fmt = (args.format as string) ?? 'YYYY-MM-DD'
  try {
    const d = new Date(value)
    if (isNaN(d.getTime())) return value
    const pad = (n: number) => String(n).padStart(2, '0')
    return fmt
      .replace('YYYY', String(d.getFullYear()))
      .replace('MM', pad(d.getMonth() + 1))
      .replace('DD', pad(d.getDate()))
      .replace('HH', pad(d.getHours()))
      .replace('mm', pad(d.getMinutes()))
      .replace('ss', pad(d.getSeconds()))
  } catch {
    return value
  }
}

/** 复数/数量格式化。 */
function _pluralize(args: Record<string, unknown>): unknown {
  const count = args.count as number | undefined
  if (count === undefined) return ''
  const one = (args.one as string) ?? ''
  const other = (args.other as string) ?? ''
  return count === 1 ? one : other
}

/** 格式化函数注册表。 */
export const formatFunctions: Record<string, (args: Record<string, unknown>) => unknown> = {
  formatString: _formatString,
  formatNumber: _formatNumber,
  formatCurrency: _formatCurrency,
  formatDate: _formatDate,
  pluralize: _pluralize,
}
