package doctmpl

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const manifestPath = "TEMPLATE-MANIFEST.md"

// manifestEntry 是清单中的一行。
type manifestEntry struct {
	Category string
	Dir      string
}

// readManifest 读合并清单表格（ID / 类别 / 目录 / 域 / 来源 / 处置）。
func readManifest(t *testing.T) map[string]manifestEntry {
	t.Helper()
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("读取清单: %v", err)
	}
	entries := make(map[string]manifestEntry)
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "| ") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 6 {
			continue
		}
		id := strings.TrimSpace(cells[0])
		if id == "" || id == "ID" || strings.HasPrefix(id, "---") {
			continue
		}
		entries[id] = manifestEntry{Category: strings.TrimSpace(cells[1]), Dir: strings.TrimSpace(cells[2])}
	}
	return entries
}

// 清单与内置模板必须一一对应：新增模板要登记来源，删除模板要清掉清单。
func TestTemplateManifestMatchesEmbedded(t *testing.T) {
	templates, err := LoadDocTemplatesFromFS(embeddedTemplatesFS, embeddedTemplatesDir)
	if err != nil {
		t.Fatalf("加载内置模板: %v", err)
	}

	byName := make(map[string]DocTemplate, len(templates))
	for _, tmpl := range templates {
		if _, dup := byName[tmpl.Name]; dup {
			t.Errorf("重复模板 ID：%s（%s）", tmpl.Name, tmpl.FilePath)
			continue
		}
		byName[tmpl.Name] = tmpl

		if tmpl.Domain == "" {
			t.Errorf("%s：缺少 domain", tmpl.Name)
		}
		seenVars := make(map[string]bool, len(tmpl.VarSchema.Definitions))
		for _, def := range tmpl.VarSchema.Definitions {
			if seenVars[def.Name] {
				t.Errorf("%s：变量 %s 重复声明", tmpl.Name, def.Name)
			}
			seenVars[def.Name] = true
		}

		// frontmatter 与正文脱节会让变量校验失效：正文有的占位符没声明就无从校验，
		// 声明了正文却没用的变量是冗余。
		declared := make(map[string]bool, len(seenVars))
		for name := range seenVars {
			declared[name] = true
		}
		for _, placeholder := range ExtractPlaceholders(tmpl.Body) {
			if !declared[placeholder] {
				t.Errorf("%s：正文占位符 {{%s}} 未在 vars 中声明", tmpl.Name, placeholder)
			}
			delete(declared, placeholder)
		}
		for name := range declared {
			t.Errorf("%s：vars 声明的 %s 未在正文中使用", tmpl.Name, name)
		}
	}

	manifest := readManifest(t)
	for name, tmpl := range byName {
		entry, ok := manifest[name]
		if !ok {
			t.Errorf("模板 %s 未在 %s 中登记", name, manifestPath)
			continue
		}
		// category 与目录可以不同名（如 patent/ 下的 patent-report），但清单必须与实际一致。
		if entry.Category != tmpl.Category {
			t.Errorf("%s：清单类别 %q 与实际 %q 不一致", name, entry.Category, tmpl.Category)
		}
		if dir := filepath.Base(filepath.Dir(tmpl.FilePath)); entry.Dir != dir {
			t.Errorf("%s：清单目录 %q 与实际 %q 不一致", name, entry.Dir, dir)
		}
	}
	for name := range manifest {
		if _, ok := byName[name]; !ok {
			t.Errorf("清单中的 %s 在内置模板里不存在", name)
		}
	}
}

// DocIndex 必须列出所有类别，包括后加的 patent-report 与 document。
func TestDocIndexCoversAllCategories(t *testing.T) {
	templates, err := LoadDocTemplatesFromFS(embeddedTemplatesFS, embeddedTemplatesDir)
	if err != nil {
		t.Fatalf("加载内置模板: %v", err)
	}
	index := DocIndex(templates)
	for _, tmpl := range templates {
		if !strings.Contains(index, "["+tmpl.Category+"]") {
			t.Errorf("索引缺少类别 %s", tmpl.Category)
		}
	}
}

// 仓库根的 doc-templates 是内置模板的镜像；任一侧单独改动都会让两边分叉。
func TestDocTemplatesMirrorMatchesEmbedded(t *testing.T) {
	root := filepath.Join("..", "..", "doc-templates")
	if _, err := os.Stat(root); err != nil {
		// 不静默跳过：镜像测试没跑，需要让人知道这份一致性没有验证。
		t.Logf("镜像测试未执行（%v）", err)
		t.Skip("镜像目录不可用")
	}

	seen := make(map[string]bool)
	err := fs.WalkDir(embeddedTemplatesFS, embeddedTemplatesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
			return err
		}
		rel := strings.TrimPrefix(path, embeddedTemplatesDir+"/")
		seen[rel] = true

		embedded, err := fs.ReadFile(embeddedTemplatesFS, path)
		if err != nil {
			return err
		}
		mirror, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("镜像缺少 %s: %v", rel, err)
			return nil
		}
		if !bytes.Equal(mirror, embedded) {
			t.Errorf("镜像 %s 与内置模板不一致", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("遍历内置模板: %v", err)
	}

	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".md" {
			return err
		}
		if strings.HasSuffix(path, ".example.md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !seen[filepath.ToSlash(rel)] {
			t.Errorf("镜像中的 %s 不在内置模板里", rel)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("遍历镜像: %v", walkErr)
	}
}
