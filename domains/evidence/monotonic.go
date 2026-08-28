// monotonic.go 实现证据形式要件的单调拒绝检查（DSH EVI-011 引入）。
//
// 规则（fail-closed）：域外证据未经公证认证、外文证据无译本时，直接拒绝
// 证据判断调用——宁可不判断，也不对形式要件缺失的证据出具结论。该检查以
// permission.DenyCheck 形式接入单调拒绝层，凌驾于任何 allow 配置之上；
// 拒绝理由包含补救指引，模型补齐形式要件声明后可重试。
//
// 与 DSH 的差异说明：DSH 从规则资产 YAML 派生deny 集合；本实现将阈值
// 硬编码在本文件（形式要件词表），词表即权威源——修改须经人工审阅。

package evidence

import (
	"encoding/json"
	"strings"

	"github.com/xujian519/mady/agentcore/permission"
)

// guardedTools 是形式要件检查覆盖的证据判断工具。
var guardedTools = []string{TypeSpecificToolName}

// notarizationConfirmed / translationConfirmed 是形式要件"已补齐"的声明值
// （大小写不敏感、去空白后比较）。
var (
	notarizationConfirmed = map[string]bool{
		"completed": true, "done": true, "yes": true, "true": true,
		"已认证": true, "已公证": true, "已公证认证": true, "完成": true,
	}
	translationConfirmed = map[string]bool{
		"completed": true, "translated": true, "yes": true, "true": true,
		"已翻译": true, "有译本": true, "已提供译本": true, "完成": true,
	}
)

// formArgs 是证据判断工具中与形式要件相关的参数约定。
type formArgs struct {
	EvidenceTypeHint   string `json:"evidence_type_hint"`
	NotarizationStatus string `json:"notarization_status"`
	TranslationStatus  string `json:"translation_status"`
}

// FormRequirementDenyCheck 返回证据形式要件的单调拒绝检查：
//   - evidence_type_hint=overseas（域外证据）且未声明公证认证 → 拒绝；
//   - evidence_type_hint=foreign_language（外文证据）且未声明译本 → 拒绝。
//
// 未声明证据类型时不拦截（引擎自行推断类型，形式要件结论随判断输出）。
func FormRequirementDenyCheck() permission.DenyCheck {
	return func(toolName string, args json.RawMessage) (string, bool) {
		guarded := false
		for _, n := range guardedTools {
			if strings.EqualFold(n, toolName) {
				guarded = true
				break
			}
		}
		if !guarded || len(args) == 0 {
			return "", false
		}

		var p formArgs
		if err := json.Unmarshal(args, &p); err != nil {
			return "", false // 参数损坏交给工具自身的参数校验，此处不重复报错
		}

		hint := strings.ToLower(strings.TrimSpace(p.EvidenceTypeHint))
		switch hint {
		case "overseas":
			if !notarizationConfirmed[strings.ToLower(strings.TrimSpace(p.NotarizationStatus))] {
				return "EVI-011 形式要件缺失：域外证据须先公证认证。请在 notarization_status 声明认证状态" +
					"（如 completed/已认证）后重试；无法取得认证的，改用其他证据形式", true
			}
		case "foreign_language":
			if !translationConfirmed[strings.ToLower(strings.TrimSpace(p.TranslationStatus))] {
				return "EVI-011 形式要件缺失：外文证据须提供中文译本。请在 translation_status 声明译本状态" +
					"（如 completed/有译本）后重试", true
			}
		}
		return "", false
	}
}
