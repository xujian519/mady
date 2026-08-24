package provisions

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// =============================================================================
// Manifest 类型定义
// =============================================================================

// ProvisionManifestEntry 定义一个 Tier A 条款智能体。
type ProvisionManifestEntry struct {
	ID               string   `yaml:"id" json:"id"`
	Worker           string   `yaml:"worker" json:"worker"`
	Name             string   `yaml:"name" json:"name"`
	PreRegister      bool     `yaml:"pre_register" json:"pre_register"`
	LegalBasis       []string `yaml:"legal_basis" json:"legal_basis"`
	ConceptIDs       []string `yaml:"concept_ids" json:"concept_ids"`
	MethodologySteps []string `yaml:"methodology_steps" json:"methodology_steps"`
	PrimaryTools     []string `yaml:"primary_tools" json:"primary_tools"`
	ExistingSubgraph string   `yaml:"existing_subgraph,omitempty" json:"existing_subgraph,omitempty"`
	ToolHints        string   `yaml:"tool_hints,omitempty" json:"tool_hints,omitempty"`
	// WikiRoots 是条款对应的 wiki 知识根目录（增量来自 Kimi 拷贝版），供检索限域。
	WikiRoots []string `yaml:"wiki_roots,omitempty" json:"wiki_roots,omitempty"`
}

// ReasoningManifestEntry 定义一个 Tier B 推理模式。
type ReasoningManifestEntry struct {
	ID               string   `yaml:"id" json:"id"`
	Worker           string   `yaml:"worker" json:"worker"`
	Name             string   `yaml:"name" json:"name"`
	PreRegister      bool     `yaml:"pre_register" json:"pre_register"`
	Serves           []string `yaml:"serves" json:"serves"`
	MethodologySteps []string `yaml:"methodology_steps" json:"methodology_steps"`
	PrimaryTools     []string `yaml:"primary_tools" json:"primary_tools"`
}

// PatentManifest 是完整的 Manifest 结构。
type PatentManifest struct {
	Provisions []ProvisionManifestEntry `yaml:"provisions" json:"provisions"`
	Reasoning  []ReasoningManifestEntry `yaml:"reasoning" json:"reasoning"`
}

// =============================================================================
// 常量
// =============================================================================

// TierAProvisionIDs 是 Tier A 条款簇最小完备集 ID（22 条）。
// 首批高频 9 条预注册，其余按需扩展。
var TierAProvisionIDs = []string{
	"P-A01", "P-A02", "P-A03", "P-A04", "P-A05",
	"P-A06", "P-A07", "P-A08", "P-A09",
	"P-B01", "P-B02", "P-B03", "P-B04", "P-B05", "P-B06",
	"P-C01", "P-C02", "P-C03", "P-C04", "P-C05",
	"P-D01", "P-D02", "P-D03",
}

// DefaultPatentTools 是条款智能体的默认工具集。
var DefaultPatentTools = []string{"read", "grep", "knowledge_search", "search_knowledge", "knowledge_rules"}

// DefaultManifestPath 是 manifest 文件的默认路径（相对于仓库根）。
const DefaultManifestPath = "domains/rules/data/provisions/manifest.yaml"

// =============================================================================
// Manifest 加载
// =============================================================================

// resolveManifestPath 尝试解析 manifest.yaml 的绝对路径。
// 如果传入的是绝对路径，直接返回；否则从调用栈的源文件目录向上查找项目根。
func resolveManifestPath(path string) string {
	if path == "" {
		path = DefaultManifestPath
	}
	if filepath.IsAbs(path) {
		return path
	}
	// 从调用方的源文件开始向上查找 go.mod
	_, file, _, ok := runtime.Caller(1)
	if ok {
		dir := filepath.Dir(file)
		for i := 0; i < 12; i++ {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return filepath.Join(dir, path)
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return path
}

// LoadManifest 从文件系统加载专利条款 Manifest。
// manifestPath 是 manifest.yaml 的路径。传空字符串则使用默认路径。
// 支持相对路径（从项目根解析）和绝对路径。
func LoadManifest(manifestPath string) (*PatentManifest, error) {
	path := resolveManifestPath(manifestPath)

	data, err := os.ReadFile(path) //nolint:gosec // path is resolved from project root, not user input
	if err != nil {
		return nil, fmt.Errorf("provisions: 读取 manifest 失败 %s: %w", path, err)
	}

	var m PatentManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("provisions: 解析 manifest 失败: %w", err)
	}

	return &m, nil
}

// LoadManifestOrDefault 尝试加载 manifest，失败时返回空 Manifest。
func LoadManifestOrDefault(manifestPath string) *PatentManifest {
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return &PatentManifest{Provisions: nil, Reasoning: nil}
	}
	return m
}

// ValidateManifest 检查 manifest 是否覆盖了全部 Tier A 条款簇。
func ValidateManifest(m *PatentManifest) (ok bool, missing []string) {
	defined := make(map[string]bool)
	for _, p := range m.Provisions {
		defined[p.ID] = true
	}
	for _, id := range TierAProvisionIDs {
		if !defined[id] {
			missing = append(missing, id)
		}
	}
	return len(missing) == 0, missing
}
