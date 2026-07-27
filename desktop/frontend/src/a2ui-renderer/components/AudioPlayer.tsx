/**
 * A2UI AudioPlayer 组件 — 音频播放。
 *
 * Props:
 *   src: string | Dynamic — 音频 URL
 *   title: string | Dynamic — 标题
 *   controls: boolean — 是否显示控制条
 */

import { useMemo } from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'

export const AudioPlayerComponent: A2UIComponent = ({ component, context }) => {
  const resolved = useMemo(() => {
    const src = resolveDynamic(component.props.src, context.surface.dataModel, context.functions) as string | undefined
    const title = resolveDynamic(component.props.title, context.surface.dataModel, context.functions) as string | undefined
    return { src, title }
  }, [component.props, context.surface.dataModel, context.functions])

  const controls = component.props.controls !== false

  if (!resolved.src) return null

  return (
    <div className="flex items-center gap-3 bg-mady-bg-secondary rounded-md p-3">
      {resolved.title && (
        <span className="text-mady-text-secondary text-mady-ui shrink-0 max-w-[120px] truncate">
          {resolved.title}
        </span>
      )}
      <audio
        src={resolved.src}
        controls={controls}
        className="flex-1 h-8"
      >
        Your browser does not support the audio element.
      </audio>
    </div>
  )
}
