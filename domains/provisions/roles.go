package provisions

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const rolesDir = "skills/patent/roles"

// RoleStep 是角色方法论中的一步（按 HITL 编号选择协议呈现）。
type RoleStep struct {
	Step        int    `yaml:"step"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Role 是专利域角色（skills/patent/roles/*.yaml）。字段贴合实际 YAML 结构；
// Methodology 为结构化步骤列表而非自由文本，解析后保留原始语义。
type Role struct {
	RoleID                string     `yaml:"role_id"`
	Name                  string     `yaml:"name"`
	Identity              string     `yaml:"identity"`
	DeveloperInstructions string     `yaml:"developer_instructions"`
	Methodology           []RoleStep `yaml:"methodology"`
	OutputFormat          string     `yaml:"output_format"`
	PrimaryTools          []string   `yaml:"primary_tools"`
}

// findProjectRoot 从调用栈源文件向上定位项目根（含 go.mod）。
func findProjectRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join(".", rolesDir)
	}
	if root := projectRootUpward(file); root != "" {
		return root
	}
	return filepath.Join(".", rolesDir)
}

// projectRootUpward 从源文件所在目录向上（最多 12 层）查找含 go.mod 的项目根。
// findProjectRoot 与 resolveManifestPath 共用此查找逻辑；未找到返回空串。
func projectRootUpward(file string) string {
	dir := filepath.Dir(file)
	for range 12 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// LoadRoles 解析 skills/patent/roles/ 下的全部角色 YAML，按 RoleID 排序返回。
// 单个文件解析失败即返回错误（fail-closed），保证角色目录完整。
func LoadRoles() ([]Role, error) {
	dir := filepath.Join(findProjectRoot(), rolesDir)
	matches, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("provisions: glob roles: %w", err)
	}

	roles := make([]Role, 0, len(matches))
	for _, path := range matches {
		data, err := os.ReadFile(path) //nolint:gosec // fixed role dir under project root
		if err != nil {
			return nil, fmt.Errorf("provisions: 读取角色 %s: %w", path, err)
		}
		var r Role
		if err := yaml.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("provisions: 解析角色 %s: %w", path, err)
		}
		if r.RoleID == "" {
			return nil, fmt.Errorf("provisions: 角色 %s 缺少 role_id", path)
		}
		roles = append(roles, r)
	}

	sort.Slice(roles, func(i, j int) bool { return roles[i].RoleID < roles[j].RoleID })
	return roles, nil
}

// BuildRoleListForSystemPrompt 生成角色能力目录，供 SystemPrompt 展示。
func BuildRoleListForSystemPrompt(roles []Role) string {
	if len(roles) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("专利域角色（可按需委派）：\n")
	for _, r := range roles {
		fmt.Fprintf(&b, "- %s（%s）\n", r.Name, r.RoleID)
	}
	return b.String()
}
