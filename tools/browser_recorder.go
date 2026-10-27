package tools

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
)

type CDPRecorder struct {
	mu          sync.Mutex
	isRecording bool
	ctx         context.Context
	cancel      context.CancelFunc
	outputPath  string
	sessionID   string
	frameCount  int
	startedAt   time.Time
	webmFile    *os.File
}

func NewCDPRecorder(outputDir string) *CDPRecorder {
	return &CDPRecorder{
		outputPath: outputDir,
	}
}

func (r *CDPRecorder) StartRecording(ctx context.Context, browserCtx context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isRecording {
		return fmt.Errorf("already recording")
	}

	if err := os.MkdirAll(r.outputPath, 0755); err != nil { //nolint:gosec // G301: recording output dir needs write permission for media files
		return fmt.Errorf("failed to create recording dir: %w", err)
	}

	filename := fmt.Sprintf("recording_%s_%d.webm", sessionID, time.Now().Unix())
	filepath := filepath.Join(r.outputPath, filename)

	f, err := os.Create(filepath) //nolint:gosec // G304: path under managed recording output dir
	if err != nil {
		return fmt.Errorf("failed to create recording file: %w", err)
	}

	recCtx, recCancel := context.WithCancel(browserCtx)

	r.webmFile = f
	r.sessionID = sessionID
	r.isRecording = true
	r.frameCount = 0
	r.startedAt = time.Now()
	r.ctx = recCtx
	r.cancel = recCancel

	go r.captureFrames(recCtx)

	return nil
}

func (r *CDPRecorder) captureFrames(ctx context.Context) {
	session := targetSession(ctx)
	if session == nil {
		return
	}

	go func() {
		for e, err := range cdp.Events(ctx, session, page.ScreencastFrame) {
			if err != nil {
				return
			}
			// NOTE: do not read r.isRecording here without the lock — StopRecording
			// flips it under r.mu. The locked re-check below is the single source
			// of truth, so this loop stays race-free.
			r.mu.Lock()
			if !r.isRecording {
				r.mu.Unlock()
				return
			}

			if _, err := r.webmFile.Write(e.Data); err != nil {
				r.mu.Unlock()
				return
			}

			r.frameCount++
			r.mu.Unlock()

			if _, err := cdp.Call(ctx, session, page.ScreencastFrameAck, page.ScreencastFrameAckParams{SessionID: e.SessionID}); err != nil {
				return
			}
		}
	}()

	if _, err := cdp.Call(ctx, session, page.StartScreencast, page.StartScreencastParams{
		Format:        page.StartScreencastFormatJpeg,
		Quality:       new(int64(60)),
		EveryNthFrame: 1,
	}); err != nil {
		return
	}

	<-ctx.Done()

	_, _ = cdp.Call(context.WithoutCancel(ctx), session, page.StopScreencast, cdp.Empty{}) //nolint:gosec // G104: cleanup-only; best-effort stop
}

func (r *CDPRecorder) StopRecording() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isRecording {
		return "", fmt.Errorf("not recording")
	}

	if r.cancel != nil {
		r.cancel()
	}

	r.isRecording = false

	if r.webmFile != nil {
		if err := r.webmFile.Close(); err != nil {
			log.Printf("close webm file: %v", err)
		}
	}

	outputPath := r.webmFile.Name()
	return outputPath, nil
}

func (r *CDPRecorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isRecording
}

func (r *CDPRecorder) FrameCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.frameCount
}

func (r *CDPRecorder) Duration() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.isRecording {
		return time.Since(r.startedAt)
	}
	return time.Since(r.startedAt)
}
