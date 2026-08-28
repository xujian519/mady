// Package invariant 提供包自含的运行时不变量自检框架（DSH
// runtime-diagnostics/invariants 模式引入）。
//
// 每个核心包可以发布自己的"不变量 companion"：校验本包拥有的持久关系
// （如状态迁移表闭合性、注册表完整性、事件配对性）。检查通过 Register
// 在包 init 时注册到全局注册表，宿主在启动后调用 RunAll 统一执行。
//
// 设计约定：
//   - 检查必须是只读、快速、无副作用的（不落盘、不发网络请求、不加锁竞争）；
//   - 注册表默认启用，RunAll 返回全部违规，由宿主决定日志级别；
//   - 环境变量 MADY_INVARIANTS_DISABLE 提供正则豁免（匹配 Package 或
//     Package/Name），豁免必须显式声明——禁止先违规后补豁免；
//   - 失败报告带归属包名，定位责任包而非笼统报错。
//
// 没有"可检查关系"的包不注册任何检查即是合规状态；注册检查的包应保证
// 检查本身的正确性（检查出错与不变量被破坏同样严重）。
package invariant

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

// Check 是一个具名不变量检查。
type Check struct {
	// Package 是归属包名（如 "agentcore/plantask"），用于失败归责与豁免匹配。
	Package string
	// Name 是检查名（包内唯一），与 Package 一起构成全局限定符 Package/Name。
	Name string
	// Fn 执行检查；返回非 nil error 表示不变量被破坏。
	Fn func() error
}

// Violation 是一次不变量违规。
type Violation struct {
	Check Check
	Err   error
}

// Error 实现 error，报告归属包与检查名。
func (v Violation) Error() string {
	return fmt.Sprintf("invariant %s/%s: %v", v.Check.Package, v.Check.Name, v.Err)
}

// Unwrap 暴露底层错误供 errors.As/Is 使用。
func (v Violation) Unwrap() error { return v.Err }

// Registry 是不变量检查注册表（线程安全）。
type Registry struct {
	mu     sync.RWMutex
	checks []Check
}

var global = &Registry{}

// Register 把检查注册到全局注册表（通常在包 init 中调用）。
func Register(c Check) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.checks = append(global.checks, c)
}

// Checks 返回已注册检查的快照。
func Checks() []Check {
	global.mu.RLock()
	defer global.mu.RUnlock()
	out := make([]Check, len(global.checks))
	copy(out, global.checks)
	return out
}

// RunAll 执行全部注册检查，返回违规列表（顺序与注册顺序一致）。
// MADY_INVARIANTS_DISABLE 中的正则（按 | 分隔多个）命中 Package 或
// Package/Name 时跳过该检查；正则编译失败本身作为违规上报——
// 豁免配置错误不静默放行。
func RunAll() []Violation {
	global.mu.RLock()
	checks := make([]Check, len(global.checks))
	copy(checks, global.checks)
	global.mu.RUnlock()

	skip, parseErr := parseDisablePatterns()
	var violations []Violation
	if parseErr != nil {
		violations = append(violations, Violation{
			Check: Check{Package: "invariant", Name: "disable-patterns"},
			Err:   parseErr,
		})
	}
	for _, c := range checks {
		if skip != nil && skip.matches(c) {
			continue
		}
		if err := c.Fn(); err != nil {
			violations = append(violations, Violation{Check: c, Err: err})
		}
	}
	return violations
}

// disableList 是豁免正则集合。
type disableList []*regexp.Regexp

func (d disableList) matches(c Check) bool {
	qualified := c.Package + "/" + c.Name
	for _, re := range d {
		if re.MatchString(c.Package) || re.MatchString(qualified) {
			return true
		}
	}
	return false
}

// parseDisablePatterns 解析 MADY_INVARIANTS_DISABLE（按 | 分隔）。
// 存在无法编译的模式时返回 error（RunAll 会将其作为违规上报）。
func parseDisablePatterns() (disableList, error) {
	raw := os.Getenv("MADY_INVARIANTS_DISABLE")
	if raw == "" {
		return nil, nil
	}
	var list disableList
	for _, part := range strings.Split(raw, "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		re, err := regexp.Compile(part)
		if err != nil {
			return list, fmt.Errorf("MADY_INVARIANTS_DISABLE 模式 %q 编译失败: %w", part, err)
		}
		list = append(list, re)
	}
	return list, nil
}
