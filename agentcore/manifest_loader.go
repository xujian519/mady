package agentcore

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
)

// ManifestLoadError 包含加载过程中遇到的非致命错误的详细信息。
// 调用方可以据此生成日志或诊断报告，而不中断整体加载流程。
type ManifestLoadError struct {
	Path  string // 出错的文件路径
	Error string // 错误描述
}

// ManifestMergeResult 描述 LoadManifests 的合并结果，供调用方生成诊断日志。
type ManifestMergeResult struct {
	// Manifests 是合并后的最终列表（内置 + 外部覆盖/新增），按 Name 排序。
	Manifests []AgentManifest
	// Errors 是加载过程中的非致命错误（文件损坏、校验失败等）。
	Errors []ManifestLoadError
	// EmbeddedCount 是从 embed.FS 成功加载的内置 manifest 数量。
	EmbeddedCount int
	// ExternalCount 是从 userDir 成功加载的外部 manifest 数量。
	ExternalCount int
	// Overridden 是被外部同名文件覆盖的内置 manifest 名称列表。
	Overridden []string
	// Added 是外部新增（内置不存在同名）的 manifest 名称列表。
	Added []string
}

// ScanManifests 递归扫描 dir 目录下的 .json manifest 文件，
// 解析并校验后返回有效的 AgentManifest 列表。
//
// 参数:
//   - dir: 待扫描的根目录。如果目录不存在或为空，返回空列表而非错误。
//
// 返回:
//   - manifests: 成功解析的 manifest 列表
//   - errs: 非致命错误列表（文件损坏、格式错误等仍返回但不中断流程）
//   - err: 致命错误（目录不可读等使扫描完全失败的错误）
func ScanManifests(dir string) ([]AgentManifest, []ManifestLoadError, error) {
	if dir == "" {
		return nil, nil, nil
	}

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil // 目录不存在不是致命错误
		}
		return nil, nil, fmt.Errorf("scan manifests: %w", err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("scan manifests: %q 不是目录", dir)
	}

	return scanManifestsFromPath(dir)
}

// ScanManifestsFromFS 从任意 fs.FS 扫描 .json manifest 文件。
// 用于从 embed.FS 或 os.DirFS 加载，目录树的根即 fsys 根。
func ScanManifestsFromFS(fsys fs.FS) ([]AgentManifest, []ManifestLoadError, error) {
	return scanManifestsFS(fsys, ".")
}

// scanManifestsFromPath 扫描磁盘 dir 目录（os.DirFS 语义），返回解析后的列表。
func scanManifestsFromPath(dir string) ([]AgentManifest, []ManifestLoadError, error) {
	fsys := os.DirFS(dir)
	return scanManifestsFS(fsys, ".")
}

// scanManifestsFS 在 fsys 的 root 子树下扫描 .json 文件并解析为 manifest。
// root 用 "." 表示 fsys 根。非致命错误（单文件解析/校验失败）收集到 errs，
// 不中断整体流程；致命错误（walk 失败）通过 err 返回。
func scanManifestsFS(fsys fs.FS, root string) ([]AgentManifest, []ManifestLoadError, error) {
	var manifests []AgentManifest
	var loadErrs []ManifestLoadError

	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".json") {
			return nil
		}

		data, readErr := fs.ReadFile(fsys, path)
		if readErr != nil {
			loadErrs = append(loadErrs, ManifestLoadError{
				Path:  path,
				Error: fmt.Sprintf("读取文件失败: %v", readErr),
			})
			return nil // 非致命，继续处理其他文件
		}

		var m AgentManifest
		if unmarshalErr := json.Unmarshal(data, &m); unmarshalErr != nil {
			loadErrs = append(loadErrs, ManifestLoadError{
				Path:  path,
				Error: fmt.Sprintf("JSON 解析失败: %v", unmarshalErr),
			})
			return nil
		}

		if validateErr := ValidateManifest(m); validateErr != nil {
			loadErrs = append(loadErrs, ManifestLoadError{
				Path:  path,
				Error: validateErr.Error(),
			})
			return nil
		}

		manifests = append(manifests, m)
		return nil
	})
	if err != nil {
		return manifests, loadErrs, fmt.Errorf("scan manifests walk: %w", err)
	}

	// 按名称排序以保证确定性
	sort.Slice(manifests, func(i, j int) bool {
		return manifests[i].Name < manifests[j].Name
	})

	return manifests, loadErrs, nil
}

// LoadManifests 合并加载内置 manifest（go:embed）与外部用户目录的 manifest。
//
// 加载语义：
//  1. 先从 embeddedManifestsFS 加载全部内置 manifest（按 Name 建索引）
//  2. 若 userDir 非空且存在，扫描其中的 .json 文件：
//     - 与内置同名 → 覆盖内置版本（记入 Overridden）
//     - 内置无同名 → 追加新增（记入 Added）
//  3. 合并后按 Name 排序返回
//
// userDir 为空或不存在时，仅返回内置 manifest。
// 单文件解析/校验失败不中断流程，记入 Errors。
//
// 这使得 mady 在任意工作目录启动都能加载内置领域定义（开箱即用），
// 同时支持用户在 MADY_HOME/manifests/ 放自定义文件覆盖或新增领域（无需重编译）。
func LoadManifests(userDir string) ManifestMergeResult {
	res := ManifestMergeResult{}

	// 1. 内置 manifest（embed.FS 中的 manifests 子目录）
	embedded, errs, err := scanManifestsFS(embeddedManifestsFS, embeddedManifestsDir)
	res.Errors = append(res.Errors, errs...)
	res.EmbeddedCount = len(embedded)
	if err != nil {
		// embed.FS 是编译期内嵌的，走到这里说明二进制构建异常，记为致命但继续
		res.Errors = append(res.Errors, ManifestLoadError{
			Path:  "embedded:" + embeddedManifestsDir,
			Error: fmt.Sprintf("扫描内置 manifest 失败: %v", err),
		})
	}

	byName := make(map[string]AgentManifest, len(embedded))
	for _, m := range embedded {
		byName[m.Name] = m
	}

	// 2. 外部用户目录（可选）
	if userDir != "" {
		if info, statErr := os.Stat(userDir); statErr == nil && info.IsDir() {
			external, extErrs, extErr := scanManifestsFromPath(userDir)
			res.Errors = append(res.Errors, extErrs...)
			res.ExternalCount = len(external)
			if extErr != nil {
				res.Errors = append(res.Errors, ManifestLoadError{
					Path:  userDir,
					Error: fmt.Sprintf("扫描外部 manifest 失败: %v", extErr),
				})
			}
			for _, m := range external {
				if _, exists := byName[m.Name]; exists {
					res.Overridden = append(res.Overridden, m.Name)
				} else {
					res.Added = append(res.Added, m.Name)
				}
				byName[m.Name] = m // 覆盖或新增
			}
		}
		// userDir 不存在时静默跳过（开箱即用靠 embed 保证）
	}

	// 3. 合并 → 排序列表
	merged := make([]AgentManifest, 0, len(byName))
	for _, m := range byName {
		merged = append(merged, m)
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Name < merged[j].Name
	})
	res.Manifests = merged

	return res
}
