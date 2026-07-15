// Package skill 提供 SKILL.md 文档的解析和管理功能，支持从 YAML
// frontmatter 提取技能元数据（名称、描述、触发词、依赖工具列表等）。
//
// 主要功能：
//   - SKILL.md 文件解析（YAML Frontmatter + Markdown 正文）
//   - 技能元数据提取与校验
//   - 技能索引（按域/标签分组）
//   - 技能定义的结构化表示
//
// 主要类型：
//   - Skill: 技能定义（名称、描述、触发词、工具列表等）
//   - Parser: SKILL.md 解析器，提取 frontmatter 和正文
//
// 使用示例：
//
//	skill, _ := skill.Parse("skills/patent/SKILL.md")
//	fmt.Println(skill.Name) // "patent-analysis"
package skill
