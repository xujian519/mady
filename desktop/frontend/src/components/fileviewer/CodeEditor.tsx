/**
 * CodeEditor — CodeMirror 6 封装（PilotDeck 风格）。
 *
 * - Markdown 语法高亮（@codemirror/lang-markdown）
 * - 行号 + 当前行高亮 + 自动折行
 * - Cmd/Ctrl+S 触发 onSave
 * - 跟随系统深浅色（prefers-color-scheme）
 */

import React, { useEffect, useRef } from 'react'
import { EditorState, Prec } from '@codemirror/state'
import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLine,
  highlightActiveLineGutter,
  drawSelection,
} from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import { markdown, markdownLanguage } from '@codemirror/lang-markdown'
import { syntaxHighlighting, HighlightStyle } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { oneDark } from '@codemirror/theme-one-dark'
import { useTheme } from '@/theme/tokens'

interface CodeEditorProps {
  value: string
  onChange: (text: string) => void
  onSave: () => void
  /** 是否 Markdown 模式（false 为纯文本）。 */
  markdown?: boolean
}

/** 浅色高亮：沿用默认高亮并微调标题/加粗。 */
const lightHighlight = HighlightStyle.define([
  { tag: tags.heading1, fontSize: '1.4em', fontWeight: '700' },
  { tag: tags.heading2, fontSize: '1.25em', fontWeight: '600' },
  { tag: tags.heading3, fontSize: '1.1em', fontWeight: '600' },
  { tag: tags.strong, fontWeight: '700' },
  { tag: tags.emphasis, fontStyle: 'italic' },
  { tag: tags.monospace, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' },
])

const baseTheme = EditorView.theme({
  '&': {
    height: '100%',
    fontSize: '12.5px',
    backgroundColor: 'transparent',
  },
  '.cm-content': {
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
    padding: '12px 0',
    caretColor: 'currentColor',
  },
  '.cm-scroller': {
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
    lineHeight: '1.6',
    overflow: 'auto',
  },
  '.cm-gutters': {
    backgroundColor: 'transparent',
    border: 'none',
    color: 'rgba(128,128,128,0.5)',
  },
  '&.cm-focused': { outline: 'none' },
})

export const CodeEditor: React.FC<CodeEditorProps> = ({ value, onChange, onSave, markdown: isMd }) => {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  // 用 ref 持有最新回调，避免重建 editor；每次 render 后经 effect 同步
  // （避免 render 期间写 ref，react-hooks/refs）
  const onChangeRef = useRef(onChange)
  const onSaveRef = useRef(onSave)
  useEffect(() => {
    onChangeRef.current = onChange
    onSaveRef.current = onSave
  })
  // F-I10：主题作为 effect 依赖——ThemeProvider 切换时重建编辑器高亮
  const { resolved } = useTheme()
  const isDark = resolved === 'dark'

  // 初始化 / 销毁
  useEffect(() => {
    if (!containerRef.current) return

    const extensions = [
      lineNumbers(),
      highlightActiveLine(),
      highlightActiveLineGutter(),
      drawSelection(),
      history(),
      Prec.highest(
        keymap.of([
          {
            key: 'Mod-s',
            run: () => {
              onSaveRef.current()
              return true
            },
          },
        ]),
      ),
      keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
      EditorView.lineWrapping,
      baseTheme,
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          onChangeRef.current(update.state.doc.toString())
        }
      }),
    ]

    if (isMd) {
      extensions.push(markdown({ base: markdownLanguage }))
    }

    if (isDark) {
      extensions.push(oneDark)
    } else {
      extensions.push(syntaxHighlighting(lightHighlight, { fallback: true }))
    }

    const state = EditorState.create({ doc: value, extensions })
    const view = new EditorView({ state, parent: containerRef.current })
    viewRef.current = view

    return () => {
      view.destroy()
      viewRef.current = null
    }
    // F-I10：isMd / isDark 均作为依赖——语言或主题切换时重建编辑器
    // value 由 updateListener 单向流出
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isMd, isDark])

  // 外部 value 变更（如打开新文件）时同步进 editor
  useEffect(() => {
    const view = viewRef.current
    if (!view) return
    const current = view.state.doc.toString()
    if (current !== value) {
      view.dispatch({ changes: { from: 0, to: current.length, insert: value } })
    }
  }, [value])

  return <div ref={containerRef} className="h-full overflow-hidden text-mady-text-primary" />
}
