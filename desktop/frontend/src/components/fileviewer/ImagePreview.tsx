/**
 * ImagePreview — 图片预览组件。
 *
 * 支持按钮缩放、滚轮缩放（Ctrl/Meta + 滚轮）和双击切换 100%/200%。
 */

import React, { useRef, useState } from 'react'
import { ZoomIn, ZoomOut } from 'lucide-react'

interface ImagePreviewProps {
  data: string
  mime: string
  name: string
}

export const ImagePreview: React.FC<ImagePreviewProps> = ({ data, mime, name }) => {
  const [zoom, setZoom] = useState(1)
  const prevZoomRef = useRef(1)

  return (
    <div className="relative h-full flex flex-col">
      <div className="absolute top-2 right-2 z-10 flex items-center gap-1 bg-mady-bg-primary/80 backdrop-blur rounded-lg border border-mady-separator px-1 py-0.5">
        <button
          onClick={() => setZoom((z) => Math.max(0.1, z - 0.25))}
          className="p-1 rounded hover:bg-mady-bg-secondary text-mady-text-secondary"
          title="缩小"
        >
          <ZoomOut size={13} />
        </button>
        <span className="text-mady-caption text-mady-text-secondary w-10 text-center">
          {Math.round(zoom * 100)}%
        </span>
        <button
          onClick={() => setZoom((z) => Math.min(8, z + 0.25))}
          className="p-1 rounded hover:bg-mady-bg-secondary text-mady-text-secondary"
          title="放大"
        >
          <ZoomIn size={13} />
        </button>
      </div>
      <div
        className="flex-1 overflow-auto flex items-center justify-center p-4"
        onWheel={(e) => {
          if (e.ctrlKey || e.metaKey) {
            e.preventDefault()
            setZoom((z) => Math.min(8, Math.max(0.1, z - e.deltaY * 0.002)))
          }
        }}
        onDoubleClick={() => {
          setZoom((z) => {
            if (Math.abs(z - 1) < 0.05) {
              prevZoomRef.current = 2
              return 2
            }
            prevZoomRef.current = z
            return 1
          })
        }}
      >
        <img
          src={`data:${mime};base64,${data}`}
          alt={name}
          style={{ transform: `scale(${zoom})`, transformOrigin: 'center center' }}
          className="max-w-full h-auto select-none transition-transform duration-100"
          draggable={false}
        />
      </div>
    </div>
  )
}
