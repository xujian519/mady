package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	agentcore_evidence "github.com/xujian519/mady/agentcore/evidence"
	"github.com/xujian519/mady/domains/evidence"
)

func runEvidenceCLI(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("evidence: missing action (use -h for help)")
	}

	action := args[0]
	var filePath string
	remaining := args[1:]
	for i := 0; i < len(remaining); i++ {
		if remaining[i] == "--file" && i+1 < len(remaining) {
			filePath = remaining[i+1]
			i++
		}
	}

	var input io.Reader
	var openedFile *os.File
	if filePath != "" {
		f, err := os.Open(filepath.Clean(filePath))
		if err != nil {
			return fmt.Errorf("evidence: 无法打开文件: %w", err)
		}
		openedFile = f
		input = f
	} else {
		input = os.Stdin
	}

	exitCode := runEvidenceAction(action, input, os.Stdout, os.Stderr)
	if openedFile != nil {
		if err := openedFile.Close(); err != nil {
			log.Printf("close evidence file: %v", err)
		}
	}
	if exitCode != 0 {
		return fmt.Errorf("evidence: action %s failed", action)
	}
	return nil
}

func runEvidenceAction(action string, input io.Reader, stdout, stderr io.Writer) int {
	data, err := io.ReadAll(input)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "读取输入失败: %v\n", err)
		return 1
	}
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		_, _ = fmt.Fprintf(stderr, "输入为空\n")
		return 1
	}

	engine := evidence.NewEngine(nil)

	switch action {
	case "triple":
		return execTriple(engine, data, stdout, stderr)
	case "burden":
		return execBurden(data, stdout, stderr)
	case "standard":
		return execStandard(data, stdout, stderr)
	case "conflict":
		return execConflict(data, stdout, stderr)
	case "type-specific":
		return execTypeSpecific(engine, data, stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "未知 action: %s\n可用: triple, burden, standard, conflict, type-specific\n", action)
		return 1
	}
}

func execTriple(engine *evidence.DefaultEngine, data []byte, stdout, stderr io.Writer) int {
	var args struct {
		SourceURI string `json:"source_uri"`
		Snippet   string `json:"snippet"`
	}
	if err := json.Unmarshal(data, &args); err != nil {
		_, _ = fmt.Fprintf(stderr, "JSON 解析失败: %v\n", err)
		return 1
	}
	if args.Snippet == "" {
		_, _ = fmt.Fprintln(stderr, "缺少必填字段: snippet")
		return 1
	}
	span := agentcore_evidence.EvidenceSpan{
		ID:        "cli_triple_input",
		SourceURI: args.SourceURI,
		Snippet:   args.Snippet,
	}
	judgment, err := engine.Judge(span)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "判断失败: %v\n", err)
		return 1
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	//nolint:errchkjson // cliJudgmentToMap returns map[string]any (dynamic)
	if err := enc.Encode(cliJudgmentToMap(judgment)); err != nil {
		slog.Warn("evidence: encode judgment", "err", err)
	}
	return 0
}

func execBurden(data []byte, stdout, stderr io.Writer) int {
	var args struct {
		Scenario string `json:"scenario"`
	}
	if err := json.Unmarshal(data, &args); err != nil {
		_, _ = fmt.Fprintf(stderr, "JSON 解析失败: %v\n", err)
		return 1
	}
	if args.Scenario == "" {
		_, _ = fmt.Fprintln(stderr, "缺少必填字段: scenario")
		return 1
	}
	result := evidence.DetermineBurden(evidence.BurdenScenario(args.Scenario), nil)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	//nolint:errchkjson // map[string]any is inherently dynamic
	if err := enc.Encode(map[string]any{
		"holder": result.BurdenHolder, "standard": result.Standard,
		"has_shifted": result.HasShifted, "shift_reason": result.ShiftReason,
		"reasoning": result.Reasoning,
	}); err != nil {
		slog.Warn("evidence: encode burden result", "err", err)
	}
	return 0
}

func execStandard(data []byte, stdout, stderr io.Writer) int {
	var args struct {
		Standard        string   `json:"standard"`
		SupportingCount int      `json:"supporting_count"`
		TotalCount      int      `json:"total_count"`
		Gaps            []string `json:"gaps"`
	}
	if err := json.Unmarshal(data, &args); err != nil {
		_, _ = fmt.Fprintf(stderr, "JSON 解析失败: %v\n", err)
		return 1
	}
	if args.Standard == "" {
		_, _ = fmt.Fprintln(stderr, "缺少必填字段: standard")
		return 1
	}
	result := evidence.AssessProofStandard(evidence.StandardOfProof(args.Standard), args.SupportingCount, args.TotalCount, args.Gaps)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	//nolint:errchkjson // map[string]any is inherently dynamic
	if err := enc.Encode(map[string]any{
		"met": result.Met, "standard": result.Standard, "confidence": result.Confidence,
		"supporting_count": result.SupportingCount, "contradicting_count": result.ContradictingCount,
		"reasoning": result.Reasoning, "gaps": result.Gaps,
	}); err != nil {
		slog.Warn("evidence: encode standard result", "err", err)
	}
	return 0
}

func execConflict(data []byte, stdout, stderr io.Writer) int {
	var args struct {
		Claims []struct {
			ClaimID       string   `json:"claim_id"`
			Supporting    []string `json:"supporting"`
			Contradicting []string `json:"contradicting"`
		} `json:"claims"`
	}
	if err := json.Unmarshal(data, &args); err != nil {
		_, _ = fmt.Fprintf(stderr, "JSON 解析失败: %v\n", err)
		return 1
	}
	cb := agentcore_evidence.NewClaimBinding()
	for _, c := range args.Claims {
		for _, sid := range c.Supporting {
			cb.RegisterSpan(agentcore_evidence.EvidenceSpan{
				ID: sid, Direction: agentcore_evidence.DirectionSupporting, ClaimRefs: []string{c.ClaimID},
			})
		}
		for _, sid := range c.Contradicting {
			cb.RegisterSpan(agentcore_evidence.EvidenceSpan{
				ID: sid, Direction: agentcore_evidence.DirectionContradicting, ClaimRefs: []string{c.ClaimID},
			})
		}
	}
	detector := agentcore_evidence.NewConflictDetector(cb)
	conflicts := detector.Detect()
	var out []map[string]any
	for _, c := range conflicts {
		out = append(out, map[string]any{
			"type": string(c.Type), "description": c.Description, "span_ids": c.SpanIDs,
		})
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	//nolint:errchkjson // map[string]any is inherently dynamic
	if err := enc.Encode(map[string]any{"conflicts": out}); err != nil {
		slog.Warn("evidence: encode conflict result", "err", err)
	}
	return 0
}

func execTypeSpecific(engine *evidence.DefaultEngine, data []byte, stdout, stderr io.Writer) int {
	var args struct {
		SourceURI string `json:"source_uri"`
	}
	if err := json.Unmarshal(data, &args); err != nil {
		_, _ = fmt.Fprintf(stderr, "JSON 解析失败: %v\n", err)
		return 1
	}
	if args.SourceURI == "" {
		_, _ = fmt.Fprintln(stderr, "缺少必填字段: source_uri")
		return 1
	}
	span := agentcore_evidence.EvidenceSpan{ID: "cli_type_input", SourceURI: args.SourceURI}
	judgment, err := engine.Judge(span)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "判断失败: %v\n", err)
		return 1
	}
	ts := judgment.TypeSpecificJudgment
	if ts == nil {
		//nolint:errchkjson // map[string]any is inherently dynamic
		if err := json.NewEncoder(stdout).Encode(map[string]any{"note": "无类型特定判断结果"}); err != nil {
			slog.Warn("evidence: encode type-specific judgment note", "err", err)
		}
		return 0
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	//nolint:errchkjson // cliTypeSpecificToMap returns map[string]any (dynamic)
	if err := enc.Encode(cliTypeSpecificToMap(ts)); err != nil {
		slog.Warn("evidence: encode type-specific result", "err", err)
	}
	return 0
}

func cliJudgmentToMap(j *evidence.EvidenceJudgment) map[string]any {
	m := map[string]any{"overall_score": j.OverallScore, "confidence": j.Confidence, "reasoning": j.Reasoning}
	if j.RelevanceJudgment != nil {
		m["relevance"] = map[string]any{"score": j.RelevanceJudgment.Score, "level": j.RelevanceJudgment.Level, "reasoning": j.RelevanceJudgment.Reasoning}
	}
	if j.LegalityJudgment != nil {
		m["legality"] = map[string]any{"score": j.LegalityJudgment.Score, "level": j.LegalityJudgment.Level, "reasoning": j.LegalityJudgment.Reasoning}
	}
	if j.AuthenticityJudgment != nil {
		m["authenticity"] = map[string]any{"score": j.AuthenticityJudgment.Score, "level": j.AuthenticityJudgment.Level, "reasoning": j.AuthenticityJudgment.Reasoning}
	}
	issues := make([]map[string]string, 0, len(j.FlaggedIssues))
	for _, issue := range j.FlaggedIssues {
		issues = append(issues, map[string]string{"type": issue.Type, "description": issue.Description, "severity": issue.Severity})
	}
	if len(issues) > 0 {
		m["issues"] = issues
	}
	return m
}

func cliTypeSpecificToMap(ts *evidence.TypeSpecificJudgment) map[string]any {
	m := map[string]any{"evidence_type": string(ts.EvidenceType)}
	if ts.PlatformCredibility != nil {
		m["platform_credibility"] = string(*ts.PlatformCredibility)
	}
	if ts.ContentIntegrity != "" {
		m["content_integrity"] = string(ts.ContentIntegrity)
	}
	if ts.PublicIntent != "" {
		m["public_intent"] = string(ts.PublicIntent)
	}
	if ts.FourElementsCheck != nil {
		fec := ts.FourElementsCheck
		m["four_elements_check"] = map[string]any{
			"time":          map[string]any{"met": fec.TimeElement.Met, "score": fec.TimeElement.Score, "detail": fec.TimeElement.Detail},
			"place":         map[string]any{"met": fec.PlaceElement.Met, "score": fec.PlaceElement.Score, "detail": fec.PlaceElement.Detail},
			"method":        map[string]any{"met": fec.MethodElement.Met, "score": fec.MethodElement.Score, "detail": fec.MethodElement.Detail},
			"accessibility": map[string]any{"met": fec.Accessibility.Met, "score": fec.Accessibility.Score, "detail": fec.Accessibility.Detail},
		}
	}
	return m
}
