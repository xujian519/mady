/**
 * A2UI Modal 组件 — 模态弹窗。
 *
 * Props:
 *   open: boolean | Dynamic — 是否显示
 *   title: string | Dynamic — 标题
 *   child: string — 内容组件 ID
 *   entryPointChild: string — 触发组件 ID
 */

import { useMemo, useState } from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'

export const ModalComponent: A2UIComponent = ({ component, context, children }) => {
  const resolved = useMemo(() => {
    const title = resolveDynamic(component.props.title, context.surface.dataModel, context.functions) as string | undefined
    const open = !!(resolveDynamic(component.props.open, context.surface.dataModel, context.functions))
    return { title, open }
  }, [component.props, context.surface.dataModel, context.functions])

  const [localOpen, setLocalOpen] = useState(false)
  const isOpen = resolved.open || localOpen

  // children[0] = child (content), children[1] = entryPointChild (trigger)
  const childrenArr = Array.isArray(children) ? children : children ? [children] : []
  const trigger = childrenArr[1] ?? childrenArr[0]
  const content = childrenArr[0]

  return (
    <>
      {/* Trigger element */}
      {trigger && (
        <div onClick={() => setLocalOpen(true)}>
          {trigger}
        </div>
      )}

      {/* Modal overlay */}
      {isOpen && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center"
          onClick={() => setLocalOpen(false)}
        >
          <div className="fixed inset-0 bg-mady-bg-overlay backdrop-blur-sm" />
          <div
            role="dialog"
            aria-modal="true"
            className="relative bg-mady-bg-primary rounded-lg shadow-mady-modal max-w-lg w-full mx-4 max-h-[80vh] overflow-y-auto"
            onClick={(e) => e.stopPropagation()}
          >
            {resolved.title && (
              <div className="flex items-center justify-between px-4 py-3 border-b border-mady-separator">
                <h3 className="text-mady-text-primary font-semibold">{resolved.title}</h3>
                <button
                  className="text-mady-text-tertiary hover:text-mady-text-primary text-lg leading-none"
                  onClick={() => setLocalOpen(false)}
                >
                  ✕
                </button>
              </div>
            )}
            <div className="p-4">
              {content ?? children}
            </div>
          </div>
        </div>
      )}
    </>
  )
}
