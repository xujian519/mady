package subcmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout 将 os.Stdout 重定向到管道并返回读取函数，供断言 CLI 输出。
func captureStdout(t *testing.T) func() string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	return func() string {
		w.Close()
		os.Stdout = old
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, r); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
}

// TestRunPatentPipeline_PrintsOutput 验证无 -o 时输出到 stdout。
func TestRunPatentPipeline_PrintsOutput(t *testing.T) {
	read := captureStdout(t)
	err := runPatentPipeline([]string{"发明描述"}, "novelty", "usage", func(string) (string, error) {
		return "分析结果", nil
	}, nil)
	if err != nil {
		t.Fatalf("runPatentPipeline: %v", err)
	}
	out := read()
	if !strings.Contains(out, "分析结果") {
		t.Fatalf("stdout 应包含分析结果，实际: %q", out)
	}
}

// TestRunPatentPipeline_SavesToFile 验证 -o 时写入文件并提示保存位置。
func TestRunPatentPipeline_SavesToFile(t *testing.T) {
	dir := t.TempDir()
	outFile := dir + "/report.md"

	var savedOutput, savedFile string
	err := runPatentPipeline([]string{"-o", outFile, "内容"}, "novelty", "usage",
		func(string) (string, error) { return "报告正文", nil },
		func(output, file string) error {
			savedOutput, savedFile = output, file
			return nil
		})
	if err != nil {
		t.Fatalf("runPatentPipeline: %v", err)
	}
	if savedOutput != "报告正文" || savedFile != outFile {
		t.Fatalf("save 参数 = (%q, %q)", savedOutput, savedFile)
	}
}

// TestRunPatentPipeline_MissingInput 验证无输入时报用法错误。
func TestRunPatentPipeline_MissingInput(t *testing.T) {
	read := captureStdout(t)
	err := runPatentPipeline(nil, "novelty", "请提供输入", func(string) (string, error) {
		return "", nil
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "请提供输入") {
		t.Fatalf("期望缺输入错误，实际: %v", err)
	}
	read() // 丢弃 usage 输出
}

// TestRunPatentPipeline_BuildError 验证 build 失败时错误传播。
func TestRunPatentPipeline_BuildError(t *testing.T) {
	err := runPatentPipeline([]string{"x"}, "oa", "usage", func(string) (string, error) {
		return "", errors.New("boom")
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("期望 build 错误传播，实际: %v", err)
	}
}

// TestRunPatentPipeline_SaveError 验证保存失败时错误传播。
func TestRunPatentPipeline_SaveError(t *testing.T) {
	dir := t.TempDir()
	err := runPatentPipeline([]string{"-o", dir + "/x.md", "内容"}, "novelty", "usage",
		func(string) (string, error) { return "输出", nil },
		func(string, string) error { return errors.New("save failed") })
	if err == nil || !strings.Contains(err.Error(), "save failed") {
		t.Fatalf("期望保存错误传播，实际: %v", err)
	}
}
