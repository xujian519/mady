package deliver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"time"
)

// maxDeliverableBytes 是单个交付件的读取上限（64 MB）。
//
// 交付核对要读模型给出的任意路径：不设上限时，管道或字符设备会让读取挂起或耗尽内存。
// 正常的专利交付件（DOCX/PDF/HTML）远低于该上限。
const maxDeliverableBytes = 64 << 20

// Artifact 是一份交付物在两个位置的副本。
type Artifact struct {
	// Role 是文件角色（如「权利要求书」）。
	Role string
	// CanonicalPath 是读者最先打开的案卷主路径。
	CanonicalPath string
	// DeliveredPath 是本次交付发出的副本。
	DeliveredPath string
}

// Requirement 是本次交付必须交代的一项要求及其证据文件。
type Requirement struct {
	// Requirement 是指示方的原话。
	Requirement string
	// Evidence 是支撑该要求的文件路径；为空表示没有记录证据。
	Evidence []string
}

// VerifyInput 是交付件核对的输入。
type VerifyInput struct {
	// Artifacts 是待核对的主路径与交付副本。
	Artifacts []Artifact
	// Figures 是本次交付的附图。
	Figures []string
	// Rendered 是渲染出的交付件（html/pdf），可为空。
	Rendered string
	// Requirements 是本次交付要交代的要求。
	Requirements []Requirement
}

// ManifestEntry 是交付清单中的一条。
type ManifestEntry struct {
	// Role 是文件角色。
	Role string `json:"role"`
	// Path 是文件路径。
	Path string `json:"path"`
	// SHA256 是文件内容摘要。
	SHA256 string `json:"sha256"`
	// Size 是字节数。
	Size int64 `json:"size"`
	// ModTime 是修改时间。
	ModTime time.Time `json:"mod_time"`
}

// VerifyResult 是核对结果。
type VerifyResult struct {
	// Passed 表示全部确定性检查通过。
	Passed bool
	// Manifest 是交付清单（输入文件的摘要与时间）。
	Manifest []ManifestEntry
	// Issues 是未通过项。
	Issues []string
}

// VerifyDeliverable 判定交付件是否自洽。
//
// 两类缺陷来自真实撰写过程：主路径仍是被替换过的旧稿，读者打开主路径看到旧文本；
// 渲染产物早于它要展示的输入，交付件里是改稿前的内容。两者都按确定性规则判定，
// 不依赖模型自述。
func VerifyDeliverable(input VerifyInput) (VerifyResult, error) {
	result := VerifyResult{Passed: true}

	artifactTimes := make(map[string]time.Time, len(input.Artifacts)*2)
	for _, artifact := range input.Artifacts {
		canonical, err := statArtifact(artifact.Role+"（主路径）", artifact.CanonicalPath)
		if err != nil {
			result.Issues = append(result.Issues, err.Error())
			continue
		}
		delivered, err := statArtifact(artifact.Role+"（交付副本）", artifact.DeliveredPath)
		if err != nil {
			result.Issues = append(result.Issues, err.Error())
			continue
		}
		result.Manifest = append(result.Manifest, canonical, delivered)
		artifactTimes[artifact.CanonicalPath] = canonical.ModTime
		artifactTimes[artifact.DeliveredPath] = delivered.ModTime

		if canonical.SHA256 != delivered.SHA256 {
			result.Issues = append(result.Issues, fmt.Sprintf(
				"%s：主路径与交付副本内容不一致，读者打开主路径看到的不是本次交付的版本", artifact.Role))
		}
	}

	for _, figure := range input.Figures {
		entry, err := statArtifact("附图", figure)
		if err != nil {
			result.Issues = append(result.Issues, err.Error())
			continue
		}
		result.Manifest = append(result.Manifest, entry)
		artifactTimes[figure] = entry.ModTime
	}

	for _, requirement := range input.Requirements {
		if len(requirement.Evidence) == 0 {
			result.Issues = append(result.Issues, fmt.Sprintf(
				"要求「%s」没有记录证据文件", requirement.Requirement))
			continue
		}
		for _, evidence := range requirement.Evidence {
			if _, err := statArtifact("证据", evidence); err != nil {
				result.Issues = append(result.Issues, fmt.Sprintf(
					"要求「%s」的证据缺失：%v", requirement.Requirement, err))
			}
		}
	}

	if input.Rendered != "" {
		rendered, err := statArtifact("交付件", input.Rendered)
		if err != nil {
			result.Issues = append(result.Issues, err.Error())
		} else {
			result.Manifest = append(result.Manifest, rendered)
			for path, modified := range artifactTimes {
				if rendered.ModTime.Before(modified) {
					result.Issues = append(result.Issues, fmt.Sprintf(
						"交付件早于其输入 %s 修改：交付件里是改稿前的内容", path))
				}
			}
		}
	}

	result.Passed = len(result.Issues) == 0
	return result, nil
}

// statArtifact 读取文件摘要与时间。
func statArtifact(role, path string) (ManifestEntry, error) {
	if path == "" {
		return ManifestEntry{}, fmt.Errorf("%s：未指定路径", role)
	}
	info, err := os.Stat(path)
	if err != nil {
		return ManifestEntry{}, fmt.Errorf("%s：%s 不可读：%v", role, path, err)
	}
	if !info.Mode().IsRegular() {
		return ManifestEntry{}, fmt.Errorf("%s：%s 不是普通文件", role, path)
	}
	if info.Size() > maxDeliverableBytes {
		return ManifestEntry{}, fmt.Errorf("%s：%s 超过 %d MB 上限", role, path, maxDeliverableBytes/(1<<20))
	}
	//nolint:gosec // path 由调用方给出，是本次交付的输入文件
	data, err := os.ReadFile(path)
	if err != nil {
		return ManifestEntry{}, fmt.Errorf("%s：读取 %s 失败：%v", role, path, err)
	}
	sum := sha256.Sum256(data)
	return ManifestEntry{
		Role:    role,
		Path:    path,
		SHA256:  hex.EncodeToString(sum[:]),
		Size:    info.Size(),
		ModTime: info.ModTime().UTC(),
	}, nil
}
