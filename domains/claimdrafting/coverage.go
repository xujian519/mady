package claimdrafting

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MAXClaimNo 是单份申请允许的最大权利要求编号（确定性上限，防溢出）。
const MAXClaimNo = 1000

var claimIDRe = regexp.MustCompile(`^claim_(\d+)$`)

// ClaimCoverageEntry 描述某个权利要求的特征与其所引用的实施例。
// Coverage 与 UncoveredFeatures 为调用方（LLM）预填的判读列；本模型不以此判定，
// 只信 Features 与 EmbodimentRefs 两列（Sati 确定性纪律）。
type ClaimCoverageEntry struct {
	ClaimID           string
	Features          []string
	EmbodimentRefs    []string
	Coverage          string // full|partial|none（LLM 判读，本模型忽略）
	UncoveredFeatures []string
}

// CoverageItem 是单条权利要求核验结果。
type CoverageItem struct {
	Entry          ClaimCoverageEntry
	ClaimNum       int
	Valid          bool
	InvalidReason  string
	ActualCoverage string // full|partial|none
	Uncovered      []string
}

// CoverageReport 是一次覆盖核验的汇总。
type CoverageReport struct {
	Items        []CoverageItem
	FullCount    int
	PartialCount int
	NoneCount    int
	Covered      int
	Uncovered    int
	Gaps         []int // 检测到的权利要求编号断档
}

// CoverageChecker 对权利要求覆盖做确定性核验。
type CoverageChecker struct{}

// NewCoverageChecker 创建覆盖核验器。
func NewCoverageChecker() *CoverageChecker { return &CoverageChecker{} }

// Check 校验每条 claim 的 feature 是否被 EmbodimentRefs 支持，并检测断号/超限/重复。
// claims 为权利要求原文列表（用于数量上限参考，可空）。
func (c *CoverageChecker) Check(claims []string, entries []ClaimCoverageEntry) CoverageReport {
	report := CoverageReport{}
	maxByClaims := len(claims)

	allValid := true
	nums := []int{}
	for _, e := range entries {
		item := checkEntry(e, maxByClaims)
		report.Items = append(report.Items, item)
		if !item.Valid {
			allValid = false
			continue
		}
		switch item.ActualCoverage {
		case "full":
			report.FullCount++
		case "partial":
			report.PartialCount++
		case "none":
			report.NoneCount++
		}
		report.Covered += len(item.Entry.Features) - len(item.Uncovered)
		report.Uncovered += len(item.Uncovered)
		nums = append(nums, item.ClaimNum)
	}

	// 断号检测：仅当全部条目编号均有效时才推断缺口。一旦存在无效条目（如
	// claim_x 格式非法），其本应占据的编号未可知——此时声称某编号「缺失」会
	// 把撰写人引向补一条已存在的权利要求。此类无效条目已在 report.Items 中
	// 逐条标注 InvalidReason，应优先修复而非推断断档。
	if allValid {
		sort.Ints(nums)
		for i := 1; i < len(nums); i++ {
			gap := nums[i] - nums[i-1]
			if gap > 1 {
				for n := nums[i-1] + 1; n < nums[i]; n++ {
					report.Gaps = append(report.Gaps, n)
				}
			}
		}
	}
	return report
}

func checkEntry(e ClaimCoverageEntry, maxByClaims int) CoverageItem {
	// reject 统一非法分支的返回形态（Entry + 空 Uncovered + InvalidReason）。
	reject := func(reason string) CoverageItem {
		return CoverageItem{Entry: e, Uncovered: []string{}, InvalidReason: reason}
	}
	m := claimIDRe.FindStringSubmatch(e.ClaimID)
	if m == nil {
		return reject("claim id 格式非法（应为 claim_<n>）")
	}
	num, err := strconv.Atoi(m[1])
	if err != nil {
		// 正则 ^claim_(\d+)$ 已限定数字串，仅溢出时失败；统一按非法编号拒绝。
		return reject("claim id 编号非法")
	}
	if num <= 0 || num > MAXClaimNo {
		return reject("claim 编号超出上限（1..1000）")
	}
	if maxByClaims > 0 && num > maxByClaims {
		return reject("claim 编号超出权利要求数量")
	}
	item := CoverageItem{Entry: e, ClaimNum: num, Valid: true, Uncovered: []string{}}

	// 只信 Features + EmbodimentRefs：重复特征去重，按实施例支持判定覆盖。
	features := dedupeFeatures(e.Features)
	if len(e.EmbodimentRefs) == 0 {
		item.ActualCoverage = "none"
		item.Uncovered = features
		return item
	}
	var uncovered []string
	for _, f := range features {
		if !featureCovered(f, e.EmbodimentRefs) {
			uncovered = append(uncovered, f)
		}
	}
	switch {
	case len(uncovered) == 0:
		item.ActualCoverage = "full"
	case len(uncovered) < len(features):
		item.ActualCoverage = "partial"
	default:
		item.ActualCoverage = "none"
	}
	item.Uncovered = uncovered
	return item
}

// featureCovered 判定 feature 的整串文本是否出现在任一 embodimentRef 中。
func featureCovered(feature string, refs []string) bool {
	f := strings.TrimSpace(feature)
	if f == "" {
		return true
	}
	joined := strings.Join(refs, " ")
	return strings.Contains(joined, f)
}

// dedupeFeatures 去重并保留顺序。
func dedupeFeatures(features []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, f := range features {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}
