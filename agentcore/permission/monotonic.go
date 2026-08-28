// monotonic.go 实现单调拒绝层（DSH EVI-011 单调 deny 模式引入）。
//
// 单调拒绝是凌驾于 Policy 全部规则（Deny/Ask/Allow）与回退逻辑之上的
// 硬拒绝底线：一旦命中，任何 allow 配置都无法覆盖。它与 Policy.Deny 的
// 区别在于语义定位——Deny 是可随策略配置调整的普通规则，单调拒绝表达的是
// "无论策略如何配置都不允许通过"的安全/专业底线（如证据形式要件）。
//
// 两种形态：
//   - MonotonicDeny []Rule：工具/specifier 粒度的规则型硬拒绝；
//   - MonotonicDenyFns []DenyCheck：参数语义粒度的谓词型硬拒绝
//     （如"域外证据未公证认证/外文证据无译本时拒绝证据判断调用"）。
//
// 约定：DenyCheck 必须只读、无副作用、快速返回；deny 理由应包含可操作的
// 补救指引，让模型知道如何补齐形式要件后重试。

package permission

import (
	"encoding/json"
)

// DenyCheck 是单调拒绝谓词：ok 为 true 表示命中硬拒绝，reason 说明拒绝理由
// 与补救方式。谓词在 Policy 规则评估之前执行，命中即 Deny，不可被覆盖。
type DenyCheck func(toolName string, args json.RawMessage) (reason string, ok bool)

// monotonicDeny 汇总两种形态的命中结果；无命中返回空串。
func (p Policy) monotonicDeny(toolName string, args json.RawMessage) string {
	for _, r := range p.MonotonicDeny {
		if r.Matches(toolName, args) {
			return "命中单调拒绝规则: " + r.Tool
		}
	}
	for _, fn := range p.MonotonicDenyFns {
		if reason, ok := fn(toolName, args); ok {
			return reason
		}
	}
	return ""
}
