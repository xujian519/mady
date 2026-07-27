/**
 * A2UI 渲染器 — 统一导出。
 *
 * Phase 2 所有 A2UI 渲染器资产的入口。
 * 外部模块（agui-bridge / App.tsx）应通过此文件引用，
 * 而不是直接导入内部模块。
 */

export { SurfaceStore, envelopeKind, envelopeSurfaceID, surfaceRoot, RootComponentID, A2UI_VERSION } from './store'
export type { SurfaceState, Envelope, Component, CreateSurface, UpdateComponents, UpdateDataModel, DeleteSurface, ClientAction, ClientDataModelPayload, Action, ActionEvent, FunctionCall, ChildList, ChildTemplate } from './store'
export { componentFromFlat, parseChildList } from './store'
export { CatalogRegistry, basicCatalog, BasicCatalogID } from './catalog'
export type { Catalog, ComponentDef } from './catalog'
export { parsePointer, joinPointer, getData, applyUpdate } from './datamodel'
export { classifyDynamic, resolveDynamic, resolveBind, callFunction, clearBindCache } from './dynamic'
export type { DynamicKind, ClassifiedDynamic } from './dynamic'
export { functionRegistry } from './functions'
export { A2UISurface, resolveProps } from './renderer'
export type { A2UIRendererProps } from './renderer'
export { registerComponent, getComponent } from './registry'
export type { A2UIComponent, RenderContext } from './registry'
export { childRefsOf } from './child-refs'
export { resolveThemeTheme, themeToStyle, agentDisplayName, iconUrl } from './theme'
export { validateEnvelope, validateSurfaceTree } from './validate'
export { useA2UIStore } from './a2ui-store'
