// Package filing 把一件 CNIPA 专利申请文件成文为 DOCX，并对成品跑体例与内容断言。
//
// 体例真相源是随包的 `申请文件模板.docx`：字体、字号、行距、首行缩进、分节与页眉
// 都由 Python 引擎从模板反解，`申请文件.json` 的 style 块只作投影核对。Go 侧负责
// 资产解压、调用引擎与结果校验，排版逻辑不在这里重写。
package filing

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xujian519/mady/pkg/util"
)

//go:embed data/template/申请文件模板.docx data/template/TEMPLATE-IDENTITY.md data/spec/申请文件.json
//go:embed engine/build.py engine/style.py engine/render_figures.py engine/verify.py
var assets embed.FS

// assetFiles 是解压后相对路径与 embed 路径的对应关系。
var assetFiles = map[string]string{
	"template/申请文件模板.docx":          "data/template/申请文件模板.docx",
	"template/TEMPLATE-IDENTITY.md": "data/template/TEMPLATE-IDENTITY.md",
	"spec/申请文件.json":                "data/spec/申请文件.json",
	"engine/build.py":               "engine/build.py",
	"engine/style.py":               "engine/style.py",
	"engine/render_figures.py":      "engine/render_figures.py",
	"engine/verify.py":              "engine/verify.py",
}

// Assets 是解压后的出件资产路径。
type Assets struct {
	// Dir 是资产根目录。
	Dir string
	// Template 是体例真相源 DOCX 的路径。
	Template string
	// Spec 是结构断言规格的路径。
	Spec string
	// BuildScript 是成文引擎入口。
	BuildScript string
	// VerifyScript 是成品验收引擎入口。
	VerifyScript string
}

// Materialize 把随包资产解压到数据目录，内容不一致时覆盖。
//
// 用内容比对而不是版本号：资产随二进制升级，旧目录里的同名文件必须同步，
// 否则会在旧模板上成文。
func Materialize() (Assets, error) {
	root, err := util.ResolveDataDir("filing-assets")
	if err != nil {
		return Assets{}, fmt.Errorf("filing: 解析资产目录: %w", err)
	}
	for relative, embedded := range assetFiles {
		data, err := assets.ReadFile(embedded)
		if err != nil {
			return Assets{}, fmt.Errorf("filing: 读取随包资产 %s: %w", embedded, err)
		}
		target := filepath.Join(root, relative)
		//nolint:gosec // target 由数据目录与固定相对路径拼接，不含用户输入
		if existing, err := os.ReadFile(target); err == nil && bytes.Equal(existing, data) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return Assets{}, fmt.Errorf("filing: 创建目录: %w", err)
		}
		//nolint:gosec // target 由数据目录与固定相对路径拼接，不含用户输入
		if err := os.WriteFile(target, data, 0o600); err != nil {
			return Assets{}, fmt.Errorf("filing: 写入资产 %s: %w", target, err)
		}
	}
	return Assets{
		Dir:          root,
		Template:     filepath.Join(root, "template", "申请文件模板.docx"),
		Spec:         filepath.Join(root, "spec", "申请文件.json"),
		BuildScript:  filepath.Join(root, "engine", "build.py"),
		VerifyScript: filepath.Join(root, "engine", "verify.py"),
	}, nil
}
