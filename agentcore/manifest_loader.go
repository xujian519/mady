package agentcore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ManifestLoadError 包含加载过程中遇到的非致命错误的详细信息。
// 调用方可以据此生成日志或诊断报告，而不中断整体加载流程。
type ManifestLoadError struct {
	Path  string // 出错的文件路径
	Error string // 错误描述
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

	var manifests []AgentManifest
	var loadErrs []ManifestLoadError

	err = filepath.Walk(dir, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fi.IsDir() {
			return nil
		}
		name := strings.ToLower(fi.Name())
		if !strings.HasSuffix(name, ".json") {
			return nil
		}

		data, readErr := os.ReadFile(path)
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
