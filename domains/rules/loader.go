package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RuleSet is the complete collection of loaded rules, article frameworks,
// orchestrations, and reflection indicators.
type RuleSet struct {
	Rules             []Rule
	Articles          map[string]*ArticleFramework
	Orchestrations    map[string]*Orchestration
	ReflectionDomains map[string]*ReflectionDomain
	rulesByDomain     map[string][]Rule
	rulesBySeverity   map[Severity][]Rule
	ruleIndex         map[string]*Rule
}

// LoadFromDir loads all rule YAML files from the given directory.
// Expected layout:
//
//	dir/
//	  *.yaml              — rule files (rules: [...])
//	  articles/*.yaml     — article frameworks
//	  orchestrations/*.yaml — orchestrations
//	  reflection-indicators.yaml — reflection phrases
func LoadFromDir(dir string) (*RuleSet, error) {
	rs := &RuleSet{
		Articles:          make(map[string]*ArticleFramework),
		Orchestrations:    make(map[string]*Orchestration),
		ReflectionDomains: make(map[string]*ReflectionDomain),
		rulesByDomain:     make(map[string][]Rule),
		rulesBySeverity:   make(map[Severity][]Rule),
		ruleIndex:         make(map[string]*Rule),
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read rules dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(dir, name)
		if name == "reflection-indicators.yaml" {
			if err := rs.loadReflection(path); err != nil {
				return nil, fmt.Errorf("load %s: %w", name, err)
			}
			continue
		}
		if err := rs.loadRuleFile(path); err != nil {
			return nil, fmt.Errorf("load %s: %w", name, err)
		}
	}

	articlesDir := filepath.Join(dir, "articles")
	if entries, err := os.ReadDir(articlesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			path := filepath.Join(articlesDir, entry.Name())
			if err := rs.loadArticle(path); err != nil {
				return nil, fmt.Errorf("load articles/%s: %w", entry.Name(), err)
			}
		}
	}

	orchDir := filepath.Join(dir, "orchestrations")
	if entries, err := os.ReadDir(orchDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}
			path := filepath.Join(orchDir, entry.Name())
			if err := rs.loadOrchestration(path); err != nil {
				return nil, fmt.Errorf("load orchestrations/%s: %w", entry.Name(), err)
			}
		}
	}

	rs.indexRules()
	return rs, nil
}

func (rs *RuleSet) loadRuleFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var rf ruleFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	rs.Rules = append(rs.Rules, rf.Rules...)
	return nil
}

func (rs *RuleSet) loadArticle(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var af ArticleFramework
	if err := yaml.Unmarshal(data, &af); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	rs.Articles[af.ArticleID] = &af
	return nil
}

func (rs *RuleSet) loadOrchestration(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var orch Orchestration
	if err := yaml.Unmarshal(data, &orch); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	rs.Orchestrations[orch.CaseType] = &orch
	return nil
}

func (rs *RuleSet) loadReflection(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var rf reflectionFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	for domain, rd := range rf {
		d := rd
		rs.ReflectionDomains[domain] = &d
	}
	return nil
}

func (rs *RuleSet) indexRules() {
	for i := range rs.Rules {
		r := &rs.Rules[i]
		rs.ruleIndex[r.RuleID] = r
		rs.rulesByDomain[r.Domain] = append(rs.rulesByDomain[r.Domain], *r)
		rs.rulesBySeverity[r.Severity] = append(rs.rulesBySeverity[r.Severity], *r)
	}
}
