/**
 * MarkdownPreview — Markdown 预览组件。
 *
 * 包装 MarkdownRenderer，提供滚动容器，供 FileViewerOverlay
 * 在编辑/预览双模式中用作预览面。
 */

import React from 'react'
import { MarkdownRenderer } from '@/components/MarkdownRenderer'

interface MarkdownPreviewProps {
  content: string
}

/** Markdown 预览：带滚动容器包装 MarkdownRenderer。 */
export const MarkdownPreview: React.FC<MarkdownPreviewProps> = ({ content }) => (
  <div className="h-full overflow-y-auto p-4">
    <MarkdownRenderer content={content} />
  </div>
)
