/**
 * A2UI Video 组件 — 视频播放。
 *
 * Props:
 *   src: string | Dynamic — 视频 URL
 *   poster: string | Dynamic — 封面图
 *   controls: boolean — 是否显示控制条
 *   autoplay: boolean — 是否自动播放
 */

import { useMemo } from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'

export const VideoComponent: A2UIComponent = ({ component, context }) => {
  const resolved = useMemo(() => {
    const src = resolveDynamic(component.props.src, context.surface.dataModel, context.functions) as string | undefined
    const poster = resolveDynamic(component.props.poster, context.surface.dataModel, context.functions) as string | undefined
    return { src, poster }
  }, [component.props, context.surface.dataModel, context.functions])

  const controls = component.props.controls !== false
  const autoPlay = !!component.props.autoplay

  if (!resolved.src) return null

  return (
    <video
      src={resolved.src}
      poster={resolved.poster}
      controls={controls}
      autoPlay={autoPlay}
      className="rounded-md w-full max-h-96"
    >
      Your browser does not support the video element.
    </video>
  )
}
