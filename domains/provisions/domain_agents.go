package provisions

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/xujian519/mady/agentcore"
)

// =============================================================================
// IPC 领域专家（Tier C）— Lazy 加载
// =============================================================================

// IpcSectionEntry 定义单个 IPC 大类的领域信息。
type IpcSectionEntry struct {
	Section       string   `yaml:"section" json:"section"`
	Name          string   `yaml:"name" json:"name"`
	WikiCardRoots []string `yaml:"wiki_card_roots" json:"wiki_card_roots"`
	Description   string   `yaml:"description" json:"description"`
	// PreRegister 标记该段是否在装配时预注册为 domain-* Handoff；
	// 仅预注册段会被 RegisterDomainExpertHandoffs 注册、被
	// ListDomainWorkerNames 发现，保证"广告的名字必然已注册"。
	PreRegister bool `yaml:"pre_register" json:"pre_register"`
}

// IpcDomainMap 是完整的 IPC 领域映射表。
type IpcDomainMap struct {
	IpcSections       []IpcSectionEntry `yaml:"ipc_sections" json:"ipc_sections"`
	ProvisionSuffixes map[string]string `yaml:"provision_suffixes" json:"provision_suffixes"`
}

// DefaultIpcMapPath 是 IPC 映射表的默认路径。
const DefaultIpcMapPath = "domains/rules/data/provisions/ipc-domain-map.yaml"

// LoadIpcDomainMap 从文件系统加载 IPC 领域映射表。
func LoadIpcDomainMap(mapPath string) (*IpcDomainMap, error) {
	path := mapPath
	if path == "" {
		path = DefaultIpcMapPath
	}
	if !filepath.IsAbs(path) {
		// 与 resolveManifestPath 同一查找策略：从调用方源文件向上定位项目根。
		_, file, _, ok := runtime.Caller(1)
		if ok {
			if root := projectRootUpward(file); root != "" {
				path = filepath.Join(root, path)
			}
		}
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is resolved from project root, not user input
	if err != nil {
		return nil, fmt.Errorf("provisions: 读取 IPC 映射表失败 %s: %w", path, err)
	}

	var m IpcDomainMap
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("provisions: 解析 IPC 映射表失败: %w", err)
	}
	return &m, nil
}

// LoadIpcDomainMapOrDefault 尝试加载 IPC 映射表，失败时返回空映射。
func LoadIpcDomainMapOrDefault(mapPath string) *IpcDomainMap {
	m, err := LoadIpcDomainMap(mapPath)
	if err != nil {
		// OrDefault 契约：加载失败降级为空映射，领域专家按"未注册领域"走默认标准。
		return &IpcDomainMap{}
	}
	return m
}

// ResolveDomainWorkerName 根据 IPC 段和条款后缀生成 domain worker 名。
// 例如：ResolveDomainWorkerName("A61", "novelty") → "domain-A61-novelty"
func ResolveDomainWorkerName(ipcSection string, suffix string) string {
	normalized := strings.ToUpper(strings.TrimSpace(ipcSection))
	return fmt.Sprintf("domain-%s-%s", normalized, suffix)
}

// BuildDomainSystemPrompt 为 IPC 领域专家构建 System Prompt。
func BuildDomainSystemPrompt(section *IpcSectionEntry, provisionName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "你是 Mady 的 %s 领域专家（IPC %s）。\n", section.Name, section.Section)
	fmt.Fprintf(&b, "领域描述：%s\n\n", section.Description)
	fmt.Fprintf(&b, "当前分析任务：%s\n\n", provisionName)
	b.WriteString("你负责在该特定技术领域内提供以下支持：\n")
	b.WriteString("- 领域特定的审查标准与统计规律\n")
	b.WriteString("- 典型技术手段的公知常识判断\n")
	b.WriteString("- 该领域技术人员的知识水平界定\n")
	b.WriteString("- 该领域常见技术偏见或惯用手段\n\n")
	b.WriteString("约束规则：\n")
	b.WriteString("- 专注于本技术领域，不提供超出领域的泛化意见\n")
	b.WriteString("- 必须区分本领域特有的审查标准与通用标准\n")
	b.WriteString("- 引用该领域的典型判决或审查决定时注明来源\n")
	b.WriteString("- 信息不足时明确标注，不得强行结论\n")
	return b.String()
}

// DomainAgentHandoffConfig 返回基于 IPC 段和条款名称的领域专家 Handoff 配置。
// 单次按需构建入口；装配期批量预注册走 RegisterDomainExpertHandoffs。
func DomainAgentHandoffConfig(ipcSection string, suffix string, provisionName string, base agentcore.Config) agentcore.HandoffConfig {
	return buildDomainHandoff(findIpcSection(LoadIpcDomainMapOrDefault(""), ipcSection), suffix, provisionName, base)
}

// findIpcSection 在映射表中按 IPC 段查找领域定义；未命中时返回使用默认标准的占位段。
func findIpcSection(ipcMap *IpcDomainMap, ipcSection string) *IpcSectionEntry {
	if ipcMap != nil {
		normalized := strings.ToUpper(strings.TrimSpace(ipcSection))
		for i := range ipcMap.IpcSections {
			if strings.ToUpper(ipcMap.IpcSections[i].Section) == normalized {
				return &ipcMap.IpcSections[i]
			}
		}
	}
	return &IpcSectionEntry{
		Section:     strings.ToUpper(strings.TrimSpace(ipcSection)),
		Name:        fmt.Sprintf("IPC %s", strings.ToUpper(strings.TrimSpace(ipcSection))),
		Description: "未在 IPC 映射表中注册的领域，使用默认标准。",
	}
}

// buildDomainHandoff 组装单个领域专家 Handoff 配置。ipcMap 由调用方加载后
// 经 findIpcSection 选段传入，装配期预注册循环可复用一次加载。
func buildDomainHandoff(section *IpcSectionEntry, suffix string, provisionName string, base agentcore.Config) agentcore.HandoffConfig {
	workerName := ResolveDomainWorkerName(section.Section, suffix)
	cfg := base
	cfg.Name = workerName
	cfg.SystemPrompt = BuildDomainSystemPrompt(section, provisionName)
	// 领域专家保持轻量：不继承扩展与生命周期，工具限定默认检索集，
	// 防止领域专家递归调用重型专利分析工具。
	cfg.Extensions = nil
	cfg.Lifecycle = nil
	cfg.Tools = filterTools(DefaultPatentTools, cfg.Tools)

	return agentcore.HandoffConfig{
		Name:        workerName,
		Description: fmt.Sprintf("%s领域%s分析专家（IPC %s）", section.Name, provisionName, section.Section),
		Mode:        agentcore.HandoffDelegate,
		AgentConfig: cfg,
		AllowedSources: []string{
			"patent-agent",
			"patent-orchestrator",
			"mady-router",
			"mady-agent",
		},
		FallbackMsg: fmt.Sprintf("%s 领域专家暂时不可用，使用通用标准分析。", section.Name),
		Invisible:   true,
	}
}

// suffixLabels 将条款后缀映射为中文分析领域名（与条款智能体命名口径一致）。
var suffixLabels = map[string]string{
	"novelty":        "新颖性",
	"inventiveness":  "创造性",
	"disclosure":     "充分公开",
	"claims-clarity": "清楚支持",
	"utility":        "实用性",
	"eligibility":    "保护客体",
	"amendment":      "修改超范围",
}

// suffixLabel 返回条款后缀的中文领域名；未知后缀原样返回。
func suffixLabel(suffix string) string {
	if label := suffixLabels[suffix]; label != "" {
		return label
	}
	return suffix
}

// RegisterDomainExpertHandoffs 从 IPC 映射表预注册 Tier C 领域专家 Handoff：
// 每个 pre_register 的 IPC 段 × provision_suffixes 生成一个
// domain-{section}-{suffix} Handoff，追加到 cfg.Handoffs。
// mapPath 为空使用默认路径。返回注册数量；映射表不可用时降级为 0
// （fail-open，不阻断 PatentAgent 装配）。
func RegisterDomainExpertHandoffs(cfg *agentcore.Config, mapPath string) int {
	ipcMap := LoadIpcDomainMapOrDefault(mapPath)
	if ipcMap == nil || len(ipcMap.IpcSections) == 0 || len(ipcMap.ProvisionSuffixes) == 0 {
		return 0
	}
	count := 0
	for i := range ipcMap.IpcSections {
		sec := &ipcMap.IpcSections[i]
		if !sec.PreRegister {
			continue
		}
		for _, suffix := range ipcMap.ProvisionSuffixes {
			cfg.Handoffs = append(cfg.Handoffs, buildDomainHandoff(sec, suffix, suffixLabel(suffix), *cfg))
			count++
		}
	}
	return count
}

// ListDomainWorkerNames 返回给定 IPC 提示列表对应的所有 domain worker 名称。
func ListDomainWorkerNames(ipcHints []string, mapPath string) []string {
	ipcMap := LoadIpcDomainMapOrDefault(mapPath)
	if ipcMap == nil || len(ipcMap.IpcSections) == 0 {
		return nil
	}

	// 收集所有条款后缀
	suffixes := ipcMap.ProvisionSuffixes
	if len(suffixes) == 0 {
		suffixes = map[string]string{
			"novelty": "novelty", "inventiveness": "inventiveness",
			"disclosure": "disclosure", "clarity": "claims-clarity",
		}
	}

	var names []string
	seen := make(map[string]bool)
	for _, hint := range ipcHints {
		normalized := strings.ToUpper(strings.TrimSpace(hint))
		// 仅广告已预注册的段，保证返回的名字必有对应的 Handoff。
		found := false
		for _, sec := range ipcMap.IpcSections {
			if sec.PreRegister && strings.ToUpper(sec.Section) == normalized {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		for _, suffix := range suffixes {
			name := ResolveDomainWorkerName(normalized, suffix)
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}
