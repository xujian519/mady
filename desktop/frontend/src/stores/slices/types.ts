/**
 * Slices 模式共享类型工具。
 *
 * - FunctionKeys：提取接口中的函数字段名（actions）
 * - SliceState：由 slice 接口推导「纯状态字段」类型
 *
 * 用于各 slice 的 initialState 标注与组合入口 ChatState 推导，
 * 避免 slice 接口与状态默认值之间字段漂移。
 */
export type FunctionKeys<T> = {
  [K in keyof T]: T[K] extends (...args: never[]) => unknown ? K : never
}[keyof T]

/** 从 slice 接口中取出纯状态字段（剔除 actions）。 */
export type SliceState<T> = Omit<T, FunctionKeys<T>>
