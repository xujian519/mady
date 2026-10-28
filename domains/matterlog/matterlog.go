package matterlog

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// LogFileName 是案卷事件日志的文件名。
const LogFileName = "_matter-log.md"

// Actions 是日志条目的动作闭集。
var Actions = []string{"建案", "检索", "分析", "撰写", "门禁", "修订", "交付", "归档"}

// producingActions 是产生新产物的动作：这类轮次必须另存新文件，产物路径不得复用。
var producingActions = map[string]bool{"撰写": true, "修订": true}

const noneMarker = "—"

// Request 是一轮作业的留档声明。
type Request struct {
	Action     string   `json:"action"`
	Artifacts  []string `json:"artifacts"`
	Changes    string   `json:"changes"`
	Supersedes []string `json:"supersedes,omitempty"`
	Approver   string   `json:"approver,omitempty"`
	Note       string   `json:"note,omitempty"`
}

// Entry 是从日志中解析出的一条结构化记录。
type Entry struct {
	Line       int      `json:"line"`
	LocalTime  string   `json:"local_time"`
	Action     string   `json:"action"`
	Artifacts  []string `json:"artifacts"`
	Supersedes []string `json:"supersedes"`
}

// Defect 是一条留档缺陷。
type Defect struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// AppendResult 是一次追加的结果。
type AppendResult struct {
	LogPath      string   `json:"log_path"`
	Entry        string   `json:"entry"`
	PriorEntries int      `json:"prior_entries"`
	Superseded   []string `json:"superseded,omitempty"`
}

var (
	entryLineRE = regexp.MustCompile(`^- (\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[+-]\d{2}:\d{2}) \| UTC (\S+) \| (\S+) \| 产物 (.+?) \| 取代 (.+?) \| ([^|]*?) \| (.+)$`)
	lineTokenRE = regexp.MustCompile(`[^\s|、]+`)
)

// ValidAction 报告动作是否在闭集内。
func ValidAction(action string) bool { return slices.Contains(Actions, action) }

// ParseEntries 解析日志中的全部结构化条目（格式外的行跳过）。
func ParseEntries(existing string) []Entry {
	var entries []Entry
	for index, line := range strings.Split(existing, "\n") {
		match := entryLineRE.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		entries = append(entries, Entry{
			Line:       index + 1,
			LocalTime:  match[1],
			Action:     match[3],
			Artifacts:  fieldPaths(match[4]),
			Supersedes: fieldPaths(match[5]),
		})
	}
	return entries
}

func fieldPaths(value string) []string {
	if value == noneMarker {
		return nil
	}
	var paths []string
	for part := range strings.SplitSeq(value, "、") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			paths = append(paths, trimmed)
		}
	}
	return paths
}

// loggedPaths 收集日志中已登记的路径：结构化条目的产物/取代列，以及格式外行里
// 含「/」的路径形状 token（兼容本工具出现之前由通用文件工具写入的日志）。
func loggedPaths(existing string) map[string]bool {
	paths := make(map[string]bool)
	for _, entry := range ParseEntries(existing) {
		for _, path := range entry.Artifacts {
			paths[path] = true
		}
		for _, path := range entry.Supersedes {
			paths[path] = true
		}
	}
	for line := range strings.SplitSeq(existing, "\n") {
		if entryLineRE.MatchString(line) {
			continue
		}
		for _, token := range lineTokenRE.FindAllString(line, -1) {
			if strings.Contains(token, "/") {
				paths[token] = true
			}
		}
	}
	return paths
}

// Defects 判定一轮声明与日志、文件系统是否冲突；空表示可以追加。
func Defects(existing string, request Request, pathExists func(string) bool) []Defect {
	var defects []Defect
	if len(request.Artifacts) == 0 {
		defects = append(defects, Defect{Rule: "no_artifacts",
			Message: "未给出本轮产物：留档条目必须点明本轮的产物文件路径。"})
	}
	if strings.TrimSpace(request.Changes) == "" {
		defects = append(defects, Defect{Rule: "no_changes",
			Message: "未给出本轮变更说明：写清改了什么、为什么、是否影响保护点，这是留档段要交代的内容。"})
	}
	for _, path := range request.Artifacts {
		if pathExists(path) {
			continue
		}
		defects = append(defects, Defect{Rule: "artifact_missing",
			Message: path + " 不存在：日志记事实不记计划，先把产物写出来再留档。"})
	}
	for _, path := range request.Supersedes {
		if !slices.Contains(request.Artifacts, path) {
			continue
		}
		defects = append(defects, Defect{Rule: "overwrite_in_place",
			Message: path + " 同时是本轮产物与被取代件：交付物一律另存新文件、禁止覆盖上一轮，请改用新路径并在 supersedes 里写旧路径。"})
	}
	recorded := loggedPaths(existing)
	supersededBefore := make(map[string]bool)
	for _, entry := range ParseEntries(existing) {
		for _, path := range entry.Supersedes {
			supersededBefore[path] = true
		}
	}
	for _, path := range request.Supersedes {
		if !recorded[path] {
			defects = append(defects, Defect{Rule: "unknown_supersede",
				Message: "supersedes 里的 " + path + " 在日志中没有先出现的记录：取代链必须指向既有产物，不能凭空引用。"})
		}
		if supersededBefore[path] {
			defects = append(defects, Defect{Rule: "forked_chain",
				Message: path + " 已被更早的条目取代过：一个旧版只能有一个取代者，取代链不得分叉。"})
		}
	}
	if producingActions[request.Action] {
		for _, path := range request.Artifacts {
			if !recorded[path] {
				continue
			}
			defects = append(defects, Defect{Rule: "reused_artifact_path",
				Message: request.Action + " 轮的产物 " + path + " 已在日志中登记过：修订必须另存新文件，用新路径重写并把旧路径写进 supersedes。"})
		}
	}
	return defects
}

// RenderEntry 渲染一条日志行（不含行尾换行）。自由文本放在最后一个字段，
// 其中出现分隔符也不会移动其他字段。
func RenderEntry(request Request, now time.Time) string {
	artifacts := strings.Join(request.Artifacts, "、")
	supersedes := strings.Join(request.Supersedes, "、")
	if supersedes == "" {
		supersedes = noneMarker
	}
	approver := request.Approver
	if approver == "" {
		approver = noneMarker
	}
	changes := request.Changes
	if request.Note != "" {
		changes += "（备注：" + request.Note + "）"
	}
	return fmt.Sprintf("- %s | UTC %s | %s | 产物 %s | 取代 %s | %s | %s",
		now.Format("2006-01-02T15:04:05-07:00"), now.UTC().Format(time.RFC3339),
		request.Action, artifacts, supersedes, approver, changes)
}
