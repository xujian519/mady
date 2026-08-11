package evidence

import _ "embed"

//go:embed data/evidence-rules.yaml
var evidenceRulesYAML []byte

// EvidenceRulesYAML 返回内嵌的证据规则 YAML 内容（数据随引擎包走，避免
// 跨包寄生：此前数据文件位于 domains/rules/data/rules/ 下、由该包导出，
// 与解析器（本包）错位，已归位到 domains/evidence/data/）。
// 为空时返回 nil。
func EvidenceRulesYAML() []byte {
	return evidenceRulesYAML
}
