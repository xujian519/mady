package inventiveness

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xujian519/mady/pkg/util"
)

// FeedbackAction 记录用户在 HITL 环节对创造性结论的操作类型。
type FeedbackAction string

const (
	// ActionRejection 用户驳回 AI 结论。
	ActionRejection FeedbackAction = "rejection"
	// ActionModification 用户修正了 AI 结论。
	ActionModification FeedbackAction = "modification"
)

// FeedbackEntry 是一次创造性结论的 HITL 用户反馈（驳回或修正）。
type FeedbackEntry struct {
	CaseID    string         `json:"case_id,omitempty"`
	Action    FeedbackAction `json:"action"`
	Reason    string         `json:"reason,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// InventivenessFeedbackFilename 是案卷级反馈文件名。
const InventivenessFeedbackFilename = "inventiveness-feedback.jsonl"

// CaseFeedbackDir 返回某案卷的反馈落盘目录（$MADY_HOME/cases/<caseID>/）。
// caseID 必须是不含路径分隔符的纯标识符，否则返回空串（禁用）。
func CaseFeedbackDir(caseID string) string {
	if caseID == "" {
		return ""
	}
	if filepath.Base(caseID) != caseID || caseID == "." || caseID == ".." {
		return ""
	}
	home, err := util.MadyHome()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "cases", caseID)
}

// FeedbackPath 返回案卷级反馈文件路径。
func FeedbackPath(caseDir string) string {
	return filepath.Join(caseDir, InventivenessFeedbackFilename)
}

// AppendInventivenessFeedback 追加一条 HITL 反馈到案卷级 JSONL。caseDir 为空时静默
// （反馈仅在有案卷上下文时才落盘）。返回错误以便数据层断言；调用方可忽略（fail-open）。
func AppendInventivenessFeedback(caseDir string, e FeedbackEntry) error {
	if caseDir == "" {
		return nil
	}
	if err := os.MkdirAll(caseDir, 0o700); err != nil {
		return err
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := util.OpenFile(FeedbackPath(caseDir), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

// LoadInventivenessFeedback 回读某案卷的全部历史反馈。文件不存在回读空列表。
func LoadInventivenessFeedback(caseDir string) ([]FeedbackEntry, error) {
	f, err := os.Open(FeedbackPath(caseDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 文件不存在：无历史反馈，返回空列表
		}
		return nil, err
	}
	defer f.Close()

	var entries []FeedbackEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e FeedbackEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue // skip malformed line, keep the rest
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

// feedbackActionLabel 把反馈类型映射为中文标签（驳回/修正）；未知类型回退原串以容错旧数据。
func feedbackActionLabel(a FeedbackAction) string {
	switch a {
	case ActionRejection:
		return "驳回"
	case ActionModification:
		return "修正"
	default:
		return string(a)
	}
}

// SummarizeInventivenessFeedback 把历史反馈压缩为给结论节点的提示词块。
// 无历史反馈时返回空串（调用方据此跳过注入）。
func SummarizeInventivenessFeedback(entries []FeedbackEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("**历史用户反馈（请吸取并据此修正你的判断）**：\n")
	for _, e := range entries {
		b.WriteString("- " + feedbackActionLabel(e.Action))
		if e.Reason != "" {
			b.WriteString("：" + e.Reason)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// FeedbackPrompt 按案卷加载历史反馈并生成提示词块；无历史反馈或案卷无效时返回空串。
// 供结论节点在 LLM 调用前注入。
func FeedbackPrompt(caseID string) string {
	dir := CaseFeedbackDir(caseID)
	if dir == "" {
		return ""
	}
	entries, err := LoadInventivenessFeedback(dir)
	if err != nil {
		return "" // 反馈加载失败：静默降级，不注入历史反馈
	}
	return SummarizeInventivenessFeedback(entries)
}
