//go:build darwin

package main

// app_skills.go — 技能管理（T5.6，PilotDeck 对齐）。
//
// ListSkills 扫描所有技能发现路径，返回已发现的技能列表。
// 扫描路径与 bootstrap.DiscoverSkills 保持一致。

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/skill"

	"github.com/xujian519/mady/pkg/util"
)

// SkillEntry 是一个已发现技能的概要信息。
type SkillEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Path 是 SKILL.md 相对项目根的路径，可直接用于 ReadFile/WriteFile。
	Path string `json:"path"`
}

// ListSkills 扫描所有技能发现路径，返回发现的技能列表。
// 项目本地技能优先，全局技能按名称去重（同名以项目本地为准）。
// 未找到任何技能时返回空列表（不视为错误）。
//
// 扫描路径与 bootstrap.DiscoverSkills 保持一致：
//   - SKILL_DIR 环境变量
//   - ~/.agent/
//   - $PWD/.agent/
//   - $PWD/skills/
//   - $PWD/plugins/
//   - $MADY_HOME/skills/
//   - ~/.agents/skills/
func (a *App) ListSkills() ([]SkillEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}

	cwd, err := a.resolveProjectDir()
	if err != nil {
		return nil, fmt.Errorf("ListSkills: %w", err)
	}

	homeDir, _ := os.UserHomeDir()
	madyHome, _ := util.MadyHome()

	type scanned struct {
		root string
		dir  string
	}
	dirs := []scanned{}

	// 1. SKILL_DIR 环境变量
	if env := os.Getenv("SKILL_DIR"); env != "" {
		for _, p := range filepath.SplitList(env) {
			if p != "" {
				dirs = append(dirs, scanned{root: p, dir: p})
			}
		}
	}
	// 2. ~/.agent/
	if homeDir != "" {
		dirs = append(dirs, scanned{root: homeDir, dir: filepath.Join(homeDir, ".agent")})
	}
	// 3. $PWD/.agent/
	dirs = append(dirs, scanned{root: cwd, dir: filepath.Join(cwd, ".agent")})
	// 4. $PWD/skills/
	dirs = append(dirs, scanned{root: cwd, dir: filepath.Join(cwd, "skills")})
	// 4b. $PWD/plugins/ (插件 SKILL.md)
	dirs = append(dirs, scanned{root: cwd, dir: filepath.Join(cwd, "plugins")})
	// 5. $MADY_HOME/skills/
	if madyHome != "" && madyHome != cwd {
		dirs = append(dirs, scanned{root: madyHome, dir: filepath.Join(madyHome, "skills")})
	}
	// 6. ~/.agents/skills/
	if homeDir != "" {
		dirs = append(dirs, scanned{root: homeDir, dir: filepath.Join(homeDir, ".agents", "skills")})
	}

	seen := make(map[string]bool)
	var result []SkillEntry

	for _, d := range dirs {
		if _, err := os.Stat(d.dir); os.IsNotExist(err) {
			continue
		}
		skills, _, err := skill.Load(d.dir)
		if err != nil {
			continue
		}
		for _, s := range skills {
			if seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			var entryPath string
			if d.root == cwd {
				rel, err := filepath.Rel(cwd, s.FilePath)
				if err != nil {
					log.Printf("ListSkills: filepath.Rel(%q, %q) failed: %v", cwd, s.FilePath, err)
					entryPath = filepath.ToSlash(s.FilePath)
				} else {
					entryPath = filepath.ToSlash(rel)
				}
			} else {
				entryPath = filepath.ToSlash(s.FilePath)
			}
			result = append(result, SkillEntry{
				Name:        s.Name,
				Description: s.Description,
				Path:        entryPath,
			})
		}
	}

	if result == nil {
		return []SkillEntry{}, nil
	}
	return result, nil
}
