// Package gatechecks 门禁自测（负控制自证）。
//
// 每条门禁规则必须有自己的 fixture 测试证明"它真的会失败"（红）与"正常时放行"（绿）：
//   - TestAichangelogFormatGate：AI 变更日志格式门禁（缺**背景**必拦 / 四字段齐全放行）
//   - TestSensitivePathsGate：敏感路径门禁（AI 标记 + 敏感路径必拦 / 无 AI 标记放行）
//   - TestCheckCoverage：覆盖率门禁（40% 必拦 / 60% 放行，阈值同源 codecov.yml）
//   - TestToneWordsGate：tone 禁用词门禁（字符串文案含「一定构成侵权」必拦 / 正常放行）
//   - TestPrecommitLintFailClosed：lint 工具缺失时阻断提交（fail-closed，单向）
//
// 红绿测试在 t.TempDir() 隔离的临时 git 仓库内执行（只 git add 不 commit），
// 不触碰真实仓库状态。位于根模块 `go test ./...` 覆盖内，make verify 与 CI 自动执行。
package gatechecks

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// repoRoot 推导：gate_test.go → scripts/ → 仓库根。
var repoRoot = filepath.Dir(filepath.Dir(mustAbs(".")))

func mustAbs(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		panic(err)
	}
	return abs
}

func scriptPath(name string) string {
	return filepath.Join(repoRoot, "scripts", name)
}

// runCmd 在 dir 下运行命令，返回退出码与合并输出。
// 环境变量 env 追加在 os.Environ() 之后（后者可覆盖同名字段）。
func runCmd(t *testing.T, dir string, env []string, name string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode(), string(out)
	}
	t.Fatalf("运行 %s %v 失败: %v（输出: %s）", name, args, err, out)
	return -1, ""
}

// gitInit 初始化临时 git 仓库并配置提交身份（只 add 不 commit，无需身份，配置以防将来扩展）。
func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "gatecheck@example.com"},
		{"config", "user.name", "gatecheck"},
	} {
		if code, out := runCmd(t, dir, nil, "git", args...); code != 0 {
			t.Fatalf("git %v 失败(%d): %s", args, code, out)
		}
	}
}

// --- AI 变更日志格式门禁 -------------------------------------------------------

func TestAichangelogFormatGate_Red_MissingBackground(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	changelogDir := filepath.Join(dir, "docs", "decisions", "ai-changelog")
	if err := os.MkdirAll(changelogDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 缺 **背景** 字段（只有改动清单）——必须被拦截
	entry := "## docs(server): 缺背景条目\n**改动清单**：xxx\n"
	if err := os.WriteFile(filepath.Join(changelogDir, "2026-08-23.md"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := runCmd(t, dir, nil, "git", "add", "."); code != 0 {
		t.Fatalf("git add 失败(%d): %s", code, out)
	}

	code, out := runCmd(t, dir, nil, "bash", scriptPath("check-aichangelog-format.sh"))
	if code != 1 {
		t.Errorf("缺**背景**的条目应被拦截（exit 1），实际 exit=%d 输出:\n%s", code, out)
	}
	if !strings.Contains(out, "背景") {
		t.Errorf("错误输出应指出缺少**背景**字段:\n%s", out)
	}
}

func TestAichangelogFormatGate_Green_FourFields(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	changelogDir := filepath.Join(dir, "docs", "decisions", "ai-changelog")
	if err := os.MkdirAll(changelogDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 四字段齐全（背景/改动清单必填 + 验证/影响建议）——应放行
	entry := "## docs(server): 四字段齐全条目\n" +
		"**背景**：为什么做（必填）\n" +
		"**改动清单**：改了什么（必填）\n" +
		"**验证**：如何验证（建议）\n" +
		"**影响**：换来了什么（建议）\n"
	if err := os.WriteFile(filepath.Join(changelogDir, "2026-08-23.md"), []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := runCmd(t, dir, nil, "git", "add", "."); code != 0 {
		t.Fatalf("git add 失败(%d): %s", code, out)
	}

	code, out := runCmd(t, dir, nil, "bash", scriptPath("check-aichangelog-format.sh"))
	if code != 0 {
		t.Errorf("四字段齐全的条目应放行（exit 0），实际 exit=%d 输出:\n%s", code, out)
	}
}

// --- 敏感路径门禁 --------------------------------------------------------------

func TestSensitivePathsGate_Red_AIPlusSensitivePath(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	// 暂存一个敏感路径文件（红线文件仅作为 fixture 被创建，不改动真实仓库）
	if err := os.MkdirAll(filepath.Join(dir, "agentcore"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agentcore", "handoff.go"), []byte("package agentcore\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := runCmd(t, dir, nil, "git", "add", "."); code != 0 {
		t.Fatalf("git add 失败(%d): %s", code, out)
	}

	// 提交消息含 AI 协助标记（Co-authored-by）——AI + 敏感路径 = 必须拦截
	msgFile := filepath.Join(dir, "msg.txt")
	if err := os.WriteFile(msgFile, []byte("feat: xxx\n\nCo-authored-by: Claude <noreply@anthropic.com>\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out := runCmd(t, dir, nil, "bash", scriptPath("check-sensitive-paths.sh"), "--msg-file", msgFile)
	if code != 1 {
		t.Errorf("AI 标记 + 敏感路径应被拦截（exit 1），实际 exit=%d 输出:\n%s", code, out)
	}
	if !strings.Contains(out, "敏感路径") {
		t.Errorf("错误输出应指出敏感路径:\n%s", out)
	}
}

func TestSensitivePathsGate_Green_NoAIMarker(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	if err := os.MkdirAll(filepath.Join(dir, "agentcore"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agentcore", "handoff.go"), []byte("package agentcore\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := runCmd(t, dir, nil, "git", "add", "."); code != 0 {
		t.Fatalf("git add 失败(%d): %s", code, out)
	}

	// 无 AI 标记——敏感路径变更仅提示，不拦截
	msgFile := filepath.Join(dir, "msg.txt")
	if err := os.WriteFile(msgFile, []byte("feat: xxx\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out := runCmd(t, dir, nil, "bash", scriptPath("check-sensitive-paths.sh"), "--msg-file", msgFile)
	if code != 0 {
		t.Errorf("无 AI 标记应放行（exit 0），实际 exit=%d 输出:\n%s", code, out)
	}
}

// --- 覆盖率门禁 -----------------------------------------------------------------

// writeCoverProfile 合成 coverprofile：total 语句数中 covered 条被覆盖。
// 路径避开 codecov.yml 的 ignore 段（example/**、**/*_test.go 等）。
func writeCoverProfile(t *testing.T, total, covered int) string {
	t.Helper()
	if covered > total {
		t.Fatalf("covered %d > total %d", covered, total)
	}
	var sb strings.Builder
	sb.WriteString("mode: set\n")
	if covered > 0 {
		sb.WriteString("pkg/a.go:1.1,2.2 " + strconv.Itoa(covered) + " 1\n")
	}
	if total-covered > 0 {
		sb.WriteString("pkg/b.go:1.1,2.2 " + strconv.Itoa(total-covered) + " 0\n")
	}
	path := filepath.Join(t.TempDir(), "coverage.out")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckCoverage_Red_BelowThreshold(t *testing.T) {
	// 40% < codecov.yml floor（target 55 − threshold 2 = 53%）——必须 exit 1
	profile := writeCoverProfile(t, 10, 4)
	code, out := runCmd(t, repoRoot, nil, "python3", scriptPath("check-coverage.py"), profile)
	if code != 1 {
		t.Errorf("40%% 覆盖率应被拦截（exit 1），实际 exit=%d 输出:\n%s", code, out)
	}
}

func TestCheckCoverage_Green_AboveThreshold(t *testing.T) {
	// 60% > 53%——应放行
	profile := writeCoverProfile(t, 10, 6)
	code, out := runCmd(t, repoRoot, nil, "python3", scriptPath("check-coverage.py"), profile)
	if code != 0 {
		t.Errorf("60%% 覆盖率应放行（exit 0），实际 exit=%d 输出:\n%s", code, out)
	}
}

// --- tone 禁用词门禁 ------------------------------------------------------------

func TestToneWordsGate_Red_ForbiddenPhrase(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	// 面向用户文案目录（server/）中的字符串文案含禁用词——必须被拦截。
	// 违规词须在字符串字面量而非注释里：脚本跳过代码注释（词表约束的是产出文案）。
	if err := os.MkdirAll(filepath.Join(dir, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package server\n\nconst red = \"该产品一定构成侵权，无需人工复核。\"\n"
	if err := os.WriteFile(filepath.Join(dir, "server", "tone_red.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := runCmd(t, dir, nil, "git", "add", "."); code != 0 {
		t.Fatalf("git add 失败(%d): %s", code, out)
	}

	code, out := runCmd(t, dir, nil, "python3", scriptPath("check-tone-words.py"), "--fail")
	if code != 1 {
		t.Errorf("含禁用词的文案应被拦截（exit 1），实际 exit=%d 输出:\n%s", code, out)
	}
	if !strings.Contains(out, "构成侵权") {
		t.Errorf("错误输出应指出命中的禁用词:\n%s", out)
	}
}

func TestToneWordsGate_Green_NormalPhrase(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	if err := os.MkdirAll(filepath.Join(dir, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package server\n\n// 面向用户文案：本方案与对比文件存在多处相似特征，需人工复核确认。\n"
	if err := os.WriteFile(filepath.Join(dir, "server", "tone_ok.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := runCmd(t, dir, nil, "git", "add", "."); code != 0 {
		t.Fatalf("git add 失败(%d): %s", code, out)
	}

	code, out := runCmd(t, dir, nil, "python3", scriptPath("check-tone-words.py"), "--fail")
	if code != 0 {
		t.Errorf("正常文案应放行（exit 0），实际 exit=%d 输出:\n%s", code, out)
	}
}

// --- lint fail-closed（单向：工具缺失必须阻断） ----------------------------------

func TestPrecommitLintFailClosed_MissingLintBinary(t *testing.T) {
	// 构造 PATH 只含 go/git 软链（无 golangci-lint），GOPATH 指向不存在目录——
	// 脚本的 GOPATH/bin 探测与 PATH 回退都失败时，必须 exit 1（fail-closed）。
	binDir := t.TempDir()
	for _, tool := range []string{"go", "git"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("环境缺少 %s，跳过: %v", tool, err)
		}
		if err := os.Symlink(path, filepath.Join(binDir, tool)); err != nil {
			t.Fatal(err)
		}
	}

	env := []string{
		"PATH=" + binDir,
		"GOPATH=" + filepath.Join(binDir, "nonexistent-gopath"),
	}
	code, out := runCmd(t, repoRoot, env, "bash", scriptPath("precommit-golangci-lint.sh"))
	if code != 1 {
		t.Errorf("golangci-lint 缺失应阻断提交（exit 1），实际 exit=%d 输出:\n%s", code, out)
	}
	if !strings.Contains(out, "未安装") {
		t.Errorf("输出应提示未安装与安装方式:\n%s", out)
	}
}
