package agentcore

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// HandoffRoleXML is the XML structure for a handoff role definition file.
type HandoffRoleXML struct {
	XMLName        xml.Name `xml:"handoff_role"`
	Name           string   `xml:"name,attr"`
	Mode           string   `xml:"mode,attr"`
	Invisible      bool     `xml:"invisible,attr"`
	Description    string   `xml:"description"`
	SystemPrompt   string   `xml:"system_prompt"`
	AllowedSources []string `xml:"allowed_sources>source"`
	Model          string   `xml:"model"`
	Temperature    float64  `xml:"temperature"`
}

// ParseHandoffRole parses a handoff role XML definition into a HandoffConfig.
// Returns an error if the XML is invalid or required fields are missing.
func ParseHandoffRole(xmlData []byte) (HandoffConfig, error) {
	var role HandoffRoleXML
	if err := xml.Unmarshal(xmlData, &role); err != nil {
		return HandoffConfig{}, fmt.Errorf("handoff role XML parse: %w", err)
	}

	if strings.TrimSpace(role.Name) == "" {
		return HandoffConfig{}, fmt.Errorf("handoff role: name is required")
	}
	if strings.TrimSpace(role.Description) == "" {
		return HandoffConfig{}, fmt.Errorf("handoff role %q: description is required", role.Name)
	}

	var mode HandoffMode
	switch strings.ToLower(role.Mode) {
	case "transfer":
		mode = HandoffTransfer
	case "delegate", "":
		mode = HandoffDelegate
	default:
		return HandoffConfig{}, fmt.Errorf("handoff role %q: unknown mode %q", role.Name, role.Mode)
	}

	config := Config{}
	config.Name = role.Name
	config.SystemPrompt = strings.TrimSpace(role.SystemPrompt)
	config.Model = role.Model
	config.Temperature = role.Temperature

	// Apply default temperature if the role does not specify one.
	if role.Temperature == 0 {
		config.Temperature = 0.3
	}

	return HandoffConfig{
		Name:           role.Name,
		Description:    strings.TrimSpace(role.Description),
		Mode:           mode,
		AgentConfig:    config,
		AllowedSources: role.AllowedSources,
		FallbackMsg:    fmt.Sprintf("%s 功能暂时不可用，请稍后再试。", role.Description),
		Invisible:      role.Invisible,
	}, nil
}

// HandoffRoleStore manages loading, caching, and hot-reloading of handoff role
// definitions from XML files on disk.
type HandoffRoleStore struct {
	roles map[string]HandoffConfig
	mu    sync.RWMutex
	dirs  []string // search directories in priority order
}

// NewHandoffRoleStore creates an empty role store.
func NewHandoffRoleStore(dirs ...string) *HandoffRoleStore {
	return &HandoffRoleStore{
		roles: make(map[string]HandoffConfig),
		dirs:  dirs,
	}
}

// Load scans all configured directories for XML role files and populates
// the store. Later directories override earlier ones. Files must have
// the .xml extension and be named {role-name}.xml.
func (s *HandoffRoleStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, dir := range s.dirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("handoff role store: read dir %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".xml") {
				continue
			}
			fullPath := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(fullPath) //nolint:gosec // 路径来自框架配置目录，非用户输入
			if err != nil {
				return fmt.Errorf("handoff role store: read %s: %w", fullPath, err)
			}
			config, err := ParseHandoffRole(data)
			if err != nil {
				return fmt.Errorf("handoff role store: %s: %w", fullPath, err)
			}
			s.roles[config.Name] = config
		}
	}
	return nil
}

// Get returns a role config by name. If not found, returns false.
func (s *HandoffRoleStore) Get(name string) (HandoffConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, ok := s.roles[name]
	return cfg, ok
}

// All returns all loaded role configs.
func (s *HandoffRoleStore) All() []HandoffConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []HandoffConfig
	for _, cfg := range s.roles {
		result = append(result, cfg)
	}
	return result
}

// MergeWithCode merges XML-defined roles with code-defined HandoffConfigs.
// Code configs take priority: if a code config has the same name as an
// XML role, the code version is used.
func (s *HandoffRoleStore) MergeWithCode(codeConfigs []HandoffConfig) []HandoffConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	codeNames := make(map[string]bool)
	for _, c := range codeConfigs {
		codeNames[c.Name] = true
	}

	var result []HandoffConfig
	result = append(result, codeConfigs...)

	for name, cfg := range s.roles {
		if !codeNames[name] {
			result = append(result, cfg)
		}
	}
	return result
}
