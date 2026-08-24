package patent

import (
	"embed"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

//go:embed manifests/*.yaml
var manifestsFS embed.FS

// PatentWorkflowStep 是声明式工作流中的一步。EntryPoint 引用既有 Go 入口，
// 本层只做声明式路由，不重写既有实现。
//
//nolint:revive // 类型名带 Patent 前缀与包名略重叠，但表意清晰且与批次计划文档命名一致
type PatentWorkflowStep struct {
	ID         string `yaml:"id"`
	StepType   string `yaml:"step_type"`   // retrieve|analyze|draft|check
	EntryPoint string `yaml:"entry_point"` // 引用既有入口（见 resolveWorkflowEntryPoint）
}

// PatentWorkflowManifest 是一个声明式专利工作流 manifest。
//
//nolint:revive // 类型名带 Patent 前缀与包名略重叠，但表意清晰且与批次计划文档命名一致
type PatentWorkflowManifest struct {
	ID       string               `yaml:"id"`
	Name     string               `yaml:"name"`
	CaseType string               `yaml:"case_type"`
	Steps    []PatentWorkflowStep `yaml:"steps"`
}

// LoadPatentWorkflowManifests 从内嵌的 manifests/ 目录加载全部专利工作流 manifest。
func LoadPatentWorkflowManifests() ([]PatentWorkflowManifest, error) {
	entries, err := manifestsFS.ReadDir("manifests")
	if err != nil {
		return nil, fmt.Errorf("workflow manifests: 读取目录失败: %w", err)
	}

	var ms []PatentWorkflowManifest
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := manifestsFS.ReadFile("manifests/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("workflow manifests: 读取 %s: %w", e.Name(), err)
		}
		var m PatentWorkflowManifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("workflow manifests: 解析 %s: %w", e.Name(), err)
		}
		if m.ID == "" {
			return nil, fmt.Errorf("workflow manifests: %s 缺少 id", e.Name())
		}
		ms = append(ms, m)
	}

	sort.Slice(ms, func(i, j int) bool { return ms[i].ID < ms[j].ID })
	return ms, nil
}

// FindWorkflowManifest 按 ID 查找工作流 manifest；未找到返回 nil。
func FindWorkflowManifest(ms []PatentWorkflowManifest, id string) *PatentWorkflowManifest {
	for i := range ms {
		if ms[i].ID == id {
			return &ms[i]
		}
	}
	return nil
}

// workflowEntryLabels 声明式路由：EntryPoint → 既有 Go 入口描述。
// 本层只产出入口标识供上层调度，绝不在此重写既有图/工具实现。
var workflowEntryLabels = map[string]string{
	"prior-art-search": "BuildNoveltyGraph/BuildInvalidationGraph 检索前置（domain.DomainRetriever）",
	"novelty-analysis": "BuildNoveltyGraph — 新颖性 Pregel 图",
	"inventiveness":    "domains/inventiveness BuildInventivenessGraph — 创造性子图",
	"infringement":     "BuildInfringementGraph — 侵权比对 Pregel 图",
	"invalidation":     "BuildInvalidationGraph — 无效宣告 Pregel 图",
	"oa-response":      "BuildOAResponseGraph — 审查意见答复",
	"reexamination":    "BuildReexaminationGraph — 驳回复审",
	"disclosure":       "disclosure BuildDisclosureAnalysisGraph — 交底书分析",
	"checker-review":   "patent.NewChecker + rule_engine_checks（CAP09 量化结论）",
	"report-draft":     "domains/doctmpl 专利报告模板渲染",
}

// ResolveWorkflowEntryPoint 把 EntryPoint 映射为既有入口描述；未知入口返回 false。
func ResolveWorkflowEntryPoint(entryPoint string) (label string, ok bool) {
	label, ok = workflowEntryLabels[entryPoint]
	return label, ok
}

// ValidateManifestSchema 校验 manifest 的步骤引用均为合法 EntryPoint，且 StepType 合法。
func ValidateManifestSchema(m *PatentWorkflowManifest) error {
	if m.ID == "" || m.Name == "" {
		return fmt.Errorf("manifest 缺 id/name: %s", m.ID)
	}
	if len(m.Steps) == 0 {
		return fmt.Errorf("manifest %s 无步骤", m.ID)
	}
	validTypes := map[string]bool{"retrieve": true, "analyze": true, "draft": true, "check": true}
	seen := map[string]bool{}
	for _, s := range m.Steps {
		if !validTypes[s.StepType] {
			return fmt.Errorf("manifest %s 步骤 %s 非法 step_type=%q", m.ID, s.ID, s.StepType)
		}
		if _, ok := ResolveWorkflowEntryPoint(s.EntryPoint); !ok {
			return fmt.Errorf("manifest %s 步骤 %s 未知 entry_point=%q", m.ID, s.ID, s.EntryPoint)
		}
		if seen[s.ID] {
			return fmt.Errorf("manifest %s 步骤 id 重复: %s", m.ID, s.ID)
		}
		seen[s.ID] = true
	}
	return nil
}

// ValidateAllManifests 校验全部 manifest schema。
func ValidateAllManifests(ms []PatentWorkflowManifest) error {
	for i := range ms {
		if err := ValidateManifestSchema(&ms[i]); err != nil {
			return err
		}
	}
	return nil
}
