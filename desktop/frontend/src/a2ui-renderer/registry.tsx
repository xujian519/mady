/**
 * A2UI 组件注册表。
 *
 * 映射 ComponentType → React 组件实现。
 * 未知组件类型渲染为占位组件。
 */

import React from 'react'
import type { Component, SurfaceState } from './store'
import { TextComponent } from './components/Text'
import { IconComponent } from './components/Icon'
import { RowComponent } from './components/Row'
import { ColumnComponent } from './components/Column'
import { ListComponent } from './components/List'
import { CardComponent } from './components/Card'
import { DividerComponent } from './components/Divider'
import { ButtonComponent } from './components/Button'
import { TextFieldComponent } from './components/TextField'
import { ImageComponent } from './components/Image'
import { TabsComponent } from './components/Tabs'
import { ModalComponent } from './components/Modal'
import { CheckBoxComponent } from './components/CheckBox'
import { ChoicePickerComponent } from './components/ChoicePicker'
import { VideoComponent } from './components/Video'
import { AudioPlayerComponent } from './components/AudioPlayer'
import { DateTimeInputComponent } from './components/DateTimeInput'
import { SliderComponent } from './components/Slider'

/** 组件渲染上下文。 */
export interface RenderContext {
  surface: SurfaceState
  /** 组件 resolveDynamic 所需的函数注册表。 */
  functions?: Record<string, (args: Record<string, unknown>) => unknown>
  /** 组件交互回调。 */
  onAction?: (surfaceId: string, sourceId: string, eventName: string, context?: Record<string, unknown>) => void
}

/** A2UI 组件渲染函数签名。 */
export type A2UIComponent = React.FC<{
  component: Component
  context: RenderContext
  children?: React.ReactNode
}>

/** 组件注册表。 */
const registry: Record<string, A2UIComponent> = {
  Text: TextComponent,
  Icon: IconComponent,
  Image: ImageComponent,
  Row: RowComponent,
  Column: ColumnComponent,
  List: ListComponent,
  Card: CardComponent,
  Divider: DividerComponent,
  Tabs: TabsComponent,
  Modal: ModalComponent,
  Button: ButtonComponent,
  TextField: TextFieldComponent,
  CheckBox: CheckBoxComponent,
  ChoicePicker: ChoicePickerComponent,
  Video: VideoComponent,
  AudioPlayer: AudioPlayerComponent,
  DateTimeInput: DateTimeInputComponent,
  Slider: SliderComponent,
}

/** 根据组件类型获取 React 组件。未知类型渲染占位。 */
export function getComponent(type: string): A2UIComponent {
  return registry[type] ?? UnknownComponent
}

/** 注册额外组件类型（供自定义 catalog 使用）。 */
export function registerComponent(type: string, comp: A2UIComponent): void {
  registry[type] = comp
}

/** 未知类型占位组件。 */
const UnknownComponent: A2UIComponent = ({ component }) => (
  <div
    className="a2ui-unknown border border-dashed border-mady-border rounded-md p-3 text-mady-text-tertiary text-mady-ui"
    data-a2ui-type={component.type}
    data-a2ui-id={component.id}
  >
    Unknown A2UI component: <code className="font-mono text-xs">{component.type}</code>
  </div>
)
