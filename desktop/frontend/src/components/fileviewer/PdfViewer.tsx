/**
 * PdfViewer — pdf.js 内嵌只读查看器（T5.5）。
 *
 * - 逐页渲染 canvas（分页 + 缩放）
 * - AnnotationLayer 批注层：highlight / underline / strikeout / ink / freeText / stamp 等只读渲染
 * - 页内批注列表：点击高亮定位（contents / 作者 / 日期）
 * - 安全：renderForms=false，不启用批注编辑（isEvalSupported 在 pdfjs 6.x 中默认禁用）
 * - cMap 已打包（build 时复制 cmaps/ 到 dist/cmaps/），支持 CJK PDF 字形映射
 */

import React, { useCallback, useEffect, useRef, useState } from 'react'
import * as pdfjsLib from 'pdfjs-dist'
import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import 'pdfjs-dist/web/pdf_viewer.css'
import { ChevronLeft, ChevronRight, ZoomIn, ZoomOut, Loader2, AlertCircle, MessageSquare } from 'lucide-react'

pdfjsLib.GlobalWorkerOptions.workerSrc = workerUrl

/** 批注摘要（用于侧栏列表）。 */
interface AnnotationSummary {
  id: string
  subtype: string
  contents: string
  author: string
  date: string
}

interface PdfViewerProps {
  /** base64 编码的 PDF 字节。 */
  data: string
  name: string
}

/** base64 → Uint8Array。 */
function base64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return bytes
}

/** 只读场景的 linkService 桩：外链新窗口打开，内部跳转忽略。 */
function makeLinkServiceStub() {
  return {
    goToDestination: async () => {},
    goToPage: async () => {},
    getDestinationHash: () => '#',
    getAnchorUrl: () => '#',
    addLinkAttributes: (anchor: HTMLAnchorElement, url: string) => {
      anchor.href = url
      anchor.target = '_blank'
      anchor.rel = 'noopener noreferrer'
    },
    executeNamedAction: () => {},
    executeSetOCGState: async () => {},
    setDocument: () => {},
    setViewer: () => {},
    setHistory: () => {},
    externalLinkEnabled: true,
    isInPresentationMode: false,
  }
}

export const PdfViewer: React.FC<PdfViewerProps> = ({ data, name }) => {  const [doc, setDoc] = useState<pdfjsLib.PDFDocumentProxy | null>(null)
  const [pageNum, setPageNum] = useState(1)
  const [numPages, setNumPages] = useState(0)
  const [scale, setScale] = useState(1.2)
  const [rendering, setRendering] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [annotations, setAnnotations] = useState<AnnotationSummary[]>([])

  const canvasRef = useRef<HTMLCanvasElement>(null)
  const annotDivRef = useRef<HTMLDivElement>(null)
  const renderSeqRef = useRef(0)

  // 加载文档
  useEffect(() => {
    let cancelled = false
    setLoadError(null)
    setDoc(null)

    const bytes = base64ToBytes(data)
    const task = pdfjsLib.getDocument({
      data: bytes,
      cMapUrl: '/cmaps/',
      cMapPacked: true,
    })

    task.promise
      .then((pdf) => {
        if (cancelled) return
        setDoc(pdf)
        setNumPages(pdf.numPages)
        setPageNum(1)
      })
      .catch((err: unknown) => {
        if (!cancelled) {
          setLoadError(err instanceof Error ? err.message : String(err))
        }
      })

    return () => {
      cancelled = true
      void task.destroy()
    }
  }, [data])

  // 渲染当前页 + 批注层
  const renderPage = useCallback(async () => {
    if (!doc || !canvasRef.current || !annotDivRef.current) return
    const seq = ++renderSeqRef.current
    setRendering(true)

    try {
      const page = await doc.getPage(pageNum)
      if (seq !== renderSeqRef.current) return

      const viewport = page.getViewport({ scale })
      const canvas = canvasRef.current
      const annotDiv = annotDivRef.current

      canvas.width = Math.floor(viewport.width)
      canvas.height = Math.floor(viewport.height)
      canvas.style.width = `${Math.floor(viewport.width)}px`
      canvas.style.height = `${Math.floor(viewport.height)}px`
      annotDiv.style.width = canvas.style.width
      annotDiv.style.height = canvas.style.height

      const ctx = canvas.getContext('2d')
      if (!ctx) return

      await page.render({ canvasContext: ctx, viewport, canvas }).promise
      if (seq !== renderSeqRef.current) return

      // 批注层
      annotDiv.replaceChildren()
      const annots = await page.getAnnotations()
      if (seq !== renderSeqRef.current) return

      if (annots.length > 0) {
        const layer = new pdfjsLib.AnnotationLayer({
          div: annotDiv,
          accessibilityManager: null,
          annotationCanvasMap: null,
          annotationEditorUIManager: null,
          page,
          viewport: viewport.clone({ dontFlip: true }),
          structTreeLayer: null,
          commentManager: null,
          linkService: makeLinkServiceStub(),
          annotationStorage: null,
        })
        await layer.render({
          viewport: viewport.clone({ dontFlip: true }),
          div: annotDiv,
          annotations: annots,
          page,
          linkService: makeLinkServiceStub(),
          renderForms: false,
        } as unknown as Parameters<pdfjsLib.AnnotationLayer['render']>[0])
      }

      // 批注摘要列表
      setAnnotations(
        annots
          .filter((a) => a.subtype !== 'Link' && (a.contents || a.titleObj?.str))
          .map((a, i) => ({
            id: a.id ?? `annot-${i}`,
            subtype: a.subtype ?? '',
            contents: a.contentsObj?.str ?? a.contents ?? '',
            author: a.titleObj?.str ?? '',
            date: a.modificationDate ?? a.creationDate ?? '',
          })),
      )
    } catch {
      // 渲染中断（快速翻页）静默忽略
    } finally {
      if (seq === renderSeqRef.current) setRendering(false)
    }
  }, [doc, pageNum, scale])

  useEffect(() => {
    void renderPage()
  }, [renderPage])

  return (
    <div className="h-full flex flex-col">
      {/* 工具栏 */}
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-mady-separator bg-mady-bg-secondary/30">
        <div className="flex items-center gap-1">
          <button
            onClick={() => setPageNum((p) => Math.max(1, p - 1))}
            disabled={pageNum <= 1}
            className="p-1 rounded hover:bg-mady-bg-secondary text-mady-text-secondary disabled:opacity-40"
            title="上一页"
          >
            <ChevronLeft size={14} />
          </button>
          <span className="text-mady-caption text-mady-text-secondary w-16 text-center">
            {pageNum} / {numPages}
          </span>
          <button
            onClick={() => setPageNum((p) => Math.min(numPages, p + 1))}
            disabled={pageNum >= numPages}
            className="p-1 rounded hover:bg-mady-bg-secondary text-mady-text-secondary disabled:opacity-40"
            title="下一页"
          >
            <ChevronRight size={14} />
          </button>
        </div>
        <div className="flex items-center gap-1">
          {rendering && <Loader2 size={12} className="animate-spin text-mady-text-tertiary" />}
          <button
            onClick={() => setScale((s) => Math.max(0.4, s - 0.2))}
            className="p-1 rounded hover:bg-mady-bg-secondary text-mady-text-secondary"
            title="缩小"
          >
            <ZoomOut size={13} />
          </button>
          <span className="text-mady-caption text-mady-text-secondary w-10 text-center">
            {Math.round(scale * 100)}%
          </span>
          <button
            onClick={() => setScale((s) => Math.min(4, s + 0.2))}
            className="p-1 rounded hover:bg-mady-bg-secondary text-mady-text-secondary"
            title="放大"
          >
            <ZoomIn size={13} />
          </button>
        </div>
      </div>

      {/* 页面区 */}
      <div className="flex-1 overflow-auto bg-mady-bg-secondary/20">
        {loadError ? (
          <div className="h-full flex flex-col items-center justify-center gap-2 px-6 text-center">
            <AlertCircle size={20} className="text-mady-warning" />
            <span className="text-mady-ui text-mady-text-secondary">PDF 加载失败：{name} — {loadError}</span>
          </div>
        ) : (
          <div className="inline-block m-4 relative shadow-lg bg-white" style={{ lineHeight: 0 }}>
            <canvas ref={canvasRef} className="block" />
            {/* 批注层：pdf_viewer.css 中的 .annotationLayer 样式生效 */}
            <div ref={annotDivRef} className="annotationLayer absolute inset-0" />
          </div>
        )}
      </div>

      {/* 批注列表 */}
      {annotations.length > 0 && (
        <div className="max-h-36 overflow-y-auto border-t border-mady-separator">
          <div className="px-3 py-1.5 text-mady-caption font-medium text-mady-text-secondary uppercase tracking-wider flex items-center gap-1">
            <MessageSquare size={11} />
            本页批注（{annotations.length}）
          </div>
          {annotations.map((a) => (
            <div key={a.id} className="px-3 py-1.5 border-t border-mady-separator/50">
              <div className="flex items-center gap-2 text-mady-caption text-mady-text-tertiary">
                <span className="px-1 rounded bg-mady-accent-soft text-mady-accent">{a.subtype}</span>
                {a.author && <span className="truncate">{a.author}</span>}
                {a.date && <span>{a.date}</span>}
              </div>
              {a.contents && (
                <p className="text-mady-small text-mady-text-primary mt-0.5 whitespace-pre-wrap">{a.contents}</p>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
