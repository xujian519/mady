package domains

import "github.com/xujian519/mady/domains/audit"

// 审计能力的实现位于 domains/audit（子包），本文件为 domains 父包提供
// 类型别名与函数转发，保持既有调用方（audit_extension.go）的包内引用
// 不变。类型别名下 AuditLogger/Encryptor 的全部方法对调用方直接可用，
// 无需逐一转发。

// AuditAction 审计动作类型（见 audit.AuditAction）。
type AuditAction = audit.AuditAction

// Audit action constants.
const (
	AuditAccess       AuditAction = audit.AuditAccess       // 查看案件数据
	AuditModify       AuditAction = audit.AuditModify       // 修改案件数据
	AuditExport       AuditAction = audit.AuditExport       // 导出/下载
	AuditDelete       AuditAction = audit.AuditDelete       // 删除案件数据
	AuditApprove      AuditAction = audit.AuditApprove      // 审批通过
	AuditReject       AuditAction = audit.AuditReject       // 审批拒绝
	AuditLogin        AuditAction = audit.AuditLogin        // 用户登录
	AuditConfigChange AuditAction = audit.AuditConfigChange // 配置变更
)

// AuditEntry 审计条目（见 audit.AuditEntry）。
type AuditEntry = audit.AuditEntry

// AuditLogger 审计日志器（见 audit.AuditLogger）。
type AuditLogger = audit.AuditLogger

// DataRetentionConfig 数据保留策略配置（见 audit.DataRetentionConfig）。
type DataRetentionConfig = audit.DataRetentionConfig

// Encryptor 加密器（见 audit.Encryptor）。
type Encryptor = audit.Encryptor

// NewAuditLogger 创建审计日志器（见 audit.NewAuditLogger）。
func NewAuditLogger(dir string) (*AuditLogger, error) {
	return audit.NewAuditLogger(dir)
}

// DefaultRetentionConfig 返回标准保留策略（见 audit.DefaultRetentionConfig）。
func DefaultRetentionConfig() DataRetentionConfig {
	return audit.DefaultRetentionConfig()
}

// NewEncryptor 创建加密器（见 audit.NewEncryptor）。
func NewEncryptor() *Encryptor {
	return audit.NewEncryptor()
}
