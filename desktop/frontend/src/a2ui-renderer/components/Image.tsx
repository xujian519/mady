/**
 * A2UI Image 组件 — 图片（Lazy loading）。
 *
 * Props:
 *   src: string | Dynamic — 图片 URL
 *   alt: string | Dynamic — 替代文本
 *   width: number | Dynamic — 宽度（可选）
 *   height: number | Dynamic — 高度（可选）
 *   fit: "cover" | "contain" | "fill"
 */

import { useMemo } from 'react'
import { resolveDynamic } from '../dynamic'
import type { A2UIComponent } from '../registry'

export const ImageComponent: A2UIComponent = ({ component, context }) => {
  const resolved = useMemo(() => {
    const src = resolveDynamic(component.props.src, context.surface.dataModel, context.functions) as string | undefined
    const alt = resolveDynamic(component.props.alt, context.surface.dataModel, context.functions) as string | undefined
    const width = resolveDynamic(component.props.width, context.surface.dataModel, context.functions) as number | undefined
    const height = resolveDynamic(component.props.height, context.surface.dataModel, context.functions) as number | undefined
    const fit = (component.props.fit as string) ?? 'cover'
    return { src, alt, width, height, fit }
  }, [component.props, context.surface.dataModel, context.functions])

  if (!resolved.src) {
    return <div className="bg-mady-bg-secondary rounded-md w-full h-32 flex items-center justify-center text-mady-text-tertiary text-mady-ui">No image</div>
  }

  return (
    <img
      src={resolved.src}
      alt={resolved.alt ?? ''}
      width={resolved.width}
      height={resolved.height}
      loading="lazy"
      className="rounded-md"
      style={{ objectFit: resolved.fit as 'cover' | 'contain' | 'fill' }}
    />
  )
}
