package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LightpandaManager 管理 lightpanda 浏览器子进程的生命周期与会话录制。
type LightpandaManager struct {
	mu          sync.Mutex
	binaryPath  string
	processes   map[string]*LightpandaProcess
	cdpBasePort int
	nextPort    int
}

// LightpandaProcess 表示一个运行中的 lightpanda 子进程。
type LightpandaProcess struct {
	TaskID    string
	CDPURL    string
	Cmd       *exec.Cmd
	Port      int
	StartedAt time.Time
}

// NewLightpandaManager 创建 LightpandaManager。
func NewLightpandaManager() *LightpandaManager {
	return &LightpandaManager{
		binaryPath:  findLightpandaBinary(),
		processes:   make(map[string]*LightpandaProcess),
		cdpBasePort: 9222,
		nextPort:    9222,
	}
}

func findLightpandaBinary() string {
	paths := []string{
		"lightpanda",
		"/usr/local/bin/lightpanda",
		"/usr/bin/lightpanda",
	}
	for _, p := range paths {
		if path, err := exec.LookPath(p); err == nil {
			return path
		}
	}
	return ""
}

// StartProcess 为 taskID 启动一个 lightpanda 子进程。
func (lm *LightpandaManager) StartProcess(ctx context.Context, taskID string, headless bool) (*LightpandaProcess, error) {
	lm.mu.Lock()
	if existing, ok := lm.processes[taskID]; ok {
		lm.mu.Unlock()
		return existing, nil
	}

	if lm.binaryPath == "" {
		lm.mu.Unlock()
		return nil, fmt.Errorf("lightpanda binary not found. Install from https://github.com/lightpanda-io/lightpanda")
	}

	port := lm.nextPort
	lm.nextPort++

	args := []string{
		"--cdp-port", fmt.Sprintf("%d", port),
	}

	if headless {
		args = append(args, "--headless")
	}

	cmd := exec.CommandContext(ctx, lm.binaryPath, args...) //nolint:gosec // G204: lightpanda browser child process; binary validated at startup
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		lm.mu.Unlock()
		return nil, fmt.Errorf("failed to start lightpanda: %w", err)
	}

	cdpURL := fmt.Sprintf("ws://127.0.0.1:%d", port)

	process := &LightpandaProcess{
		TaskID:    taskID,
		CDPURL:    cdpURL,
		Cmd:       cmd,
		Port:      port,
		StartedAt: time.Now(),
	}

	// Register under the lock so concurrent StartProcess callers dedup on the
	// in-flight process, then release the lock before the slow readiness probe.
	lm.processes[taskID] = process
	lm.mu.Unlock()

	// waitForCDP runs curl polls + sleeps (up to 10s); keep it OUT of lm.mu
	// because CreateSession may be invoked while bm.mu is held.
	if err := lm.waitForCDP(cdpURL, 10*time.Second); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait() // reap to avoid a zombie
		lm.mu.Lock()
		delete(lm.processes, taskID)
		lm.mu.Unlock()
		return nil, fmt.Errorf("lightpanda CDP not ready: %w", err)
	}

	return process, nil
}

func (lm *LightpandaManager) waitForCDP(cdpURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isCDPReady(cdpURL) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for CDP at %s", cdpURL)
}

func isCDPReady(cdpURL string) bool {
	port := extractPortFromCDPURL(cdpURL)
	if port == 0 {
		return false
	}

	conn, err := exec.CommandContext(context.Background(), "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", fmt.Sprintf("http://127.0.0.1:%d/json/version", port)).Output() //nolint:gosec // G204: curl health check to localhost only
	if err != nil {
		// 健康检查请求失败→CDP 尚未就绪，返回 false 由调用方继续轮询。
		return false
	}

	return strings.Contains(string(conn), "200")
}

func extractPortFromCDPURL(url string) int {
	re := regexp.MustCompile(`:(\d+)`)
	matches := re.FindStringSubmatch(url)
	if len(matches) < 2 {
		return 0
	}
	// 正则已保证 matches[1] 全为数字；转换失败（如整数溢出）时返回 0 表示端口未知。
	port, err := strconv.Atoi(matches[1])
	if err != nil { // 端口未知，交由调用方按 0 处理
		return 0
	}
	return port
}

// StopProcess 停止 taskID 对应的 lightpanda 子进程（幂等）。
func (lm *LightpandaManager) StopProcess(taskID string) {
	// Collect + remove under the lock; kill+wait outside so a slow reap
	// doesn't block other callers.
	lm.mu.Lock()
	proc, ok := lm.processes[taskID]
	if ok {
		delete(lm.processes, taskID)
	}
	lm.mu.Unlock()

	if !ok {
		return
	}
	if proc.Cmd != nil && proc.Cmd.Process != nil {
		_ = proc.Cmd.Process.Kill()
		proc.Cmd.Wait() //nolint:gosec // G104: cleanup-only; reap the zombie, ignore "already exited" errors
	}
}

// StopAll 停止所有 lightpanda 子进程。
func (lm *LightpandaManager) StopAll() {
	lm.mu.Lock()
	procs := lm.processes
	lm.processes = make(map[string]*LightpandaProcess)
	lm.mu.Unlock()

	for _, proc := range procs {
		if proc.Cmd != nil && proc.Cmd.Process != nil {
			_ = proc.Cmd.Process.Kill()
			proc.Cmd.Wait() //nolint:gosec // G104: cleanup-only; reap the zombie, ignore "already exited" errors
		}
	}
}

// SessionRecorder 通过 lightpanda 帧回调录制 webm 文件。
type SessionRecorder struct {
	mu          sync.Mutex
	isRecording bool
	frames      []Frame
	outputPath  string
	frameChan   chan Frame
	doneChan    chan struct{}
}

// Frame 表示一帧图像数据。
type Frame struct {
	Data      []byte
	Timestamp time.Time
	Format    string
}

// NewSessionRecorder 创建输出到 outputDir 的录制器。
func NewSessionRecorder(outputDir string) *SessionRecorder {
	return &SessionRecorder{
		outputPath: outputDir,
		frameChan:  make(chan Frame, 100),
		doneChan:   make(chan struct{}),
	}
}

// StartRecording 开始录制并返回帧写入回调（供 lightpanda 进程注册）。
func (sr *SessionRecorder) StartRecording(ctx context.Context, sessionID string) error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if sr.isRecording {
		return fmt.Errorf("already recording")
	}

	sr.isRecording = true
	sr.frames = nil

	go sr.processFrames(ctx)

	return nil
}

func (sr *SessionRecorder) processFrames(ctx context.Context) {
	defer func() {
		sr.mu.Lock()
		sr.isRecording = false
		sr.mu.Unlock()
		close(sr.doneChan)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-sr.frameChan:
			if !ok {
				return
			}
			sr.frames = append(sr.frames, frame)
		}
	}
}

// AddFrame 向录制队列追加一帧图像数据。
func (sr *SessionRecorder) AddFrame(data []byte, format string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if !sr.isRecording {
		return
	}

	select {
	case sr.frameChan <- Frame{
		Data:      data,
		Timestamp: time.Now(),
		Format:    format,
	}:
	default:
	}
}

// StopRecording 停止录制并将帧编码为 webm 文件返回路径。
func (sr *SessionRecorder) StopRecording() (string, error) {
	// Flip the flag under the lock, then RELEASE it before closing frameChan
	// and waiting on doneChan. processFrames's defer needs sr.mu to clear the
	// flag; holding sr.mu across <-doneChan deadlocks forever.
	sr.mu.Lock()
	if !sr.isRecording {
		sr.mu.Unlock()
		return "", fmt.Errorf("not recording")
	}
	sr.isRecording = false
	sr.mu.Unlock()

	close(sr.frameChan)
	<-sr.doneChan

	sr.mu.Lock()
	defer sr.mu.Unlock()

	if len(sr.frames) == 0 {
		return "", fmt.Errorf("no frames captured")
	}

	outputFile := filepath.Join(sr.outputPath, fmt.Sprintf("recording_%d.webm", time.Now().Unix()))

	if err := sr.saveAsWebM(outputFile); err != nil {
		return "", fmt.Errorf("failed to save recording: %w", err)
	}

	return outputFile, nil
}

func (sr *SessionRecorder) saveAsWebM(outputPath string) error {
	if len(sr.frames) == 0 {
		return fmt.Errorf("no frames to save")
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg not found, required for recording: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "browser-recording")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	for i, frame := range sr.frames {
		framePath := filepath.Join(tmpDir, fmt.Sprintf("frame_%06d.png", i))
		if err := os.WriteFile(framePath, frame.Data, 0600); err != nil {
			return err
		}
	}

	cmd := exec.Command(ffmpegPath, //nolint:gosec,noctx // G204: ffmpeg video encoding; binary validated via LookPath; noctx: simple binary call with fixed args
		"-framerate", "10",
		"-pattern_type", "glob",
		"-i", filepath.Join(tmpDir, "frame_*.png"),
		"-c:v", "libvpx-vp9",
		"-pix_fmt", "yuv420p",
		"-y",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %w\n%s", err, string(output))
	}

	return nil
}

// IsRecording 报告是否正在录制。
func (sr *SessionRecorder) IsRecording() bool {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return sr.isRecording
}

// FrameCount 返回已捕获的帧数。
func (sr *SessionRecorder) FrameCount() int {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return len(sr.frames)
}
