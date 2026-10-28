package lawcite

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// SkeletonLaw 是条号骨架中的一部法规。
//
// 骨架只回答「这部法规有没有这一条」，不回答条文内容。Transcribed 表示该部法规
// 在骨架源里有条文转录，但骨架本身不携带条文，因此不能据此判为已核验。
type SkeletonLaw struct {
	// Name 是法律简称。
	Name string
	// Document 是法律全称。
	Document string
	// Revision 是修订版本。
	Revision string
	// MaxArticle 是条数上限；0 表示来源未声明。
	MaxArticle int
	// Transcribed 表示该部法规在骨架源中带条文转录。
	Transcribed bool
}

// Skeleton 是合并后的法规条号骨架。
type Skeleton struct {
	// GeneratedFrom 是骨架的来源说明。
	GeneratedFrom string
	byName        map[string]SkeletonLaw
	normalized    map[string]SkeletonLaw
}

// skeletonFile 是骨架索引的文件结构。
type skeletonFile struct {
	GeneratedFrom string           `json:"generatedFrom"`
	Count         int              `json:"count"`
	Laws          []skeletonLawRaw `json:"laws"`
}

// skeletonLawRaw 是骨架索引中的一条。
type skeletonLawRaw struct {
	Name        string `json:"name"`
	Document    string `json:"document"`
	Revision    string `json:"revision"`
	MaxArticle  int    `json:"maxArticle"`
	Transcribed bool   `json:"transcribed"`
}

// LoadSkeleton 解析骨架索引。
func LoadSkeleton(data []byte, origin string) (*Skeleton, error) {
	var file skeletonFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("lawcite: 解析条号骨架 %s: %w", origin, err)
	}
	skeleton := &Skeleton{
		GeneratedFrom: file.GeneratedFrom,
		byName:        make(map[string]SkeletonLaw, len(file.Laws)),
		normalized:    make(map[string]SkeletonLaw, len(file.Laws)),
	}
	for _, raw := range file.Laws {
		name := strings.TrimSpace(raw.Name)
		if name == "" {
			return nil, fmt.Errorf("lawcite: 条号骨架 %s 存在空名称条目", origin)
		}
		law := SkeletonLaw{
			Name:        name,
			Document:    raw.Document,
			Revision:    raw.Revision,
			MaxArticle:  raw.MaxArticle,
			Transcribed: raw.Transcribed,
		}
		skeleton.byName[name] = law
		key := normalizeLawName(name)
		// 归一化名冲突时保留先出现的条目：骨架按名称排序，先出现的更短、更常见。
		if _, exists := skeleton.normalized[key]; !exists {
			skeleton.normalized[key] = law
		}
	}
	if file.Count > 0 && file.Count != len(file.Laws) {
		return nil, fmt.Errorf("lawcite: 条号骨架 %s 声明 %d 部法规，实际 %d 部",
			origin, file.Count, len(file.Laws))
	}
	return skeleton, nil
}

// LoadSkeletonFile 从文件加载骨架索引。
//
// 文件可以是 gzip 压缩的：按前两个字节的 gzip 魔数识别，与扩展名无关。
func LoadSkeletonFile(path string) (*Skeleton, error) {
	//nolint:gosec // path 由调用方指定（$MADY_HOME 或仓库 data 目录），只读取该 JSON
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("lawcite: 读取条号骨架 %s: %w", path, err)
	}
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("lawcite: 解压条号骨架 %s: %w", path, err)
		}
		defer func() { _ = reader.Close() }()
		plain, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("lawcite: 解压条号骨架 %s: %w", path, err)
		}
		data = plain
	}
	return LoadSkeleton(data, path)
}

// Lookup 按法律简称或全称查找。
//
// 先按原样匹配，再按归一化名（去掉书名号、空白与「中华人民共和国」前缀）匹配。
func (s *Skeleton) Lookup(name string) (SkeletonLaw, bool) {
	if s == nil {
		return SkeletonLaw{}, false
	}
	name = strings.TrimSpace(name)
	if law, ok := s.byName[name]; ok {
		return law, true
	}
	law, ok := s.normalized[normalizeLawName(name)]
	return law, ok
}

// Len 返回骨架中的法规数量。
func (s *Skeleton) Len() int {
	if s == nil {
		return 0
	}
	return len(s.byName)
}

// Resolve 判定一条引用的条号是否存在。
//
// 骨架只能给出两种结论：条号存在（StatusExistOnly），或无法判定（StatusUnverified）。
// 它绝不会返回 StatusVerified——条文内容不在骨架里，据此断言条文含义会违背
// 「未转录不判已核验」的规则。条数上限也按未核验处理：骨架的上限来自自动抽取，
// 没有核验日期，不能用来断言某条不存在。
func (s *Skeleton) Resolve(lawName string, article int) Resolution {
	citation := Citation{Statute: StatuteUnknown, Article: article}
	if s == nil {
		return Resolution{Citation: citation, Status: StatusUnverified, Reason: "未加载条号骨架，未做核验"}
	}
	law, ok := s.Lookup(lawName)
	if !ok {
		return Resolution{
			Citation: citation,
			Status:   StatusUnverified,
			Reason:   fmt.Sprintf("《%s》不在条号骨架中，未做核验", lawName),
		}
	}
	if article <= 0 {
		return Resolution{Citation: citation, Status: StatusUnverified, Reason: "条号不合法，未做核验"}
	}
	if law.MaxArticle > 0 && article > law.MaxArticle {
		return Resolution{
			Citation: citation,
			Status:   StatusUnverified,
			Reason: fmt.Sprintf("《%s》第 %d 条超出骨架声明的上限 %d 条，但该上限未核验",
				law.Name, article, law.MaxArticle),
		}
	}
	return Resolution{
		Citation: citation,
		Status:   StatusExistOnly,
		Reason: fmt.Sprintf("《%s》第 %d 条条号存在（条号骨架），条文未转录，不能据此确认内容",
			law.Name, article),
	}
}

// normalizeLawName 归一化法律名称，便于全称与简称互相命中。
func normalizeLawName(name string) string {
	normalized := strings.TrimSpace(name)
	normalized = strings.Trim(normalized, "《》")
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "　", "")
	normalized = strings.TrimPrefix(normalized, "中华人民共和国")
	return normalized
}
