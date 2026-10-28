package cnipr

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Session 是通道依赖的 ego 会话抽象：可用性检查 + 运行一段脚本体。
// 生产实现走 ego-browser CLI；测试用假会话注入成功/失败/超时。
type Session interface {
	// CheckAvailability 报告 ego 是否可用；不可用时给出去向说明。
	CheckAvailability() (bool, string)
	// RunScript 以 stdin 运行脚本体，返回输出（ego 的 console 走 stderr）。
	RunScript(script string, timeout time.Duration) (RunResult, error)
}

// RunResult 是一次脚本运行结果。
type RunResult struct {
	Output   string
	Stderr   string
	ExitCode int
	TimedOut bool
}

// outputTailLength 是收据缺失时附在原因里的输出长度。
const outputTailLength = 400

// Channel 是 CNIPR 通道：复用同一个 ego 任务空间，登录态由 ego lite 提供。
type Channel struct {
	session Session
	queue   *SerialQueue
	timeout time.Duration
	budgets PageBudgets
}

// Option 配置通道。
type Option func(*Channel)

// WithTimeout 设置单次脚本运行的期限。
func WithTimeout(timeout time.Duration) Option {
	return func(c *Channel) { c.timeout = timeout }
}

// WithPageBudgets 覆盖页面等待预算。
func WithPageBudgets(budgets PageBudgets) Option {
	return func(c *Channel) { c.budgets = budgets }
}

// NewChannel 创建通道。
func NewChannel(session Session, opts ...Option) *Channel {
	channel := &Channel{
		session: session, queue: &SerialQueue{},
		timeout: 180 * time.Second, budgets: DefaultPageBudgets,
	}
	for _, opt := range opts {
		opt(channel)
	}
	return channel
}

// Search 关键词检索；条数收敛到通道上限（30 条）。
func (c *Channel) Search(query string, limit int) (*SearchResult, error) {
	request := PlanSearchRequest(limit)
	payload, err := c.run(BuildSearchScript(query, request.Pages, c.budgets), tagSearch)
	if err != nil {
		return nil, err
	}
	total, hits, failure := readSearchPayload(payload)
	if failure != nil {
		return nil, &ChannelError{Code: failure.Code, Detail: failure.Message}
	}
	if len(hits) > request.Limit {
		hits = hits[:request.Limit]
	}
	return &SearchResult{Total: total, Hits: hits}, nil
}

// PDFLinks 按公开(公告)号取全文 PDF 直链；非公开号按条目报错。
func (c *Channel) PDFLinks(numbers []string) ([]PdfLinkItem, error) {
	requested, unsupported := PlanPDFRequests(numbers)
	if len(requested) == 0 {
		return alignPDF(numbers, unsupported), nil
	}
	payload, err := c.run(BuildPDFLinksScript(requested, c.budgets), tagPDFLinks)
	if err != nil {
		return nil, err
	}
	items, failure := readPdfPayload(payload)
	if failure != nil {
		return nil, &ChannelError{Code: failure.Code, Detail: failure.Message}
	}
	return alignPDF(numbers, append(items, unsupported...)), nil
}

// LegalStatus 查询法律状态：申请号直接查，公开号先解析申请号。
func (c *Channel) LegalStatus(numbers []string) ([]LegalStatusItem, error) {
	queries, unsupported := PlanLegalQueries(numbers)
	if len(queries) == 0 {
		return alignLegal(numbers, unsupported), nil
	}
	payload, err := c.run(BuildLegalScript(queries, c.budgets), tagLegal)
	if err != nil {
		return nil, err
	}
	items, failure := readLegalPayload(payload)
	if failure != nil {
		return nil, &ChannelError{Code: failure.Code, Detail: failure.Message}
	}
	return alignLegal(numbers, append(items, unsupported...)), nil
}

// run 串行执行一个脚本并取回收据；通道级失败在这里分类。
func (c *Channel) run(script, tag string) (map[string]any, error) {
	var payload map[string]any
	err := c.queue.Run(func() error {
		if c.session == nil {
			return &ChannelError{Code: FailureSetupRequired, Detail: "未配置 ego 会话"}
		}
		if ok, reason := c.session.CheckAvailability(); !ok {
			if reason == "" {
				reason = "ego-browser 不可用；请先安装 ego lite 并完成首次引导。"
			}
			return &ChannelError{Code: FailureSetupRequired, Detail: reason}
		}
		result, runErr := c.session.RunScript(script, c.timeout)
		if runErr != nil {
			return fmt.Errorf("CNIPR 脚本运行失败：%w", runErr)
		}
		if result.TimedOut {
			return &ChannelError{Code: FailureTimeout, Detail: fmt.Sprintf("CNIPR 脚本超出期限（exit %d）", result.ExitCode)}
		}
		if extracted, ok := ExtractTaggedJSON(result.Output, tag); ok {
			payload = extracted
			return nil
		}
		if extracted, ok := ExtractTaggedJSON(result.Stderr, tag); ok {
			payload = extracted
			return nil
		}
		tail := result.Output
		if len([]rune(tail)) > outputTailLength {
			tail = string([]rune(tail)[len([]rune(tail))-outputTailLength:])
		}
		return &ChannelError{Code: FailureReceiptMissing,
			Detail: fmt.Sprintf("脚本未回传 %s 收据（exit %d）：%s", tag, result.ExitCode, tail)}
	})
	if err != nil {
		return nil, err
	}
	return payload, nil
}

// EgoSession 是通过 ego-browser CLI 运行脚本的会话实现。
type EgoSession struct {
	// Executable 是 ego-browser 路径；为空时 CheckAvailability 判为不可用。
	Executable string
}

// CheckAvailability 报告 ego-browser 是否可用。
func (s *EgoSession) CheckAvailability() (bool, string) {
	if strings.TrimSpace(s.Executable) == "" {
		return false, "未找到 ego-browser：安装 ego lite 后重试（或设置 EGO_BROWSER_PATH）。"
	}
	return true, ""
}

// RunScript 以 stdin 交给 `ego-browser nodejs` 执行，合并两路输出。
func (s *EgoSession) RunScript(script string, timeout time.Duration) (RunResult, error) {
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.Executable, "nodejs") //nolint:gosec // G204: ego-browser 路径来自配置
	cmd.Stdin = strings.NewReader(script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := RunResult{Output: stdout.String(), Stderr: stderr.String()}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		return result, nil
	}
	if err != nil {
		// 脚本自身失败时仍可能已回传收据：把退出错误交给调用方按收据判断，
		// 只有完全拿不到收据时才由 run 报 receipt_missing。
		return result, nil
	}
	return result, nil
}
