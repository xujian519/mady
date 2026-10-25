package util

import "regexp"

// 常见凭证形态的脱敏规则。只匹配有明确前缀或 key/value 结构的片段，
// 避免误伤普通文本。
var (
	reSecretToken = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}\b`)
	reBearerToken = regexp.MustCompile(`(?i)\b(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`)
	reAPIKeyValue = regexp.MustCompile(`(?i)\b((?:api[ _-]?key|authorization)\s*[:=]\s*)\S+`)
)

// RedactSecrets 将字符串中的常见凭证形态替换为 [REDACTED]。
//
// 上游供应商的错误响应可能回显密钥片段（例如 401 中的
// "Your api key: ****7c08"）。任何面向用户或日志的展示路径都应先经过
// 本函数，避免把凭证扩散到终端、报告或审计记录。
func RedactSecrets(s string) string {
	s = reSecretToken.ReplaceAllString(s, "[REDACTED]")
	s = reBearerToken.ReplaceAllString(s, "$1[REDACTED]")
	s = reAPIKeyValue.ReplaceAllString(s, "$1[REDACTED]")
	return s
}
