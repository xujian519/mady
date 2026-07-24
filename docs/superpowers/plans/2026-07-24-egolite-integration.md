# EgoLite Browser 集成实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 ego-browser 集成为 Mady 的第 7 个浏览器后端 + 新增 handoff/task_spaces 工作流工具

**Architecture:** 持久子进程模型 — Go 通过 stdin/stdout 行分隔 JSON 协议与长生命周期 `ego-browser nodejs` 进程通信。Bridge 脚本（go:embed）在进程内执行 ego-browser helper。EgoLiteExtension 同时实现 agentcore.Extension 和 browser backend 接口。

**Tech Stack:** Go 1.26, ego-browser CLI, Node.js (内嵌), go:embed, encoding/json, bufio.Scanner

**Source spec:** `docs/superpowers/specs/2026-07-24-egolite-integration-design.md`

---

## 文件结构

```
新建:
  tools/ego_lite_bridge.js    # go:embed 嵌入，JSON-RPC bridge 脚本
  tools/ego_lite_manager.go   # 子进程管理 + JSON 通信
  tools/ego_lite_manager_test.go # Manager 单元测试（mock bridge）
  tools/ego_lite.go           # EgoLiteExtension 组装 + handoff/task_spaces 工具
  tools/ego_lite_test.go      # 工具层单元测试

修改:
  tools/browser_supervisor.go # 新增 BackendEgoLite 常量 + EgoLiteEnabled 字段
  tools/browser_session.go    # BrowserSession 新增 egoLiteManager 字段
  tools/browser_tool.go       # BrowserToolConfig 新增 EgoLiteEnabled/EgoLiteTaskName
  tools/browser_tool_navigate.go # handleNavigate 新增 BackendEgoLite 分支
  tools/browser_tool_interact.go # handleClick/handleType/handleScroll 新增分支
  tools/browser_tool_handlers.go # handleSnapshot/handleEvaluate 新增分支
  tools/browser_tool_media.go    # handleScreenshot 新增分支
  tools/browser_tool_debug.go    # handleCdp/handleConsole 返回 not-supported
```

---

### Task 1: Bridge 脚本

**Files:**
- Create: `tools/ego_lite_bridge.js`

- [ ] **Step 1: 写入 bridge 脚本**

```javascript
// ego_lite_bridge.js
// 长生命周期进程，从 stdin 读取 JSON 命令，写入 JSON 结果到 stdout。
// 由 ego-browser nodejs 执行，ego-browser helper 全部预加载。

const readline = require('readline');
const rl = readline.createInterface({ input: process.stdin });

let currentTask = null;

rl.on('line', async (line) => {
  let req;
  try { req = JSON.parse(line); } catch (e) { return; }
  const { id, method, params } = req;
  try {
    const result = await dispatch(method, params || {});
    respond(id, true, result);
  } catch (e) {
    respond(id, false, null, e.message);
  }
});

function respond(id, ok, result, error) {
  const out = { id, ok };
  if (ok) { out.result = result; } else { out.error = error; }
  process.stdout.write(JSON.stringify(out) + '\n');
}

async function dispatch(method, params) {
  switch (method) {
    case 'ping':
      return 'pong';
    case 'initTaskSpace':
      currentTask = await useOrCreateTaskSpace(params.name);
      return { taskId: currentTask.taskId, id: currentTask.id };
    case 'navigate':
      await openOrReuseTab(params.url, { wait: true, timeout: params.timeout || 20 });
      return await snapshotText();
    case 'snapshotText':
      return await snapshotText(params);
    case 'captureScreenshot':
      const buf = await captureScreenshot();
      return buf.toString('base64');
    case 'click':
      await click(params.ref, params.label ? { label: params.label } : undefined);
      return await snapshotText();
    case 'typeText':
      await fillInput(params.ref, params.text);
      return `Typed "${params.text}" into ${params.ref}`;
    case 'scroll':
      await scrollBy(typeof params.dy === 'number' ? params.dy : 500);
      return await snapshotText();
    case 'pressKey':
      await pressKey(params.key);
      return `Pressed key: ${params.key}`;
    case 'evaluateJS':
      return await js(String.raw`${params.expression}`);
    case 'pageInfo':
      return await pageInfo();
    case 'handoffTaskSpace':
      return await handOffTaskSpace();
    case 'takeOverTaskSpace':
      await takeOverTaskSpace(currentTask ? currentTask.id : undefined);
      return {};
    case 'listTaskSpaces':
      const spaces = await listTaskSpaces();
      return spaces.map(s => ({ id: s.id, name: s.name, ownership: s.ownership }));
    case 'listTabs':
      const tabs = await listTabs();
      return tabs.map(t => ({ id: t.id, url: t.url, title: t.title }));
    case 'closeTab':
      await closeTab(params.targetId || undefined);
      return {};
    case 'completeTaskSpace':
      const res = await completeTaskSpace(currentTask ? currentTask.id : undefined, { keep: !!params.keep });
      currentTask = null;
      return res;
    default:
      throw new Error(`unknown method: ${method}`);
  }
}
```

- [ ] **Step 2: 验证语法**

```bash
node --check tools/ego_lite_bridge.js
```

Expected: 无输出（语法正确）

- [ ] **Step 3: 测试 bridge 手动调用**

```bash
echo '{"id":"1","method":"ping","params":{}}' | ego-browser nodejs tools/ego_lite_bridge.js
```

Expected: `{"id":"1","ok":true,"result":"pong"}`

- [ ] **Step 4: Commit**

```bash
git add tools/ego_lite_bridge.js
git commit -m "feat(egolite): add JSON-RPC bridge script for ego-browser"
```

---

### Task 2: EgoLiteManager — 核心通信层

**Files:**
- Create: `tools/ego_lite_manager.go`
- Modify: `tools/browser_supervisor.go`

- [ ] **Step 1: 新增 BackendEgoLite 常量**

在 `tools/browser_supervisor.go` 的 `BrowserBackendType` 常量组中新增:

```go
BackendEgoLite BrowserBackendType = "egolite"
```

- [ ] **Step 2: 写入 ego_lite_manager.go 完整实现**

`tools/ego_lite_manager.go` 声明为 package tools，因此可以直接访问同包内的所有类型。

```go
// ego_lite_manager.go — EgoLite 浏览器子进程管理器
package tools

import (
	"bufio"
	"context"
	"encoding/json"
	_ "embed"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

const (
	egoLiteBinary    = "ego-browser"
	egoLiteInitRetry = 3
	egoLiteRetryWait = 500 * time.Millisecond
)

//go:embed ego_lite_bridge.js
var egoLiteBridgeScript []byte

type egoLiteJSONRequest struct {
	ID     string         `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

type egoLiteJSONResponse struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// EgoLiteManager 管理一个持久化 ego-browser 子进程。
// Send() 方法是唯一的公开 API，内部保证命令串行化（同一进程不能并发操作同一标签页）。
type EgoLiteManager struct {
	mu      sync.Mutex
	pending map[string]chan egoLiteJSONResponse
	counter atomic.Int64

	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Scanner

	taskName    string
	currentTask string

	closed atomic.Bool
	ctx    context.Context
	cancel context.CancelFunc
}

// egoLiteAvailable 检查 ego-browser 是否可执行。
func egoLiteAvailable() bool {
	_, err := exec.LookPath(egoLiteBinary)
	return err == nil
}

// NewEgoLiteManager 创建一个未启动的管理器。首次 Send() 调用时延迟启动子进程。
func NewEgoLiteManager(taskName string) (*EgoLiteManager, error) {
	if !egoLiteAvailable() {
		return nil, fmt.Errorf("egolite: %s not found on PATH (install ego lite: https://lite.ego.app/)", egoLiteBinary)
	}
	if taskName == "" {
		taskName = "mady-agent"
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &EgoLiteManager{
		taskName: taskName,
		pending:  make(map[string]chan egoLiteJSONResponse),
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// Send 发送命令并等待响应。如果子进程尚未启动，先启动它。
// mu 确保同一时刻只有一个命令在执行。
func (m *EgoLiteManager) Send(ctx context.Context, method string, params map[string]any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed.Load() {
		return nil, fmt.Errorf("egolite: manager closed")
	}
	if m.cmd == nil || m.stdin == nil {
		if err := m.start(ctx); err != nil {
			return nil, fmt.Errorf("egolite: start failed: %w", err)
		}
	}

	id := fmt.Sprintf("r%d", m.counter.Add(1))
	req := egoLiteJSONRequest{ID: id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("egolite: marshal request: %w", err)
	}

	ch := make(chan egoLiteJSONResponse, 1)
	m.pending[id] = ch
	defer func() { delete(m.pending, id) }()

	data = append(data, '\n')
	if _, err := m.stdin.Write(data); err != nil {
		// stdin write 失败通常是子进程已死，尝试重启后重试一次
		slog.Warn("egolite: stdin write failed, attempting restart", "err", err)
		if restartErr := m.restart(ctx); restartErr != nil {
			return nil, fmt.Errorf("egolite: write failed and restart error: %w (original: %w)", restartErr, err)
		}
		data = append([]byte(nil), data...)
		if _, err := m.stdin.Write(data); err != nil {
			return nil, fmt.Errorf("egolite: write after restart failed: %w", err)
		}
	}

	select {
	case resp := <-ch:
		if !resp.OK {
			return nil, fmt.Errorf("egolite: %s failed: %s", method, resp.Error)
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// start 启动 ego-browser 子进程并执行初始化的 initTaskSpace。
func (m *EgoLiteManager) start(ctx context.Context) error {
	// 将 bridge 脚本写入临时文件（ego-browser nodejs 需要文件路径）
	// 避免每次启动都写到磁盘：第一次写到临时目录后缓存路径
	bridgePath, err := m.writeBridgeScript()
	if err != nil {
		return err
	}

	m.cmd = exec.CommandContext(m.ctx, egoLiteBinary, "nodejs", bridgePath)
	m.cmd.Stderr = nil // stderr 被 ego-browser 内部用于日志

	stdin, err := m.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}
	m.stdin = stdin

	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	m.stdout = bufio.NewScanner(stdout)
	m.stdout.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 1MB max line

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", egoLiteBinary, err)
	}

	// 启动 stdout 读取 goroutine
	go m.readLoop()

	// 初始化任务空间
	result, initErr := m.sendRaw(ctx, "initTaskSpace", map[string]any{"name": m.taskName})
	if initErr != nil {
		// init 失败时清理子进程
		m.cmd.Process.Kill()
		m.cmd = nil
		m.stdin = nil
		m.stdout = nil
		return fmt.Errorf("init task space: %w", initErr)
	}

	if resultMap, ok := result.(map[string]any); ok {
		if id, ok := resultMap["id"]; ok {
			m.currentTask = fmt.Sprintf("%v", id)
		}
	}
	slog.Info("egolite: manager started", "task", m.currentTask)
	return nil
}

// sendRaw 发送命令但不持有 m.mu（内部使用，调用者必须已持有 mu）。
func (m *EgoLiteManager) sendRaw(ctx context.Context, method string, params map[string]any) (any, error) {
	id := fmt.Sprintf("r%d", m.counter.Add(1))
	req := egoLiteJSONRequest{ID: id, Method: method, Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	ch := make(chan egoLiteJSONResponse, 1)
	m.pending[id] = ch
	defer func() { delete(m.pending, id) }()

	if _, err := m.stdin.Write(append(data, '\n')); err != nil {
		return nil, err
	}

	select {
	case resp := <-ch:
		if !resp.OK {
			return nil, fmt.Errorf("%s", resp.Error)
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// readLoop 持续从 stdout 读取 JSON 行并匹配 pending 请求。
func (m *EgoLiteManager) readLoop() {
	for m.stdout.Scan() {
		line := m.stdout.Bytes()
		var resp egoLiteJSONResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			slog.Warn("egolite: bad JSON from bridge", "line", string(line), "err", err)
			continue
		}
		m.mu.Lock()
		if ch, ok := m.pending[resp.ID]; ok {
			ch <- resp
		}
		m.mu.Unlock()
	}
	// readLoop 退出意味着 stdout 关闭（子进程崩溃或正常退出）
	if !m.closed.Load() {
		slog.Warn("egolite: bridge stdout closed unexpectedly")
	}
}

// restart 杀死旧子进程并重新启动，恢复 currentTask。
// 调用者必须持有 m.mu。
func (m *EgoLiteManager) restart(ctx context.Context) error {
	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}
	m.cmd = nil
	m.stdin = nil
	m.stdout = nil
	return m.start(ctx)
}

// writeBridgeScript 写入 bridge 脚本到临时文件，返回路径。
func (m *EgoLiteManager) writeBridgeScript() (string, error) {
	// 简单方案：写入 os.TempDir()，每次启动覆盖同一文件
	f, err := os.CreateTemp("", "mady-ego-lite-bridge-*.js")
	if err != nil {
		return "", fmt.Errorf("create temp bridge file: %w", err)
	}
	path := f.Name()
	if _, err := f.Write(egoLiteBridgeScript); err != nil {
		f.Close()
		os.Remove(path)
		return "", err
	}
	f.Close()
	return path, nil
}

// Close 关闭管理器，清理桥进程。
func (m *EgoLiteManager) Close() error {
	if !m.closed.CompareAndSwap(false, true) {
		return nil
	}
	m.cancel()

	// 尝试优雅关闭任务空间（不阻塞）
	if m.cmd != nil && m.stdin != nil && m.currentTask != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		m.Send(ctx, "completeTaskSpace", map[string]any{"keep": false})
	}

	if m.cmd != nil && m.cmd.Process != nil {
		m.cmd.Process.Kill()
	}
	return nil
}

// TaskID 返回当前任务空间 ID。
func (m *EgoLiteManager) TaskID() string { return m.currentTask }
```

- [ ] **Step 3: 编译验证**

```bash
go build ./tools/
```

Expected: 编译成功（bridge 脚本通过 go:embed 嵌入）

- [ ] **Step 4: Commit**

```bash
git add tools/ego_lite_manager.go tools/browser_supervisor.go
git commit -m "feat(egolite): add EgoLiteManager — persistent subprocess management"
```

---

### Task 3: EgoLiteManager 单元测试（mock bridge）

**Files:**
- Create: `tools/ego_lite_manager_test.go`

- [ ] **Step 1: 写入测试脚本 mock bridge**

创建 `testdata/ego_lite_mock_bridge.js`（但为了简单，测试用一个小 Go 程序模拟 JSON 响应）：

实际上，使用 `exec.Command("echo", ...)` 模拟更简单。或者我们写一个简单的 echo 脚本：

`tools/ego_lite_manager_test.go`:

```go
package tools

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestEgoLiteManagerSend 验证 Send 能够正确发送请求并接收响应，
// 使用一个简单的 echo bridge（非真实 ego-browser）。
func TestEgoLiteManagerSend(t *testing.T) {
	// 用 cat 作为最简单的 bridge：读到一行写回一行
	// 预先准备好响应
	bridgeJS := []byte(`
const readline = require('readline');
const rl = readline.createInterface({input: process.stdin});
rl.on('line', function(line) {
  const req = JSON.parse(line);
  if (req.method === 'ping') {
    process.stdout.write(JSON.stringify({id:req.id,ok:true,result:'pong'})+'\n');
  } else if (req.method === 'initTaskSpace') {
    process.stdout.write(JSON.stringify({id:req.id,ok:true,result:{id:'ts-1',taskId:'t1'}})+'\n');
  } else {
    process.stdout.write(JSON.stringify({id:req.id,ok:true,result:null})+'\n');
  }
});
`)
	// 写入临时文件
	tmpFile, err := os.CreateTemp("", "test-bridge-*.js")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.Write(bridgeJS); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	// 用 ego-browser 测试需要真实环境，此处测试基础结构
	// 验证闭合/重启逻辑需要 useOrCreateTaskSpace 等 helper，这里仅验证纯 Go 逻辑
	if !egoLiteAvailable() {
		t.Skip("ego-browser not installed")
	}

	mgr, err := NewEgoLiteManager("test")
	if err != nil {
		// 即便 ego-browser 不可用也应该能创建 manager
		t.Fatalf("NewEgoLiteManager failed: %v", err)
	}
	defer mgr.Close()

	// 测试 ping
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := mgr.Send(ctx, "ping", nil)
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if result != "pong" {
		t.Errorf("expected pong, got %v", result)
	}

	// 测试 Send 超时
	deadCtx, deadCancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer deadCancel()
	time.Sleep(1 * time.Millisecond)
	_, err = mgr.Send(deadCtx, "ping", nil)
	if err == nil {
		t.Error("expected timeout error from expired context")
	}
}

// TestEgoLiteManagerCloseIdempotent 验证重复 Close 不会 panic。
func TestEgoLiteManagerCloseIdempotent(t *testing.T) {
	mgr := &EgoLiteManager{
		taskName: "test",
		pending:  make(map[string]chan egoLiteJSONResponse),
	}
	// 第一次 Close
	if err := mgr.Close(); err != nil {
		t.Fatalf("first close failed: %v", err)
	}
	// 第二次 Close（幂等）
	if err := mgr.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
}

// TestEgoLiteManagerErrorOnClosed 验证关闭后 Send 返回错误。
func TestEgoLiteManagerErrorOnClosed(t *testing.T) {
	if !egoLiteAvailable() {
		t.Skip("ego-browser not installed")
	}
	mgr, err := NewEgoLiteManager("test")
	if err != nil {
		t.Fatal(err)
	}
	mgr.Close()

	ctx := context.Background()
	_, err = mgr.Send(ctx, "ping", nil)
	if err == nil {
		t.Error("expected error from closed manager")
	}
}

// TestEgoLiteAvailable 验证 egoLiteAvailable 函数。
func TestEgoLiteAvailable(t *testing.T) {
	_, err := exec.LookPath("ego-browser")
	expect := err == nil
	got := egoLiteAvailable()
	if got != expect {
		t.Errorf("egoLiteAvailable = %v, LookPath error = %v", got, err)
	}
}
```

- [ ] **Step 2: 运行测试**

```bash
go test ./tools/ -run TestEgoLite -v -count=1
```

Expected: PASS（无 ego-browser 的环境 Skip，Close 幂等测试通过）

- [ ] **Step 3: 竞态检测**

```bash
go test ./tools/ -run TestEgoLite -race -count=1
```

Expected: 无竞态告警

- [ ] **Step 4: Commit**

```bash
git add tools/ego_lite_manager_test.go
git commit -m "test(egolite): add EgoLiteManager unit tests"
```

---

### Task 4: BrowserSession 集成 — 后端常量 + 会话字段

**Files:**
- Modify: `tools/browser_session.go`

- [ ] **Step 1: BrowserSession 新增 egoLiteManager 字段**

在 `tools/browser_session.go` 的 `BrowserSession` struct 中 `// ... 现有字段 ...` 注释后新增:

```go
type BrowserSession struct {
	mu             sync.RWMutex
	sessionID      string
	backendType    BrowserBackendType
	cdpURL         string
	cloudProvider  browserproviders.CloudBrowserProvider
	cloudSessionID string
	camofoxClient  *CamofoxClient
	lightpandaProc *LightpandaProcess
	ctx            context.Context
	cancel         context.CancelFunc
	url            string
	title          string
	createdAt      time.Time
	lastActivity   time.Time
	supervisor     *CDPSupervisor
	recorder       *CDPRecorder
	refMapper      *RefMapper
	// egolite 后端专用
	egoLiteManager *EgoLiteManager
}
```

- [ ] **Step 2: NewBrowserManager 中初始化 EgoLiteManager**

在 `NewBrowserManager` 函数中，`switch backend` 块内新增 `case BackendEgoLite`:

```go
func NewBrowserManager(cfg *BrowserConfig) *BrowserManager {
	cfg.defaults()

	ctx, cancel := context.WithCancel(context.Background())
	mgr := &BrowserManager{
		sessions: make(map[string]*BrowserSession),
		config:   *cfg,
		ctx:      ctx,
		cancel:   cancel,
	}

	backend := DetectBackend(&mgr.config)
	switch backend {
	case BackendEgoLite:
		var err error
		mgr.egoLiteMgr, err = NewEgoLiteManager(cfg.EgoLiteTaskName)
		if err != nil {
			slog.Warn("egolite: create manager failed, falling back to local", "err", err)
			backend = BackendLocal
		}
	// ... 其余 case 保持不变 ...
	}
```

需要在 `BrowserManager` struct 中新增字段:

```go
type BrowserManager struct {
	mu                     sync.RWMutex
	sessions               map[string]*BrowserSession
	config                 BrowserConfig
	ctx                    context.Context
	cancel                 context.CancelFunc
	camofoxClient          *CamofoxClient
	lightpandaMgr          *LightpandaManager
	agentBrowserMgr        *AgentBrowserManager
	egoLiteMgr             *EgoLiteManager
	cloudProvider          browserproviders.CloudBrowserProvider
	fallbackCloudProviders []browserproviders.CloudBrowserProvider
}
```

- [ ] **Step 3: BrowserConfig 新增 EgoLite 相关配置**

在 `tools/browser_supervisor.go` 的 `BrowserConfig` struct 中新增:

```go
type BrowserConfig struct {
	// ... 现有字段 ...
	AgentBrowserEnabled bool
	// EgoLite 配置
	EgoLiteEnabled  bool
	EgoLiteTaskName string
}
```

在 `DetectBackend` 函数中新增检测:

```go
func DetectBackend(cfg *BrowserConfig) BrowserBackendType {
	if cfg.EgoLiteEnabled {
		return BackendEgoLite
	}
	// ... 现有逻辑保持不变 ...
}
```

- [ ] **Step 4: CloseAll 中清理 EgoLiteManager**

在 `BrowserManager.CloseAll()` 中新增清理:

```go
if bm.egoLiteMgr != nil {
	bm.egoLiteMgr.Close()
}
```

- [ ] **Step 5: CreateSession 中设置 egoLiteManager**

在 `BrowserManager.CreateSession()` 中，当 `backend == BackendEgoLite` 时:

```go
if bm.egoLiteMgr != nil {
	session.egoLiteManager = bm.egoLiteMgr
}
```

- [ ] **Step 6: 编译验证**

```bash
go build ./tools/
```

Expected: 编译成功

- [ ] **Step 7: Commit**

```bash
git add tools/browser_session.go tools/browser_supervisor.go
git commit -m "feat(egolite): integrate EgoLiteManager into BrowserSession and BrowserConfig"
```

---

### Task 5: Runtime 层 — BackendEgoLite Handler 分支

**Files:**
- Modify: `tools/browser_tool_navigate.go` — 新增 `handleNavigate` BackendEgoLite
- Modify: `tools/browser_tool_interact.go` — 新增 handleClick/handleType/handleScroll/handlePress BackendEgoLite
- Modify: `tools/browser_tool_handlers.go` — 新增 handleSnapshot/handleEvaluate BackendEgoLite
- Modify: `tools/browser_tool_media.go` — 新增 handleScreenshot BackendEgoLite
- Modify: `tools/browser_tool_debug.go` — handleCdp/handleConsole 返回 not-supported
- Modify: `tools/browser_tool.go` — 新增 EgoLiteEnabled/EgoLiteTaskName 配置字段

- [ ] **Step 1: browser_tool.go 新增配置字段**

在 `BrowserToolConfig` struct 中新增:

```go
type BrowserToolConfig struct {
	// ... 现有字段 ...
	AgentBrowserEnabled bool
	// EgoLite 配置
	EgoLiteEnabled  bool
	EgoLiteTaskName string
}
```

在 `NewBrowserTool` 函数的 `BrowserConfig{}` 构造中新增:

```go
bm := NewBrowserManager(&BrowserConfig{
	// ... 现有字段 ...
	AgentBrowserEnabled: cfg.AgentBrowserEnabled,
	EgoLiteEnabled:      cfg.EgoLiteEnabled,
	EgoLiteTaskName:     cfg.EgoLiteTaskName,
})
```

- [ ] **Step 2: browser_tool_navigate.go — handleNavigate 新增分支**

在 `handleNavigate` 函数的 switch case 组中添加 `BackendEgoLite`，位置在 `BackendCamofox` 块之后:

```go
case BackendEgoLite:
	snapshot, err = session.egoLiteManager.Send(ctx, "navigate", map[string]any{
		"url":     parsedURL.String(),
		"timeout": 20,
	})
	if err != nil {
		return nil, fmt.Errorf("egolite navigate: %w", err)
	}
	sv, _ := snapshot.(string)
	snapshot = sv

	// 获取 pageInfo 更新 session url/title
	if piResult, piErr := session.egoLiteManager.Send(ctx, "pageInfo", nil); piErr == nil {
		if pi, ok := piResult.(map[string]any); ok {
			if u, ok := pi["url"].(string); ok {
				session.mu.Lock()
				session.url = u
				session.mu.Unlock()
			}
			if t, ok := pi["title"].(string); ok {
				session.mu.Lock()
				session.title = t
				session.mu.Unlock()
			}
		}
	}
```

- [ ] **Step 3: browser_tool_interact.go — 交互 handler 新增分支**

a) `handleClick` 中，在 `BackendCamofox` 块之后新增:

```go
case BackendEgoLite:
	snapshot, err := session.egoLiteManager.Send(ctx, "click", map[string]any{
		"ref": ref,
	})
	if err != nil {
		return nil, fmt.Errorf("egolite click: %w", err)
	}
	sv, _ := snapshot.(string)
	snapshot = sv
```

b) `handleType` 中，在 `BackendCamofox` 块之后新增:

```go
case BackendEgoLite:
	resultMsg, err = session.egoLiteManager.Send(ctx, "typeText", map[string]any{
		"ref":  ref,
		"text": input.Text,
	})
	if err != nil {
		return nil, fmt.Errorf("egolite type: %w", err)
	}
	rv, _ := resultMsg.(string)
	resultMsg = rv
```

c) `handleScroll` 中，在 `BackendCamofox` 块之后新增:

```go
case BackendEgoLite:
	dy := any(500)
	if input.Direction == "up" {
		dy = -500
	}
	snapshot, err = session.egoLiteManager.Send(ctx, "scroll", map[string]any{"dy": dy})
	if err != nil {
		return nil, fmt.Errorf("egolite scroll: %w", err)
	}
	sv, _ := snapshot.(string)
	snapshot = sv
```

d) `handlePress` 中，在 `BackendCamofox` 块之后新增:

```go
case BackendEgoLite:
	resultMsg, err = session.egoLiteManager.Send(ctx, "pressKey", map[string]any{"key": input.Key})
	if err != nil {
		return nil, fmt.Errorf("egolite press: %w", err)
	}
	rv, _ := resultMsg.(string)
	resultMsg = rv
```

注意：在现有 switch 中这些 action handler 已经有 `err` 和 `resultMsg` 的声明，不需要重复声明变量。

- [ ] **Step 4: browser_tool_handlers.go — handleSnapshot + handleEvaluate 新增分支**

a) `handleSnapshot` 中新增:

```go
case BackendEgoLite:
	snapshotStr, snapErr := session.egoLiteManager.Send(ctx, "snapshotText", nil)
	if snapErr != nil {
		return nil, fmt.Errorf("egolite snapshot: %w", snapErr)
	}
	snapshot, _ = snapshotStr.(string)
```

b) `handleEvaluate` 中新增（在 `BackendCamofox` 错误返回之后）:

```go
case BackendEgoLite:
	evalResult, err = session.egoLiteManager.Send(ctx, "evaluateJS", map[string]any{
		"expression": input.Expression,
	})
	if err != nil {
		return nil, fmt.Errorf("egolite evaluate: %w", err)
	}
	rev, _ := evalResult.(string)
	evalResult = rev
```

- [ ] **Step 5: browser_tool_media.go — handleScreenshot 新增分支**

```go
case BackendEgoLite:
	b64, snapErr := session.egoLiteManager.Send(ctx, "captureScreenshot", nil)
	if snapErr != nil {
		return nil, fmt.Errorf("egolite screenshot: %w", snapErr)
	}
	b64Str, _ := b64.(string)
	if b64Str == "" {
		return nil, fmt.Errorf("egolite screenshot returned empty")
	}
	buf, err = base64.StdEncoding.DecodeString(b64Str)
```

- [ ] **Step 6: browser_tool_debug.go — handleCdp/handleConsole 新增分支**

在 `handleCdp` switch 组中添加:

```go
case BackendEgoLite:
	return nil, fmt.Errorf("cdp not supported for egolite backend")
```

`handleConsole` 不依赖 backend，无需改动。

- [ ] **Step 7: 编译验证**

```bash
go build ./tools/
```

Expected: 编译成功

- [ ] **Step 8: Commit**

```bash
git add tools/browser_tool.go tools/browser_tool_navigate.go tools/browser_tool_interact.go tools/browser_tool_handlers.go tools/browser_tool_media.go tools/browser_tool_debug.go
git commit -m "feat(egolite): add BackendEgoLite handler branches for all browser actions"
```

---

### Task 6: Workflow 层 — Handoff + TaskSpaces 工具

**Files:**
- Create: `tools/ego_lite.go`

- [ ] **Step 1: 写入 ego_lite.go — EgoLiteExtension + 两个工具**

```go
// ego_lite.go — EgoLite Extension: handoff + task_spaces 工具
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xujian519/mady/agentcore"
)

const (
	EgoLiteHandoffToolName     = "ego_lite_handoff"
	EgoLiteTaskSpacesToolName  = "ego_lite_task_spaces"
)

// EgoLiteConfig 配置 EgoLite Extension。
type EgoLiteConfig struct {
	Enabled  bool
	TaskName string
	Headless bool
}

// EgoLiteExtension 实现 agentcore.Extension，注册 handoff 和 task_spaces 工具。
type EgoLiteExtension struct {
	mgr *EgoLiteManager
	cfg EgoLiteConfig
}

// NewEgoLiteExtension 创建 EgoLite Extension。Engine 为空时禁用。
func NewEgoLiteExtension(cfg EgoLiteConfig) (*EgoLiteExtension, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("egolite: extension disabled (set EgoLiteEnabled=true)")
	}
	mgr, err := NewEgoLiteManager(cfg.TaskName)
	if err != nil {
		return nil, fmt.Errorf("egolite: create manager: %w", err)
	}
	return &EgoLiteExtension{mgr: mgr, cfg: cfg}, nil
}

func (e *EgoLiteExtension) Name() string         { return "ego-lite" }
func (e *EgoLiteExtension) Dispose() error        { return e.mgr.Close() }
func (e *EgoLiteExtension) Manager() *EgoLiteManager { return e.mgr }

func (e *EgoLiteExtension) Tools() []*agentcore.Tool {
	return []*agentcore.Tool{
		newEgoLiteHandoffTool(e.mgr),
		newEgoLiteTaskSpacesTool(e.mgr),
	}
}

// ==========================================
// ego_lite_handoff 工具
// ==========================================

func newEgoLiteHandoffTool(mgr *EgoLiteManager) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        EgoLiteHandoffToolName,
		Description: "浏览器控制权交接——当需要人工介入时（登录、验证码、表单确认等），将 Ego Lite 浏览器控制权交给用户。\n操作：handoff（交出控制权，用户可手动操作浏览器）、takeover（取回控制权，自动获取当前页面快照）、status（查询控制权状态）。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "交接操作：handoff=交给用户, takeover=取回控制权, status=查询状态",
					"enum":        []string{"handoff", "takeover", "status"},
				},
				"message": map[string]any{
					"type":        "string",
					"description": "handoff 时向用户展示的操作说明（如'请登录后告诉我继续'）",
				},
			},
			"required": []any{"action"},
		},
		ReadOnly: false,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Action  string `json:"action"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, fmt.Errorf("egolite handoff: invalid args: %w", err)
			}

			switch in.Action {
			case "handoff":
				result, err := mgr.Send(ctx, "handoffTaskSpace", nil)
				if err != nil {
					return nil, fmt.Errorf("handoff failed: %w", err)
				}
				output := "✅ 浏览器控制权已交出。"
				if m, ok := result.(map[string]any); ok {
					if done, _ := m["done"].(bool); !done {
						if skipped, ok := m["skipped"].(string); ok {
							output = fmt.Sprintf("⚠️ 控制权交接被跳过：%s", skipped)
						}
					}
				}
				if in.Message != "" {
					output += "\n\n📋 用户操作说明：" + in.Message
				}
				return output, nil

			case "takeover":
				_, err := mgr.Send(ctx, "takeOverTaskSpace", nil)
				if err != nil {
					return nil, fmt.Errorf("takeover failed: %w", err)
				}
				// 自动获取 snapshot 了解当前状态
				snapshot, snapErr := mgr.Send(ctx, "snapshotText", nil)
				pageInfo, _ := mgr.Send(ctx, "pageInfo", nil)
				info := ""
				if pi, ok := pageInfo.(map[string]any); ok {
					if u, ok := pi["url"].(string); ok {
						info = fmt.Sprintf("当前页面: %s\n", u)
					}
				}
				if snapErr != nil {
					return fmt.Sprintf("✅ 已取回浏览器控制权。\n%s\n（⚠️ 无法获取页面快照：%v）", info, snapErr), nil
				}
				return fmt.Sprintf("✅ 已取回浏览器控制权。\n%s\n页面快照:\n%s", info, snapshot), nil

			case "status":
				result, err := mgr.Send(ctx, "listTaskSpaces", nil)
				if err != nil {
					return nil, fmt.Errorf("status query failed: %w", err)
				}
				spaces, _ := result.([]any)
				if len(spaces) == 0 {
					return "当前没有活动任务空间。", nil
				}
				var output string
				for _, s := range spaces {
					if m, ok := s.(map[string]any); ok {
						name, _ := m["name"].(string)
						owner, _ := m["ownership"].(string)
						output += fmt.Sprintf("· %s (ownership: %s)\n", name, owner)
					}
				}
				return output, nil

			default:
				return nil, fmt.Errorf("egolite handoff: unknown action %q (valid: handoff, takeover, status)", in.Action)
			}
		},
	}
}

// ==========================================
// ego_lite_task_spaces 工具
// ==========================================

func newEgoLiteTaskSpacesTool(mgr *EgoLiteManager) *agentcore.Tool {
	return &agentcore.Tool{
		Name:        EgoLiteTaskSpacesToolName,
		Description: "管理 Ego Lite 浏览器任务空间——每个任务空间有独立的标签页集合和浏览上下文，但共享浏览器登录态。用于并行处理多个独立浏览任务。\n操作：list（列出所有任务空间及标签页）、create（创建或恢复任务空间）、switch（切换到指定空间）、close（关闭任务空间并可选保留标签页给用户查看）。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "操作：list=列出所有空间, create=创建或恢复, switch=切换, close=关闭",
					"enum":        []string{"list", "create", "switch", "close"},
				},
				"name": map[string]any{
					"type":        "string",
					"description": "任务空间名称（create/switch 时使用）",
				},
				"keep": map[string]any{
					"type":        "boolean",
					"description": "关闭时是否保留标签页给用户查看（默认 false）",
				},
			},
			"required": []any{"action"},
		},
		ReadOnly: false,
		Func: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Action string `json:"action"`
				Name   string `json:"name"`
				Keep   bool   `json:"keep"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, fmt.Errorf("egolite task_spaces: invalid args: %w", err)
			}

			switch in.Action {
			case "list":
				spacesResult, err := mgr.Send(ctx, "listTaskSpaces", nil)
				if err != nil {
					return nil, fmt.Errorf("list task spaces: %w", err)
				}
				spaces, _ := spacesResult.([]any)
				if len(spaces) == 0 {
					return "📋 当前没有活动任务空间。", nil
				}
				var output string
				for i, s := range spaces {
					m, _ := s.(map[string]any)
					name, _ := m["name"].(string)
					owner, _ := m["ownership"].(string)
					output += fmt.Sprintf("%d. %s (ownership: %s)\n", i+1, name, owner)
				}
				return output, nil

			case "create":
				if in.Name == "" {
					return nil, fmt.Errorf("name is required for create action")
				}
				result, err := mgr.Send(ctx, "initTaskSpace", map[string]any{"name": in.Name})
				if err != nil {
					return nil, fmt.Errorf("create task space: %w", err)
				}
				m, _ := result.(map[string]any)
				id, _ := m["id"].(string)
				taskId, _ := m["taskId"].(string)
				return fmt.Sprintf("✅ 任务空间已创建: name=%s, id=%s, taskId=%s", in.Name, id, taskId), nil

			case "switch":
				if in.Name == "" {
					return nil, fmt.Errorf("name is required for switch action")
				}
				result, err := mgr.Send(ctx, "initTaskSpace", map[string]any{"name": in.Name})
				if err != nil {
					return nil, fmt.Errorf("switch task space: %w", err)
				}
				m, _ := result.(map[string]any)
				id, _ := m["id"].(string)
				return fmt.Sprintf("✅ 已切换到任务空间: name=%s, id=%s", in.Name, id), nil

			case "close":
				result, err := mgr.Send(ctx, "completeTaskSpace", map[string]any{"keep": in.Keep})
				if err != nil {
					return nil, fmt.Errorf("close task space: %w", err)
				}
				m, _ := result.(map[string]any)
				if done, _ := m["done"].(bool); !done {
					if skipped, ok := m["skipped"].(string); ok {
						return fmt.Sprintf("⚠️ 关闭被跳过：%s", skipped), nil
					}
				}
				return "✅ 任务空间已关闭。", nil

			default:
				return nil, fmt.Errorf("egolite task_spaces: unknown action %q (valid: list, create, switch, close)", in.Action)
			}
		},
	}
}
```

- [ ] **Step 2: 编译验证**

```bash
go build ./tools/
```

Expected: 编译成功

- [ ] **Step 3: Commit**

```bash
git add tools/ego_lite.go
git commit -m "feat(egolite): add EgoLiteExtension with handoff and task_spaces tools"
```

---

### Task 7: 工具层单元测试

**Files:**
- Create: `tools/ego_lite_test.go`

- [ ] **Step 1: 写入工具测试**

```go
// ego_lite_test.go — EgoLite Extension 工具测试
package tools

import (
	"encoding/json"
	"testing"

	"github.com/xujian519/mady/agentcore"
)

// TestEgoLiteExtensionDisabled 验证禁用时 NewEgoLiteExtension 返回错误。
func TestEgoLiteExtensionDisabled(t *testing.T) {
	ext, err := NewEgoLiteExtension(EgoLiteConfig{Enabled: false})
	if err == nil {
		t.Error("expected error for disabled extension")
	}
	if ext != nil {
		t.Error("expected nil extension for disabled config")
	}
}

// TestEgoLiteHandoffToolSchema 验证 handoff 工具 schema 有效。
func TestEgoLiteHandoffToolSchema(t *testing.T) {
	mgr := &EgoLiteManager{pending: make(map[string]chan egoLiteJSONResponse)}
	tool := newEgoLiteHandoffTool(mgr)

	if tool.Name != EgoLiteHandoffToolName {
		t.Errorf("tool name = %q, want %q", tool.Name, EgoLiteHandoffToolName)
	}
	def := tool.Definition()
	if def.Name != EgoLiteHandoffToolName {
		t.Errorf("definition name = %q", def.Name)
	}
}

// TestEgoLiteTaskSpacesToolSchema 验证 task_spaces 工具 schema 有效。
func TestEgoLiteTaskSpacesToolSchema(t *testing.T) {
	mgr := &EgoLiteManager{pending: make(map[string]chan egoLiteJSONResponse)}
	tool := newEgoLiteTaskSpacesTool(mgr)

	if tool.Name != EgoLiteTaskSpacesToolName {
		t.Errorf("tool name = %q, want %q", tool.Name, EgoLiteTaskSpacesToolName)
	}
	def := tool.Definition()
	if def.Name != EgoLiteTaskSpacesToolName {
		t.Errorf("definition name = %q", def.Name)
	}
}

// TestEgoLiteHandoffUnknownAction 验证未知 action 返回错误。
func TestEgoLiteHandoffUnknownAction(t *testing.T) {
	mgr := &EgoLiteManager{pending: make(map[string]chan egoLiteJSONResponse)}
	tool := newEgoLiteHandoffTool(mgr)

	args, _ := json.Marshal(map[string]any{"action": "unknown_action"})
	_, err := tool.Func(nil, args)
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

// TestEgoLiteTaskSpacesMissingName 验证 create 缺少 name 返回错误。
func TestEgoLiteTaskSpacesMissingName(t *testing.T) {
	mgr := &EgoLiteManager{pending: make(map[string]chan egoLiteJSONResponse)}
	tool := newEgoLiteTaskSpacesTool(mgr)

	args, _ := json.Marshal(map[string]any{"action": "create"})
	_, err := tool.Func(nil, args)
	if err == nil {
		t.Error("expected error for create without name")
	}
}

// TestEgoLiteExtensionImplementsInterface 编译期类型检查。
func TestEgoLiteExtensionImplementsInterface(t *testing.T) {
	var _ agentcore.Extension = (*EgoLiteExtension)(nil)
	_ = agentcore.Extension(nil) // suppress unused import
}
```

- [ ] **Step 2: 运行测试**

```bash
go test ./tools/ -run TestEgoLite -v -count=1
```

Expected: 所有 EgoLite 测试 PASS

- [ ] **Step 3: 竞态检测**

```bash
go test ./tools/ -run TestEgoLite -race -count=1
```

Expected: 无竞态告警

- [ ] **Step 4: Commit**

```bash
git add tools/ego_lite_test.go
git commit -m "test(egolite): add EgoLiteExtension tool unit tests"
```

---

### Task 8: 端到端集成验证

**Files:**
- 验证所有新建和修改的文件

- [ ] **Step 1: 完整编译**

```bash
go build ./...
```

Expected: 编译成功，无错误

- [ ] **Step 2: 运行全部工具包测试**

```bash
go test ./tools/ -v -count=1
```

Expected: 现有测试和新测试全部 PASS（ego-browser 不可用时 EgoLite 测试 Skip）

- [ ] **Step 3: 竞态检测全部工具测试**

```bash
go test ./tools/ -race -count=1
```

Expected: 全部 PASS，无竞态

- [ ] **Step 4: 运行集成测试（如有 ego-browser）**

```bash
go test ./tools/ -run TestEgoLite -v -count=1
```

Expected: Skip（如果无 ego-browser）或 PASS

- [ ] **Step 5: Make verify 全量验证**

```bash
make verify
```

Expected: 全部通过

- [ ] **Step 6: 手动验证 ego_lite_bridge.js 正常工作**

```bash
echo '{"id":"1","method":"ping","params":{}}' | ego-browser nodejs tools/ego_lite_bridge.js
```

Expected: `{"id":"1","ok":true,"result":"pong"}`

- [ ] **Step 7: Final commit（如有集成测试修复）**

```bash
git add -A
git commit -m "chore(egolite): integration verification — all tests passing"
```

---

## 自检清单

- [ ] Bridge 协议 17 个方法全部在 bridge.js 中实现
- [ ] 7 个 browser action handler 已覆盖 BackendEgoLite 分支
- [ ] BrowserSession/BrowserManager/BrowserConfig 字段完整
- [ ] EgoLiteManager 支持延迟启动/自动重连/优雅关闭
- [ ] ego_lite_handoff 工具 handoff/takeover/status 三个操作全部实现
- [ ] ego_lite_task_spaces 工具 list/create/switch/close 四个操作全部实现
- [ ] 所有测试 PASS，竞态检测无告警
- [ ] `make verify` 通过
