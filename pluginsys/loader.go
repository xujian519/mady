package pluginsys

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// LoadPlugin reads a plugin.json file and returns the parsed manifest.
// opts may be nil for structural-only validation.
func LoadPlugin(path string, opts *ValidateOptions) (*PluginManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("plugin: read %s: %w", path, err)
	}
	var p PluginManifest
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("plugin: parse %s: %w", path, err)
	}
	if err := ValidatePlugin(p, opts); err != nil {
		return nil, err
	}
	return &p, nil
}

// ScanPlugins discovers all plugin.json files under the given root
// directories. Plugins with the same name keep the first one found.
// opts may be nil for structural-only validation.
func ScanPlugins(roots []string, opts *ValidateOptions) ([]PluginManifest, error) {
	var all []PluginManifest
	seen := make(map[string]bool)
	for _, root := range roots {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || filepath.Base(path) != "plugin.json" {
				return nil
			}
			p, err := LoadPlugin(path, opts)
			if err != nil {
				return fmt.Errorf("plugin: %s: %w", path, err)
			}
			if seen[p.Name] {
				return nil // first wins
			}
			seen[p.Name] = true
			// Resolve skill path relative to plugin directory.
			if p.SkillPath == "" {
				p.SkillPath = filepath.Join(filepath.Dir(path), "SKILL.md")
			} else if !filepath.IsAbs(p.SkillPath) {
				p.SkillPath = filepath.Join(filepath.Dir(path), p.SkillPath)
			}
			all = append(all, *p)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return all, nil
}
