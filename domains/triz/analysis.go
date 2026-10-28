package triz

import (
	"fmt"

	"github.com/xujian519/mady/domains/claimchart"
)

// 确定性组装：模型只负责「识别」矛盾与缺口；编号合法性（1-39）、矩阵落格、
// 参数与原理名称、证据能否在原文定位，全部由本文件判定。证据定位失败的一律
// 丢弃并计数——识别粒度越细，编造的矛盾越多，这是核验必须付出的代价。

// BuildAnalysis 把模型抽取结果组装成已核验的矛盾分析。
//
// sourceText 是交底书原文，用于证据定位核验（剥空白后逐字子串）。
func BuildAnalysis(extraction Extraction, sourceText string) (Analysis, error) {
	analysis := Analysis{
		Contradictions: make([]Contradiction, 0, len(extraction.Contradictions)),
	}
	for index, input := range extraction.Contradictions {
		statement := input.Statement
		if statement == "" {
			statement = fmt.Sprintf("第 %d 条矛盾", index+1)
		}
		if !validParameterNumber(input.Improving) || !validParameterNumber(input.Worsening) {
			analysis.Unmapped = append(analysis.Unmapped, UnmappedItem{
				Statement: statement,
				Reason: fmt.Sprintf("改善／恶化参数不是 %d-%d 的整数编号，无法落格到矛盾矩阵",
					MatrixMin, MatrixMax),
			})
			continue
		}
		if !claimchart.ValidateQuote(input.Evidence, sourceText) {
			analysis.DroppedForEvidence++
			analysis.Unmapped = append(analysis.Unmapped, UnmappedItem{
				Statement: statement,
				Reason:    "证据片段无法在交底书原文中定位，按无依据矛盾丢弃",
			})
			continue
		}
		status, principles, err := resolveMatrix(input.Improving, input.Worsening)
		if err != nil {
			return Analysis{}, err
		}
		analysis.Contradictions = append(analysis.Contradictions, Contradiction{
			ID:                 fmt.Sprintf("C%d", len(analysis.Contradictions)+1),
			Improving:          parameterRef(input.Improving),
			Worsening:          parameterRef(input.Worsening),
			Statement:          statement,
			Evidence:           input.Evidence,
			Principles:         principles,
			MatrixStatus:       status,
			SolutionDirections: nonEmpty(input.SolutionDirections),
		})
	}

	analysis.ParameterGaps = mergeGaps(extraction.ParameterGaps, &analysis.Unmapped)
	for _, item := range extraction.Unmapped {
		if item.Statement == "" {
			continue
		}
		reason := item.Reason
		if reason == "" {
			reason = "未映射到 1-39 工程参数"
		}
		analysis.Unmapped = append(analysis.Unmapped, UnmappedItem{Statement: item.Statement, Reason: reason})
	}
	return analysis, nil
}

func validParameterNumber(no int) bool { return no >= MatrixMin && no <= MatrixMax }

func parameterRef(no int) Parameter { return Parameter{Number: no, Name: ParamLabel(no)} }

// resolveMatrix 落格到经典矛盾矩阵：对角格为物理矛盾，空值为转录缺口，二者都不给原理。
func resolveMatrix(improving, worsening int) (MatrixStatus, []Principle, error) {
	if improving == worsening {
		return MatrixPhysical, nil, nil
	}
	ids, err := LookupCell(improving, worsening)
	if err != nil {
		return "", nil, err
	}
	if len(ids) == 0 {
		return MatrixGap, nil, nil
	}
	principles := make([]Principle, 0, len(ids))
	for _, no := range ids {
		principle, ok := PrincipleByNo(no)
		if !ok {
			return "", nil, fmt.Errorf("triz: 矩阵引用了不存在的发明原理 %d", no)
		}
		principles = append(principles, principle)
	}
	return MatrixRecommended, principles, nil
}

// mergeGaps 按参数编号合并缺口：同一参数多次点名时取维度并集，保留首条非空说明。
func mergeGaps(inputs []ParameterGapInput, unmapped *[]UnmappedItem) []ParameterGap {
	type merged struct {
		missing map[GapKind]bool
		detail  string
	}
	byParameter := make(map[int]*merged)
	var order []int
	for _, input := range inputs {
		missing := normalizeGapKinds(input.Missing)
		if !validParameterNumber(input.Parameter) || len(missing) == 0 {
			*unmapped = append(*unmapped, UnmappedItem{
				Statement: fmt.Sprintf("参数缺口条目（参数 %d）", input.Parameter),
				Reason:    "参数编号或缺失维度非法，未采纳",
			})
			continue
		}
		entry, seen := byParameter[input.Parameter]
		if !seen {
			entry = &merged{missing: make(map[GapKind]bool, len(missing))}
			byParameter[input.Parameter] = entry
			order = append(order, input.Parameter)
		}
		for _, kind := range missing {
			entry.missing[kind] = true
		}
		if entry.detail == "" {
			entry.detail = input.Detail
		}
	}
	gaps := make([]ParameterGap, 0, len(order))
	for _, no := range order {
		entry := byParameter[no]
		kinds := make([]GapKind, 0, len(entry.missing))
		for _, kind := range GapKinds {
			if entry.missing[kind] {
				kinds = append(kinds, kind)
			}
		}
		gaps = append(gaps, ParameterGap{
			Parameter: parameterRef(no),
			Missing:   kinds,
			Detail:    entry.detail,
		})
	}
	return gaps
}

// normalizeGapKinds 过滤非法维度并按全集顺序去重。
func normalizeGapKinds(kinds []GapKind) []GapKind {
	seen := make(map[GapKind]bool, len(kinds))
	for _, kind := range kinds {
		seen[kind] = true
	}
	out := make([]GapKind, 0, len(seen))
	for _, kind := range GapKinds {
		if seen[kind] {
			out = append(out, kind)
		}
	}
	return out
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
