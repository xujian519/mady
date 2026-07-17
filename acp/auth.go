package acp

// 本文件提供内置的 ACP 认证实现：静态令牌认证（TokenAuthProvider）。
// 适用于本地优先部署中需要 basic 访问控制的场景（如通过环境变量
// MADY_ACP_TOKEN 为 `mady acp` 启用令牌校验）。

import (
	"context"
	"crypto/subtle"
	"fmt"
)

// TokenAuthMethodID 是静态令牌认证方式的 methodId。
const TokenAuthMethodID = "token"

// TokenAuthProvider 基于静态令牌实现 AuthProvider。
// 客户端在 authenticate 请求中以 methodId="token" + token 字段提交令牌，
// 服务端使用常量时间比较校验，避免时序侧信道。
type TokenAuthProvider struct {
	token string
}

// NewTokenAuthProvider 创建一个静态令牌认证提供者。token 为空时
// Authenticate 将始终失败（fail-closed）。
func NewTokenAuthProvider(token string) *TokenAuthProvider {
	return &TokenAuthProvider{token: token}
}

// AuthMethods 声明 "token" 认证方式，随 initialize 响应告知客户端。
func (p *TokenAuthProvider) AuthMethods() []any {
	return []any{
		AuthMethodAgent{
			ID:          TokenAuthMethodID,
			Name:        "Token",
			Description: "Static token authentication (MADY_ACP_TOKEN)",
		},
	}
}

// Authenticate 校验客户端提交的令牌。methodId 必须为 "token"，
// 令牌不匹配或为空时报错（认证失败）。
func (p *TokenAuthProvider) Authenticate(_ context.Context, params AuthenticateParams) (*AuthenticateResult, error) {
	if params.MethodID != TokenAuthMethodID {
		return nil, fmt.Errorf("unsupported auth method %q", params.MethodID)
	}
	if p.token == "" || subtle.ConstantTimeCompare([]byte(params.Token), []byte(p.token)) != 1 {
		return nil, fmt.Errorf("invalid token")
	}
	return &AuthenticateResult{}, nil
}
