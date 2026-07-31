/**
 * A2UI Zustand Store — SurfaceStore 的响应式封装。
 *
 * 提供:
 *   1. SurfaceStore 的单一实例引用
 *   2. applyEnvelope 操作（供 agui-bridge/reducer 调用）
 *   3. surface 状态快照（供 React 渲染器消费）
 *
 * 用法:
 *   import { useA2UIStore } from '@/a2ui-renderer/a2ui-store'
 *
 *   // 在 reducer 中：
 *   useA2UIStore.getState().applyEnvelope(payload)
 *
 *   // 在 React 组件中：
 *   const surfaces = useA2UIStore((s) => s.surfaces)
 */

import { create } from 'zustand'
import { SurfaceStore, type Envelope, type SurfaceState, type ClientDataModelPayload, envelopeSurfaceID, envelopeKind, SurfaceExistsError } from './store'
import { functionRegistry } from './functions'

interface A2UIState {
  /** 底层 SurfaceStore 实例。 */
  _store: SurfaceStore

  /** surface ID → SurfaceState 的快照（供 React 渲染）。 */
  surfaces: Record<string, SurfaceState>

  /** 函数注册表（供组件渲染时使用）。 */
  functions: Record<string, (args: Record<string, unknown>) => unknown>

  /** 应用一个服务端信封，更新 surface 状态。 */
  applyEnvelope: (env: Envelope) => void

  /** 收集所有 sendDataModel=true 的 data model。 */
  clientDataModel: () => ClientDataModelPayload

  /** 获取指定 surface。 */
  getSurface: (id: string) => SurfaceState | undefined
}

export const useA2UIStore = create<A2UIState>((set, get) => {
  const store = new SurfaceStore()

  return {
    _store: store,
    surfaces: {},
    functions: functionRegistry,

    applyEnvelope: (env: Envelope) => {
      // 本次信封作用的 surface ID（用于快照浅拷贝，见下）
      const targetId = envelopeSurfaceID(env)
      try {
        store.apply(env)
      } catch (err) {
        // F-I5 幂等：重复 createSurface（后端重发/重试）视为已存在、跳过。
        // 其余异常（如 payload 结构非法）向上抛，由 reducer 兜底记录。
        if (!(envelopeKind(env) === 'createSurface' && err instanceof SurfaceExistsError)) {
          throw err
        }
      }

      // 快照化：将 Map 转为普通对象供 React 响应式检测。
      // F-B2：受影响的 surface 必须浅拷贝出「新引用」——底层 _applyComponents/
      // _applyDataModel 是原地 mutate，若快照仍持有旧引用，React 的 useMemo/
      // React.memo 依赖比较将永远命中缓存，surface 更新后 UI 冻结。
      const snapshot: Record<string, SurfaceState> = {}
      for (const [id, srf] of store.surfaces) {
        snapshot[id] = id === targetId ? { ...srf } : srf
      }
      set({ surfaces: snapshot })
    },

    clientDataModel: () => {
      return store.clientDataModel()
    },

    getSurface: (id: string) => {
      return get().surfaces[id]
    },
  }
})
