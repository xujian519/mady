package core

// link.go — 受信任超链接（OSC 8）元数据。
//
// 安全模型：LLM 原始输出中的 OSC 8 / 伪装 APC 一律走 Raw fallback +
// SanitizeRawContent 剥离（见 sanitize.go，严格白名单）。本文件的链接通道
// 只对"显式实现 LinkProvider 的组件"开放：组件在渲染文本之外，将链接的
// 列区间与 URL 作为元数据直接提供，ParseLine 不解析任何链接标记——恶意
// 内容无法伪造，安全边界不变。

// LinkSpan 记录一行内一段超链接的可见列区间 [StartCol, EndCol)（半开，
// 按终端可见列计数，宽字符占 2 列）与目标 URL。
//
// 单行内的多个 LinkSpan 必须按 StartCol 升序排列且互不重叠（SerializeRow
// 按该顺序消费）；链接不跨行——每行独立提供自己的 LinkSpan。
type LinkSpan struct {
	StartCol, EndCol int64
	URL              string
}

// LinkProvider 是可选接口：组件可显式提供与 Render(width) 输出行一一对应
// 的链接元数据。返回的 [][]LinkSpan 长度应与渲染行数一致（多余/缺失的行
// 忽略）；每行内的链接须满足 LinkSpan 的排序/不重叠前置条件。返回 nil
// 表示无链接。
//
// 实现此接口的组件输出在 SerializeRow 时按 LinkSpan 注入 OSC 8 超链接；
// 未实现的组件（含 LLM 原始文本的 Markdown 组件）不产生任何链接。
//
// width 参数仅供实现参考（如按宽度决定是否生成链接），实现可忽略它——
// 引擎按 Render 的行数对齐元数据，不依赖 width 的语义。
type LinkProvider interface {
	RenderLinks(width int64) [][]LinkSpan
}

// LinkSpanFor 返回从 prefix 末尾开始、恰好覆盖 text 的链接跨度。
// prefix/text 可含 ANSI 样式（VisibleWidth 透明剥离），宽字符按 2 列计数。
// 组件用它为结构化文本（如证据来源）构造链接，无需手工数列。
func LinkSpanFor(prefix, text, url string) LinkSpan {
	start := VisibleWidth(prefix)
	return LinkSpan{StartCol: start, EndCol: start + VisibleWidth(text), URL: url}
}

// osc8Enabled 控制 SerializeRow 是否输出 OSC 8 序列。
// 终端不支持 OSC 8 时关闭，链接退化为纯文本（无下划线/可点击性）。
var osc8Enabled = true

// SetOSC8Enabled 开关 OSC 8 输出（终端能力检测时调用；默认开启）。
// 关闭后 LinkSpan 元数据仍被解析，但序列化时不注入 OSC 8。
func SetOSC8Enabled(enabled bool) { osc8Enabled = enabled }

// OSC8Enabled 报告 OSC 8 输出是否开启。
func OSC8Enabled() bool { return osc8Enabled }

// linksEqual 比较两组链接元数据是否一致（RowsEqual 用）。
func linksEqual(a, b []LinkSpan) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
