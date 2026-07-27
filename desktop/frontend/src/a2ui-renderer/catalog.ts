/**
 * A2UI CatalogRegistry + BasicCatalog（对齐 Go a2ui/catalog.go）。
 *
 * 定义组件类型、函数名、主题属性的注册表。
 * BasicCatalog 覆盖 A2UI v0.9.1 的 18 个标准组件和 15 个内置函数。
 */

// ── 类型 ───────────────────────────────────────────

/** 单个组件类型的定义。 */
export interface ComponentDef {
  /** 组件类型名（如 "Text", "Button", "Column"）。 */
  name: string
  /** 持有单个子组件 ID 的属性名。 */
  childFields: string[]
  /** 持有子组件 ID 列表的属性名。 */
  childListFields: string[]
  /**
   * 嵌套子组件映射：属性名 → 子组件 ID 所在的键名。
   * 用于 Tabs 等组件，其 children 嵌套在结构化数组中。
   */
  nestedChildFields: Record<string, string>
  /** 是否为交互式输入组件（建立双向数据绑定）。 */
  input: boolean
}

/** A2UI Catalog。 */
export interface Catalog {
  /** 唯一 catalog ID（匹配 CreateSurface.CatalogID）。 */
  id: string
  /** 组件类型 → 组件定义。 */
  components: Record<string, ComponentDef>
  /** 已注册的函数名集合。 */
  functions: Set<string>
  /** 已识别的主题属性名集合。 */
  themeProperties: Set<string>
}

// ── 辅助函数 ───────────────────────────────────────

function _setOf(items: string[]): Set<string> {
  return new Set(items)
}

// ── BasicCatalog ───────────────────────────────────

export const BasicCatalogID =
  'https://a2ui.org/specification/v0_9_1/catalogs/basic/catalog.json'

/** 返回 A2UI v0.9.1 BasicCatalog。 */
export function basicCatalog(): Catalog {
  const funcs = [
    'required', 'regex', 'length', 'numeric', 'email',
    'formatString', 'formatNumber', 'formatCurrency', 'formatDate',
    'pluralize', 'openUrl', 'and', 'or', 'not',
  ]

  const themeProps = ['primaryColor', 'iconUrl', 'agentDisplayName']

  const defs: ComponentDef[] = [
    { name: 'Text', childFields: [], childListFields: [], nestedChildFields: {}, input: false },
    { name: 'Image', childFields: [], childListFields: [], nestedChildFields: {}, input: false },
    { name: 'Icon', childFields: [], childListFields: [], nestedChildFields: {}, input: false },
    { name: 'Video', childFields: [], childListFields: [], nestedChildFields: {}, input: false },
    { name: 'AudioPlayer', childFields: [], childListFields: [], nestedChildFields: {}, input: false },
    { name: 'Row', childFields: [], childListFields: ['children'], nestedChildFields: {}, input: false },
    { name: 'Column', childFields: [], childListFields: ['children'], nestedChildFields: {}, input: false },
    { name: 'List', childFields: [], childListFields: ['children'], nestedChildFields: {}, input: false },
    { name: 'Card', childFields: ['child'], childListFields: [], nestedChildFields: {}, input: false },
    { name: 'Tabs', childFields: [], childListFields: [], nestedChildFields: { tabs: 'child' }, input: false },
    { name: 'Divider', childFields: [], childListFields: [], nestedChildFields: {}, input: false },
    { name: 'Modal', childFields: ['child', 'entryPointChild'], childListFields: [], nestedChildFields: {}, input: false },
    { name: 'Button', childFields: ['child'], childListFields: [], nestedChildFields: {}, input: false },
    { name: 'CheckBox', childFields: [], childListFields: [], nestedChildFields: {}, input: true },
    { name: 'TextField', childFields: [], childListFields: [], nestedChildFields: {}, input: true },
    { name: 'DateTimeInput', childFields: [], childListFields: [], nestedChildFields: {}, input: true },
    { name: 'ChoicePicker', childFields: [], childListFields: [], nestedChildFields: {}, input: true },
    { name: 'Slider', childFields: [], childListFields: [], nestedChildFields: {}, input: true },
  ]

  const comps: Record<string, ComponentDef> = {}
  for (const d of defs) {
    comps[d.name] = d
  }

  return {
    id: BasicCatalogID,
    components: comps,
    functions: _setOf(funcs),
    themeProperties: _setOf(themeProps),
  }
}

// ── CatalogRegistry ────────────────────────────────

/** Catalog 注册表，支持按 ID 查找。 */
export class CatalogRegistry {
  private _catalogs = new Map<string, Catalog>()

  constructor() {
    this.register(basicCatalog())
  }

  /** 注册（或替换）一个 Catalog。 */
  register(cat: Catalog): void {
    this._catalogs.set(cat.id, cat)
  }

  /** 按 ID 查找 Catalog。 */
  lookup(id: string): Catalog | undefined {
    return this._catalogs.get(id)
  }
}
